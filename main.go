package main

import (
	"authnet/authorizenet"
	"encoding/json"
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

	authnetEnv := os.Getenv("AUTHNET_ENVIRONMENT")
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

func (app *application) createCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// The validation mode can now come from the config for consistency
	validationMode := app.config.AuthNet.ValidationMode
	if req.ValidationMode != "" {
		validationMode = req.ValidationMode // Allow overriding from request
	}

	profileID, err := app.client.CreateCustomerProfile(req.Profile, validationMode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]string{"customerProfileId": profileID}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (app *application) getCustomerProfileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing customer profile ID", http.StatusBadRequest)
		return
	}

	profile, err := app.client.GetCustomerProfile(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile.Profile)
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
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		http.Error(w, "Missing request body", http.StatusBadRequest)
		return
	}

	var req AddShippingAddressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
