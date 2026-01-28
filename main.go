package main

import (
	"authnet/authorizenet"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	_ "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"strings"

	"database/sql"

	_ "github.com/lib/pq"
)

type authNetConfig struct {
	Endpoint       string
	ValidationMode string
	LoginID        string
	TransactionKey string
}

type config struct {
	AuthNet authNetConfig
}

type application struct {
	config *config
	client *authorizenet.APIClient
	db     *sql.DB
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file", err)
	}

	cfg := &config{}
	cfg.AuthNet.LoginID = os.Getenv("AUTHORIZENET_NAME")
	cfg.AuthNet.TransactionKey = os.Getenv("AUTHORIZENET_TRANSACTION_KEY")

	if cfg.AuthNet.LoginID == "" || cfg.AuthNet.TransactionKey == "" {
		log.Fatal("Missing login-id or transaction-key")
	}

	authnetEnv := os.Getenv("AUTHORIZENET_ENVIRONMENT")
	log.Printf("Main: authnetEnv: %s", authnetEnv)
	if authnetEnv == "production" {
		cfg.AuthNet.ValidationMode = "liveMode"
		cfg.AuthNet.Endpoint = authorizenet.ProductionEndpoint
	} else {
		cfg.AuthNet.ValidationMode = "testMode"
		cfg.AuthNet.Endpoint = authorizenet.SandboxEndpoint
	}

	db_dsn := os.Getenv("DB_DSN")
	if db_dsn == "" {
		log.Fatal("Missing DB_DSN")
	}

	db, err := sql.Open("postgres", db_dsn)
	if err != nil {
		log.Fatalf("Cannot connect to database: %v", err)
	}
	defer db.Close()

	log.Println("Database connected")

	client := authorizenet.NewAPIClient(
		cfg.AuthNet.LoginID,
		cfg.AuthNet.TransactionKey,
		cfg.AuthNet.Endpoint,
	)

	app := &application{
		config: cfg,
		client: client,
		db:     db,
	}

	r := mux.NewRouter()

	allowedOrigins := handlers.AllowedOrigins([]string{"https://www.handbellworld.com"})
	allowedMethods := handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	allowedHeaders := handlers.AllowedHeaders([]string{"Content-Type", "Authorization"})
	corsHandler := handlers.CORS(allowedOrigins, allowedMethods, allowedHeaders)(r)

	r.HandleFunc("/customer-profiles", app.createCustomerProfileHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}", app.getCustomerProfileHandler).Methods("GET")
	r.HandleFunc("/customer-profiles", app.getAllCustomerProfilesHandler).Methods("GET")

	r.HandleFunc("/customer-profiles/{id}", app.updateCustomerProfileHandler).Methods("PUT")
	r.HandleFunc("/customer-profiles/{id}/shipping-addresses", app.addShippingAddressHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}/shipping-addresses/{addressId}", app.deleteShippingAddressHandler).Methods("DELETE")
	r.HandleFunc("/customer-profiles/{id}/payment-profiles", app.addPaymentProfileHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}/payment-profiles/{paymentProfileId}", app.updateBillingAddressHandler).Methods("PUT")

	r.HandleFunc("/customer-profiles/{customerProfileId}/payment-profiles/{paymentProfileId}", app.updateCustomerPaymentProfileHandler).Methods("PUT")
	r.HandleFunc("/customer-profiles/{customerProfileId}/payment-profiles/{paymentProfileId}", app.deletePaymentProfileHandler).Methods("DELETE")

	r.HandleFunc("/update-payment-profile", app.updatePaymentProfileHandler).Methods("PUT")

	r.HandleFunc("/transactions", app.chargeCustomerProfileHandler).Methods("POST")
	r.HandleFunc("/transactions/authorize", app.authorizeCustomerProfileHandler).Methods("POST")
	r.HandleFunc("/transactions/capture", app.capturePriorAuthTransactionHandler).Methods("POST")

	log.Println("Server starting on :1337")
	if err := http.ListenAndServeTLS(":1337", "cert.pem", "key.pem", corsHandler); err != nil {
		log.Fatal(err)
	}
}

type ApiResponse struct {
	IsSuccess   bool                                  `json:"is_success"`
	Message     string                                `json:"message"`
	Action      string                                `json:"action,omitempty"`
	Transaction *authorizenet.FullTransactionResponse `json:"transaction,omitempty"`
}

type CreateProfileRequest struct {
	Profile        authorizenet.CustomerProfile `json:"profile"`
	ValidationMode string                       `json:"validationMode"`
}

type ChargeRequest struct {
	ProfileID        string `json:"profileId"`
	PaymentProfileID string `json:"paymentProfileId"`
	Amount           string `json:"amount"`
	InvoiceNumber    string `json:"invoiceNumber,omitempty"`
	Description      string `json:"description,omitempty"`
	TransactionType  string `json:"transactionType,omitempty"`
}

type CaptureRequest struct {
	RefTransId string `json:"refTransId"`
	Amount     string `json:"amount,omitempty"`
}

type UpdateProfileRequest struct {
	Email       string `json:"email"`
	Description string `json:"description"`
}

type AddPaymentProfileRequest struct {
	CreditCard authorizenet.CreditCard `json:"creditCard"`
}

type AddShippingAddressRequest struct {
	Address authorizenet.ShippingAddress `json:"address"`
}

type UpdateBillingAddressRequest struct {
	Address authorizenet.ShippingAddress `json:"address"`
}

func (app *application) createCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	log.Print("Create New Customer Profile Handler")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read request body", http.StatusInternalServerError)
		return
	}

	// NEW: Sanitize malformed JSON from CFM (replace \' with ' to fix escape errors)
	bodyStr := string(body)
	bodyStr = strings.Replace(bodyStr, "\\'", "'", -1)
	body = []byte(bodyStr)

	// log.Printf("Sanitized JSON payload: %s", string(body)) // Log for verification

	var req CreateProfileRequest

	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		log.Printf("--> JSON decoding error: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// log.Printf("Successfully decoded request.")

	validationMode := app.config.AuthNet.ValidationMode
	if req.ValidationMode != "" {
		validationMode = req.ValidationMode
	}

	// log.Printf("Create Customer Profile: ValidationMode %s", validationMode)

	profileID, err := app.client.CreateCustomerProfile(req.Profile, validationMode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully created profile. Returning ID: %s to client.", profileID)

	response := map[string]string{"customerProfileId": profileID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (app *application) getCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Get Customer Handler")
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing customer profile ID", http.StatusBadRequest)
		return
	}

	// The 'profile' variable is now the *CustomerProfile object you want
	profile, err := app.client.GetCustomerProfile(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Just encode the profile object directly
	json.NewEncoder(w).Encode(profile)
}
func (app *application) getAllCustomerProfilesHandler(w http.ResponseWriter, r *http.Request) {
	profiles, err := app.client.GetAllCustomerProfiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profiles)
}

func (app *application) chargeCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Charge Customer Profile Handler")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read request body", http.StatusInternalServerError)
		return
	}
	log.Printf("--- Raw /transactions request body: %s", string(body))

	var req ChargeRequest
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		log.Printf("!!! Failed to decode JSON body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Successfully decoded ChargeRequest: %+v", req)

	transactionResponse, err := app.client.ChargeCustomerProfile(req.ProfileID, req.PaymentProfileID, req.Amount, req.InvoiceNumber, req.TransactionType, req.Description)

	w.Header().Set("Content-Type", "application/json")

	// Handle errors by sending the standard ApiResponse
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ApiResponse{
			IsSuccess: false,
			Message:   err.Error(),
		})
		return
	}

	// Determine the action for the response
	action := "authCaptureTransaction"
	if req.TransactionType == "authOnlyTransaction" {
		action = "authOnlyTransaction"
	}

	// On success, wrap the result in the standard ApiResponse
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ApiResponse{
		IsSuccess:   true,
		Message:     "Transaction successful.",
		Action:      action,
		Transaction: transactionResponse,
	})
}

func (app *application) authorizeCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	var req ChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ProfileID == "" || req.PaymentProfileID == "" || req.Amount == "" {
		http.Error(w, "Missing required fields: profileId, paymentProfileId, or amount", http.StatusBadRequest)
		return
	}

	// The function now returns the full transaction response object
	fullResponse, err := app.client.AuthorizeCustomerProfile(req.ProfileID, req.PaymentProfileID, req.Amount)

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		// On error, send a structured error response
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ApiResponse{
			IsSuccess: false,
			Message:   err.Error(),
		})
		return
	}

	// On success, send a structured success response
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ApiResponse{
		IsSuccess:   true,
		Message:     "Transaction authorized successfully.",
		Action:      "AUTH_ONLY",
		Transaction: fullResponse,
	})
}

func (app *application) capturePriorAuthTransactionHandler(w http.ResponseWriter, r *http.Request) {
	var req CaptureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	log.Printf("Version 2 Handler:Capture Prior Auth Transaction %+v", req)
	if req.RefTransId == "" {
		// Add a version marker to the error message
		http.Error(w, "V2 Error: Missing required field: refTransId", http.StatusBadRequest)
		return
	}
	// This function also returns the full response now
	fullResponse, err := app.client.CapturePriorAuthTransaction(req.RefTransId, req.Amount)

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		log.Printf("Error Capturing Prior Auth Transaction: %+v", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ApiResponse{
			IsSuccess: false,
			Message:   err.Error(),
		})
		return
	}

	responseBytes, err := json.Marshal(ApiResponse{
		IsSuccess:   true,
		Message:     "Previously authorized transaction captured successfully.",
		Action:      "priorAuthCaptureTransaction",
		Transaction: fullResponse,
	})
	if err != nil {
		log.Printf("Failed to carshal capture response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ApiResponse{
			IsSuccess: false,
			Message:   "Failed to process transaction response internally.",
		})
		return
	}

	originalTransId := req.RefTransId
	newTransId := fullResponse.TransId

	stmt := `
		UPDATE header
			SET authorizenet_results = authorizenet_results || '|' || $1,
			authorizenet_ts = now(),
			transactionnum = $2
		WHERE transactionnum = $3;
	`

	_, dbErr := app.db.Exec(stmt, string(responseBytes), newTransId, originalTransId)

	if dbErr != nil {
		log.Printf("Database update failed: %v", dbErr)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ApiResponse{
			IsSuccess: false,
			Message:   "CRITICAL:Payment was processed but failed to update order record.",
		})
		return
	}

	log.Printf("Successfully updated order record for transaction %s", originalTransId)

	w.WriteHeader(http.StatusCreated)
	w.Write(responseBytes)
}

func (app *application) updateCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing customer profile ID", http.StatusBadRequest)
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := app.client.UpdateCustomerProfile(id, req.Email, req.Description); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (app *application) updateCustomerPaymentProfileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	customerProfileId := vars["customerProfileId"]
	paymentProfileId := vars["paymentProfileId"]

	var req struct {
		CreditCard authorizenet.CreditCard      `json:"creditCard"`
		BillTo     authorizenet.ShippingAddress `json:"billTo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := app.client.UpdateCustomerPaymentProfile(customerProfileId, paymentProfileId, req.CreditCard, req.BillTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Payment profile updated successfully"})
}

func (app *application) addShippingAddressHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Add Shipping Address Handler reached.")
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing customer profile ID in URL path", http.StatusBadRequest)
		return
	}
	log.Println("Profile ID from URL:", id)

	// Add logging to see the raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read request body", http.StatusInternalServerError)
		return
	}
	log.Printf("Received raw JSON for shipping address: %s", string(body))

	var req AddShippingAddressRequest
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		log.Printf("--> JSON decoding error: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	addressID, err := app.client.AddShippingAddress(id, req.Address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]string{"customerAddressId": addressID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (app *application) deleteShippingAddressHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Delete Shipping Address Handler")
	vars := mux.Vars(r)
	profileId, ok1 := vars["id"]
	addressId, ok2 := vars["addressId"]

	if !ok1 || !ok2 {
		http.Error(w, "Missing customer or address profile ID in URL", http.StatusBadRequest)
		return
	}

	err := app.client.DeleteShippingAddress(profileId, addressId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (app *application) addPaymentProfileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing customer profile ID", http.StatusBadRequest)
		return
	}

	var req AddPaymentProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	paymentProfileID, err := app.client.AddPaymentProfile(id, req.CreditCard)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]string{"customerPaymentProfileId": paymentProfileID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (app *application) updatePaymentProfileHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("=== UPDATE PAYMENT PROFILE HANDLER REACHED ===")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read body: %v", err)
		http.Error(w, "Cannot read request body", http.StatusInternalServerError)
		return
	}
	log.Printf("Raw body for update: %s", string(body))

	// FIXED: Nest Payment to match incoming JSON: "payment": { "creditCard": { ... } }
	var req struct {
		CustomerProfileId string `json:"customerProfileId"`
		PaymentProfileId  string `json:"paymentProfileId"`
		Payment           struct {
			CreditCard authorizenet.CreditCard `json:"creditCard"`
		} `json:"payment"`
		BillTo authorizenet.ShippingAddress `json:"billTo"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("Decode error: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	log.Printf("Decoded req: |%+v|", req) // Now CardNumber should log as populated

	customerProfileId := req.CustomerProfileId
	paymentProfile := struct {
		BillTo  *authorizenet.ShippingAddress `json:"billTo,omitempty"`
		Payment struct {
			CreditCard authorizenet.CreditCard `json:"creditCard,omitempty"`
		} `json:"payment,omitempty"`
		CustomerPaymentProfileId string `json:"customerPaymentProfileId,omitempty"`
	}{
		BillTo: &req.BillTo,
		// FIXED: Access the nested CreditCard
		Payment: struct {
			CreditCard authorizenet.CreditCard `json:"creditCard,omitempty"`
		}{
			CreditCard: req.Payment.CreditCard, // Now passes the actual card details
		},
		CustomerPaymentProfileId: req.PaymentProfileId,
	}

	err = app.client.UpdatePaymentProfile(customerProfileId, &paymentProfile)
	if err != nil {
		log.Printf("API error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Println("Payment profile updated successfully")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Payment profile updated successfully"})
}

func (app *application) deletePaymentProfileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	customerProfileId := vars["customerProfileId"]
	paymentProfileId := vars["paymentProfileId"]

	if customerProfileId == "" || paymentProfileId == "" {
		http.Error(w, "Missing customerProfileId or paymentProfileId", http.StatusBadRequest)
		return
	}

	err := app.client.DeletePaymentProfile(customerProfileId, paymentProfileId)
	if err != nil {
		log.Printf("Error deleting payment profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Payment profile deleted successfully"})
}

func (app *application) updateBillingAddressHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Updating Customer Billing Address")

	// Read the raw body from the request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("!!! Could not read request body: %v", err)
		http.Error(w, "Cannot read request body", http.StatusInternalServerError)
		return
	}

	// Log the raw body so we can see exactly what ColdFusion is sending
	log.Printf("--- Raw request body received: %s", string(body))

	vars := mux.Vars(r)
	customerProfileId, ok1 := vars["id"]
	paymentProfileId, ok2 := vars["paymentProfileId"]

	if !ok1 || !ok2 {
		http.Error(w, "Missing customer or payment profile ID in URL", http.StatusBadRequest)
		return
	}

	var req UpdateBillingAddressRequest
	// We now use bytes.NewReader(body) because the original r.Body has already been read
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		log.Printf("!!! Failed to decode billing address JSON body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err = app.client.UpdateBillingAddress(customerProfileId, paymentProfileId, req.Address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
