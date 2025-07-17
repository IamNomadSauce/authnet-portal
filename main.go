package main

import (
	"authnet/authorizenet"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

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

func createCustomerProfileHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		profileID, err := client.CreateCustomerProfile(req.Profile, req.ValidationMode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := map[string]string{"customerProfileId": profileID}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func getCustomerProfileHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, ok := vars["id"]
		if !ok {
			http.Error(w, "Missing customer profile ID", http.StatusBadRequest)
			return
		}

		profile, err := client.GetCustomerProfile(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile.Profile)
	}
}

func getAllCustomerProfilesHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := client.GetAllCustomerProfiles()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profiles)
	}
}

func chargeCustomerProfileHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ChargeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		transID, err := client.ChargeCustomerProfile(req.ProfileID, req.PaymentProfileID, req.Amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := map[string]string{"transactionId": transID}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func authorizeCustomerProfileHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ChargeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.ProfileID == "" || req.PaymentProfileID == "" || req.Amount == "" {
			http.Error(w, "Missing required fields: profileId, paymentProfileId, or amount", http.StatusBadRequest)
			return
		}
		transID, err := client.AuthorizeCustomerProfile(req.ProfileID, req.PaymentProfileID, req.Amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := map[string]string{"transactionId": transID}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func capturePriorAuthTransactionHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CaptureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.RefTransId == "" {
			http.Error(w, "Missing required field: refTransId", http.StatusBadRequest)
			return
		}
		transID, err := client.CapturePriorAuthTransaction(req.RefTransId, req.Amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response := map[string]string{"transactionId": transID}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func updateCustomerProfileHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		if err := client.UpdateCustomerProfile(id, req.Email, req.Description); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func addShippingAddressHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		addressID, err := client.AddShippingAddress(id, req.Address)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := map[string]string{"customerAddressId": addressID}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func addPaymentProfileHandler(client *authorizenet.APIClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		paymentProfileID, err := client.AddPaymentProfile(id, req.CreditCard)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := map[string]string{"customerPaymentProfileId": paymentProfileID}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file", err)
	}
	apiLoginID := os.Getenv("AUTHORIZENET_NAME")
	transactionKey := os.Getenv("AUTHORIZENET_TRANSACTION_KEY")

	if apiLoginID == "" || transactionKey == "" {
		log.Fatal("Missing login-id or transaction-key")
	}

	client := authorizenet.NewAPIClient(apiLoginID, transactionKey, authorizenet.SandboxEndpoint)

	r := mux.NewRouter()
	r.HandleFunc("/customer-profiles", createCustomerProfileHandler(client)).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}", getCustomerProfileHandler(client)).Methods("GET")
	r.HandleFunc("/customer-profiles", getAllCustomerProfilesHandler(client)).Methods("GET")
	r.HandleFunc("/transactions", chargeCustomerProfileHandler(client)).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}", updateCustomerProfileHandler(client)).Methods("PUT")
	r.HandleFunc("/customer-profiles/{id}/shipping-addresses", addShippingAddressHandler(client)).Methods("POST")
	r.HandleFunc("/customer-profiles/{id}/payment-profiles", addPaymentProfileHandler(client)).Methods("POST")
	r.HandleFunc("/transactions/authorize", authorizeCustomerProfileHandler(client)).Methods("POST")
	r.HandleFunc("/transactions/capture", capturePriorAuthTransactionHandler(client)).Methods("POST")

	log.Println("Server starting on :1337")
	if err := http.ListenAndServe(":1337", r); err != nil {
		log.Fatal(err)
	}
}
