package main

import (
	"authnet/authorizenet"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	// "golang.org/x/crypto/nacl/auth"
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

	client := authorizenet.NewAPIClient(
		cfg.AuthNet.LoginID,
		cfg.AuthNet.TransactionKey,
		cfg.AuthNet.Endpoint,
	)

	app := &application{
		config: cfg,
		client: client,
	}

	r := mux.NewRouter()
	r.HandleFunc("/customer-profiles", app.createCustomerProfileHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}", app.getCustomerProfileHandler).Methods("GET")
	r.HandleFunc("/customer-profiles", app.getAllCustomerProfilesHandler).Methods("GET")
	r.HandleFunc("/transactions", app.chargeCustomerProfileHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}", app.updateCustomerProfileHandler).Methods("PUT")
	r.HandleFunc("/customer-profiles/{id}/shipping-addresses", app.addShippingAddressHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}/payment-profiles", app.addPaymentProfileHandler).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}/payment-profiles/{paymentProfileId}", app.updateBillingAddressHandler).Methods("PUT")
	r.HandleFunc("/transactions/authorize", app.authorizeCustomerProfileHandler).Methods("POST")
	r.HandleFunc("/transactions/capture", app.capturePriorAuthTransactionHandler).Methods("POST")

	log.Println("Server starting on :1337")
	if err := http.ListenAndServeTLS(":1337", "cert.pem", "key.pem", r); err != nil {
		log.Fatal(err)
	}
}

type CreateProfileRequest struct {
	Profile        authorizenet.CustomerProfile `json:"profile"`
	ValidationMode string                       `json:"validationMode"`
}

type ChargeRequest struct {
	ProfileID        string `json:"profileId"`
	PaymentProfileID string `json:"paymentProfileId"`
	Amount           string `json:"amount"`
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read request body", http.StatusInternalServerError)
		return
	}

	log.Printf("Received raw JSON payload: %s", string(body))

	var req CreateProfileRequest

	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		log.Printf("--> JSON decoding error: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Successfully decoded request.")

	validationMode := app.config.AuthNet.ValidationMode
	if req.ValidationMode != "" {
		validationMode = req.ValidationMode
	}

	log.Printf("Create Customer Profile: ValidationMode %s", validationMode)

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
	var req ChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	transactionResponse, err := app.client.ChargeCustomerProfile(req.ProfileID, req.PaymentProfileID, req.Amount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(transactionResponse)
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
	transID, err := app.client.AuthorizeCustomerProfile(req.ProfileID, req.PaymentProfileID, req.Amount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response := map[string]string{"transactionId": transID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (app *application) capturePriorAuthTransactionHandler(w http.ResponseWriter, r *http.Request) {
	var req CaptureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RefTransId == "" {
		http.Error(w, "Missing required field: refTransId", http.StatusBadRequest)
		return
	}
	transID, err := app.client.CapturePriorAuthTransaction(req.RefTransId, req.Amount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response := map[string]string{"transactionId": transID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
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

func (app *application) updateBillingAddressHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Updating Customer Billing Address")
	vars := mux.Vars(r)
	customerProfileId, ok1 := vars["id"]
	paymentProfiled, ok2 := vars["paymentProfileId"]

	if !ok1 || !ok2 {
		http.Error(w, "Missing customer or payment profile ID in URL", http.StatusBadRequest)
		return
	}

	var req UpdateBillingAddressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := app.client.UpdateBillingAddress(customerProfileId, paymentProfiled, req.Address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
