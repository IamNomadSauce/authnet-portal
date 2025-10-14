package authorizenet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

const (
	SandboxEndpoint    = "https://apitest.authorize.net/xml/v1/request.api"
	ProductionEndpoint = "https://api.authorize.net/xml/v1/request.api"
)

type MerchantAuthentication struct {
	Name           string `json:"name"`
	TransactionKey string `json:"transactionKey"`
}

type APIClient struct {
	Auth     MerchantAuthentication
	Endpoint string
}

func NewAPIClient(apiLoginID, transactionKey, endpoint string) *APIClient {
	return &APIClient{
		Auth: MerchantAuthentication{
			Name:           apiLoginID,
			TransactionKey: transactionKey,
		},
		Endpoint: endpoint,
	}
}

func (c *APIClient) makeRequest(requestBody interface{}, response interface{}) error {
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	// ✅ Trim whitespace from the body before checking its length
	trimmedBody := bytes.TrimSpace(body)

	// ✅ Only attempt to parse if the trimmed body has content
	if len(trimmedBody) > 0 {
		bom := []byte{0xef, 0xbb, 0xbf}
		bodyToParse := bytes.TrimPrefix(trimmedBody, bom)

		if err := json.Unmarshal(bodyToParse, response); err != nil {
			// Updated error message to include the problematic body for easier debugging
			return fmt.Errorf("failed to unmarshal response: %v. Body received: %s", err, string(bodyToParse))
		}
	}

	return nil
}

type CustomerProfile struct {
	CustomerProfileId  string            `json:"customerProfileId,omitempty"`
	MerchantCustomerId string            `json:"merchantCustomerId,omitempty"`
	Description        string            `json:"description"`
	Email              string            `json:"email"`
	ProfileType        string            `json:"profileType,omitempty"`
	PaymentProfiles    []PaymentProfile  `json:"paymentProfiles,omitempty"`
	ShipToList         []ShippingAddress `json:"shipToList,omitempty"`
}

type CreateCustomerProfileRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	Profile                CustomerProfile        `json:"profile"`
	ValidationMode         string                 `json:"validationMode,omitempty"`
}

type CreateCustomerProfileResponse struct {
	CustomerProfileId string `json:"customerProfileId"`
	Messages          struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

func (c *APIClient) CreateCustomerProfile(profile CustomerProfile, validationMode string) (string, error) {
	requestWrapper := struct {
		CreateCustomerProfileRequest CreateCustomerProfileRequest `json:"createCustomerProfileRequest"`
	}{
		CreateCustomerProfileRequest: CreateCustomerProfileRequest{
			MerchantAuthentication: c.Auth,
			Profile:                profile,
			ValidationMode:         validationMode,
		},
	}
	log.Printf("CreateCustomerProfile:ValidationMode:%s", validationMode)
	var response CreateCustomerProfileResponse
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return "", err
	}

	if response.Messages.ResultCode != "Ok" {
		log.Printf("Authorize.Net Error Response: %+v", response)
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error %s %s", response.Messages.Message[0].Text, response.Messages.Message[0].Code)
		}
		return "", fmt.Errorf("API error: Request failed with ResultCode '%s' but no message was provided", response.Messages.ResultCode)
	}
	return response.CustomerProfileId, nil
}

type GetCustomerProfileRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	CustomerProfileId      string                 `json:"customerProfileId"`
}

type GetCustomerProfileResponse struct {
	Profile  CustomerProfile `json:"profile"`
	Messages struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

// Change the function signature to return *CustomerProfile
func (c *APIClient) GetCustomerProfile(profileID string) (*CustomerProfile, error) {
	log.Println("--- GetCustomerProfile ---")

	requestWrapper := struct {
		Request GetCustomerProfileRequest `json:"getCustomerProfileRequest"`
	}{
		Request: GetCustomerProfileRequest{
			MerchantAuthentication: c.Auth,
			CustomerProfileId:      profileID,
		},
	}

	var response GetCustomerProfileResponse

	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return nil, err
	}

	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return nil, fmt.Errorf("API error: unknown error from Authorize.Net")
	}

	// Return the nested profile directly
	return &response.Profile, nil
}

type Paging struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type GetCustomerProfileIdsRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	Paging                 *Paging                `json:"paging,omitempty"`
}

type GetCustomerProfileIdsResponse struct {
	Ids                 []string `json:"ids"`
	TotalNumInResultSet int      `json:"totalNumInResultSet"`
	Messages            struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

func (c *APIClient) GetAllCustomerProfileIds() ([]string, error) {
	var allProfileIds []string
	limit := 1000
	offset := 1

	for {
		requestWrapper := struct {
			GetCustomerProfileIdsRequest GetCustomerProfileIdsRequest `json:"getCustomerProfileIdsRequest"`
		}{
			GetCustomerProfileIdsRequest: GetCustomerProfileIdsRequest{
				MerchantAuthentication: c.Auth,
				Paging: &Paging{
					Limit:  limit,
					Offset: offset,
				},
			},
		}

		var responseWrapper struct {
			GetCustomerProfileIdsResponse GetCustomerProfileIdsResponse `json:"getCustomerProfileIdsResponse"`
		}
		if err := c.makeRequest(requestWrapper, &responseWrapper); err != nil {
			return nil, fmt.Errorf("failed to make API request: %v", err)
		}

		response := responseWrapper.GetCustomerProfileIdsResponse

		if response.Messages.ResultCode != "Ok" {
			if len(response.Messages.Message) > 0 {
				return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
			}
			return nil, fmt.Errorf("API error: unknown error")
		}

		allProfileIds = append(allProfileIds, response.Ids...)

		if len(response.Ids) < limit {
			break
		}

		offset += limit
	}

	return allProfileIds, nil
}

func (c *APIClient) GetAllCustomerProfiles() ([]CustomerProfile, error) {
	log.Println("Get All Customer Profiles")
	ids, err := c.GetAllCustomerProfileIds()
	if err != nil {
		return nil, err
	}

	var profiles []CustomerProfile
	for _, id := range ids {
		// 'profile' is now type *CustomerProfile
		profile, err := c.GetCustomerProfile(id)
		if err != nil {
			return nil, err
		}
		// Append the dereferenced struct, not profile.Profile
		profiles = append(profiles, *profile)
	}
	return profiles, nil
}

type Order struct {
	InvoiceNumber string `json:"invoiceNumber,omitempty"`
	Description   string `json:"description,omitempty"`
}

type TransactionRequestType struct {
	TransactionType string `json:"transactionType"`
	Amount          string `json:"amount"`
	Profile         *struct {
		CustomerProfileID string `json:"customerProfileId"`
		PaymentProfile    struct {
			PaymentProfileId string `json:"paymentProfileId"`
		} `json:"paymentProfile"`
	} `json:"profile,omitempty"`
	Order      *Order `json:"order,omitempty"`
	RefTransId string `json:"refTransId,omitempty"`
}

type FullTransactionResponse struct {
	ResponseCode  string `json:"responseCode"`
	AuthCode      string `json:"authCode"`
	AvsResultCode string `json:"avsResultCode"`
	CvvResultCode string `json:"cvvResultCode"`
	TransId       string `json:"transId"`
	Messages      []struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"messages"`
	Errors []struct {
		ErrorCode string `json:"errorCode"`
		ErrorText string `json:"errorText"`
	} `json:"errors"`
}

type CreateTransactionRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	TransactionRequest     TransactionRequestType `json:"transactionRequest"`
}

type CreateTransactionResponse struct {
	TransactionResponse FullTransactionResponse `json:"transactionResponse"`
	Messages            struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

func (c *APIClient) ChargeCustomerProfile(profileID, paymentProfileID, amount, invoiceNumber, description, transactionType string) (*FullTransactionResponse, error) {
	log.Println("ChargeCustomerProfile")

	finalTransactionType := "authCaptureTransaction"
	if transactionType == "authOnlyTransaction" {
		finalTransactionType = "authOnlyTransaction"
	}

	profileData := &struct {
		CustomerProfileID string `json:"customerProfileId"`
		PaymentProfile    struct {
			PaymentProfileId string `json:"paymentProfileId"`
		} `json:"paymentProfile"`
	}{
		CustomerProfileID: profileID,
		PaymentProfile: struct {
			PaymentProfileId string `json:"paymentProfileId"`
		}{
			PaymentProfileId: paymentProfileID,
		},
	}

	transactionRequest := TransactionRequestType{
		TransactionType: finalTransactionType,
		Amount:          amount,
		Profile:         profileData,
	}
	if invoiceNumber != "" {
		transactionRequest.Order = &Order{
			InvoiceNumber: invoiceNumber,
			Description:   description,
		}
	}

	request := struct {
		Request CreateTransactionRequest `json:"createTransactionRequest"`
	}{
		Request: CreateTransactionRequest{
			MerchantAuthentication: c.Auth,
			TransactionRequest:     transactionRequest,
		},
	}

	log.Printf("Backend Charge Request %+v", request)
	var response CreateTransactionResponse
	if err := c.makeRequest(request, &response); err != nil {
		return nil, err
	}
	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			log.Printf("Error charging customer profile, %s", response.Messages.Message[0].Text)
			return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		log.Println("API Error: unknown error")
		return nil, fmt.Errorf("API error: unknown error")
	}
	return &response.TransactionResponse, nil
}

func (c *APIClient) AuthorizeCustomerProfile(profileID, paymentProfileID, amount string) (*FullTransactionResponse, error) {
	profileData := &struct {
		CustomerProfileID string `json:"customerProfileId"`
		PaymentProfile    struct {
			PaymentProfileId string `json:"paymentProfileId"`
		} `json:"paymentProfile"`
	}{
		CustomerProfileID: profileID,
		PaymentProfile: struct {
			PaymentProfileId string `json:"paymentProfileId"`
		}{
			PaymentProfileId: paymentProfileID,
		},
	}

	transactionRequst := TransactionRequestType{
		TransactionType: "authOnlyTransaction",
		Amount:          amount,
		Profile:         profileData, // <--- Assign the pointer
	}

	request := struct {
		Request CreateTransactionRequest `json:"createTransactionRequest"`
	}{
		Request: CreateTransactionRequest{
			MerchantAuthentication: c.Auth,
			TransactionRequest:     transactionRequst,
		},
	}

	var response CreateTransactionResponse
	if err := c.makeRequest(request, &response); err != nil {
		return nil, err
	}
	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return nil, fmt.Errorf("API error: unknown error")
	}
	return &response.TransactionResponse, nil
}

func (c *APIClient) CapturePriorAuthTransaction(refTransId, amount string) (*FullTransactionResponse, error) {
	// This request does NOT include the customer profile.
	transactionRequest := TransactionRequestType{
		TransactionType: "priorAuthCaptureTransaction",
		RefTransId:      refTransId,
		Amount:          amount,
	}

	requestWrapper := struct {
		CreateTransactionRequest CreateTransactionRequest `json:"createTransactionRequest"`
	}{
		CreateTransactionRequest: CreateTransactionRequest{
			MerchantAuthentication: c.Auth,
			TransactionRequest:     transactionRequest,
		},
	}

	var response CreateTransactionResponse
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return nil, err
	}

	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return nil, fmt.Errorf("API error: unknown error")
	}
	return &response.TransactionResponse, nil
}

type UpdateableProfileData struct {
	CustomerProfileId string `json:"customerProfileId"`
	Email             string `json:"email,omitempty"`
	Description       string `json:"description,omitempty"`
}

type UpdateCustomerProfileRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	Profile                UpdateableProfileData  `json:"profile"`
}

func (c *APIClient) UpdateCustomerProfile(profileID, email, description string) error {
	requestWrapper := struct {
		Request UpdateCustomerProfileRequest `json:"updateCustomerProfileRequest"`
	}{
		Request: UpdateCustomerProfileRequest{
			MerchantAuthentication: c.Auth,
			Profile: UpdateableProfileData{
				CustomerProfileId: profileID,
				Email:             email,
				Description:       description,
			},
		},
	}

	var response struct {
		Messages struct {
			ResultCode string `json:"resultCode"`
			Message    []struct {
				Code string `json:"code"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"messages"`
	}
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return err
	}
	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return fmt.Errorf("API error: unknown error")
	}

	return nil
}

type ShippingAddress struct {
	CustomerAddressId string `json:"customerAddressId,omitempty"`
	FirstName         string `json:"firstName"`
	LastName          string `json:"lastName"`
	Address           string `json:"address"`
	City              string `json:"city"`
	State             string `json:"state"`
	Zip               string `json:"zip"`
	Country           string `json:"country"`
}

type CreateCustomerShippingAddressRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	CustomerProfileId      string                 `json:"customerProfileId"`
	Address                ShippingAddress        `json:"address"`
}

type CreateCustomerShippingAddressResponse struct {
	CustomerAddressId string `json:"customerAddressId"`
	Messages          struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

func (c *APIClient) AddShippingAddress(profileID string, address ShippingAddress) (string, error) {
	log.Println("Add shipping address to profile:", profileID)

	requestWrapper := struct {
		Request CreateCustomerShippingAddressRequest `json:"createCustomerShippingAddressRequest"`
	}{
		Request: CreateCustomerShippingAddressRequest{
			MerchantAuthentication: c.Auth,
			CustomerProfileId:      profileID,
			Address:                address,
		},
	}

	var response CreateCustomerShippingAddressResponse
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return "", err
	}

	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error: %s (Code: %s)", response.Messages.Message[0].Text, response.Messages.Message[0].Code)
		}
		return "", fmt.Errorf("API error: add shipping address failed with ResultCode '%s'", response.Messages.ResultCode)
	}

	return response.CustomerAddressId, nil
}

type DeleteCustomerShippingAddressRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	CustomerProfileId      string                 `json:"customerProfileId"`
	CustomerAddressId      string                 `json:"customerAddressId"`
}

func (c *APIClient) DeleteShippingAddress(profileID, addressID string) error {
	requestWrapper := struct {
		Request DeleteCustomerShippingAddressRequest `json:"deleteCustomerShippingAddressRequest"`
	}{
		Request: DeleteCustomerShippingAddressRequest{
			MerchantAuthentication: c.Auth,
			CustomerProfileId:      profileID,
			CustomerAddressId:      addressID,
		},
	}

	// This variable is just a placeholder for makeRequest. Our updated makeRequest
	// will handle the empty body from a successful delete and won't try to parse it.
	var response interface{}

	// If makeRequest returns an error, it's a real issue.
	// If it returns nil, the delete was successful.
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return err
	}

	return nil
}

type UpdateCustomerPaymentProfileRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	CustomerProfileId      string                 `json:"customerProfileId"`
	PaymentProfile         PaymentProfile         `json:"paymentProfile"`
}

type UpdateCustomerPaymentProfileResponse struct {
	Messages struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

func (c *APIClient) UpdateBillingAddress(customerprofileID string, paymentProfileID string, address ShippingAddress) error {
	log.Printf("Updating billing address for customer profile: %s, payment profile: %s", customerprofileID, paymentProfileID)

	requestWrapper := struct {
		Request UpdateCustomerPaymentProfileRequest `json:"updateCustomerPaymentProfileRequest"`
	}{
		Request: UpdateCustomerPaymentProfileRequest{
			MerchantAuthentication: c.Auth,
			CustomerProfileId:      customerprofileID,
			PaymentProfile: PaymentProfile{
				CustomerPaymentProfileId: paymentProfileID,
				BillTo:                   &address,
			},
		},
	}

	var response UpdateCustomerPaymentProfileResponse
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return err
	}

	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return fmt.Errorf("API error: %s (Code: %s)", response.Messages.Message[0].Text, response.Messages.Message[0].Code)
		}
		return fmt.Errorf("API error: update billing failed with ResultCode '%s'", response.Messages.ResultCode)
	}

	return nil
}

type CreditCard struct {
	CardNumber     string `json:"cardNumber"`
	ExpirationDate string `json:"expirationDate"`
}

type Payment struct {
	CreditCard CreditCard `json:"creditCard"`
}

type PaymentProfile struct {
	CustomerPaymentProfileId string           `json:"customerPaymentProfileId,omitempty"`
	CustomerType             string           `json:"customerType,omitempty"`
	BillTo                   *ShippingAddress `json:"billTo,omitempty"`
	Payment                  Payment          `json:"payment"`
}

type CreateCustomerPaymentProfileRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	CustomerProfileId      string                 `json:"customerProfileId"`
	PaymentProfile         PaymentProfile         `json:"paymentProfile"`
}

func (c *APIClient) AddPaymentProfile(profileID string, creditCard CreditCard) (string, error) {
	requestWrapper := struct {
		Request CreateCustomerPaymentProfileRequest `json:"createCustomerPaymentProfileRequest"`
	}{
		Request: CreateCustomerPaymentProfileRequest{
			MerchantAuthentication: c.Auth,
			CustomerProfileId:      profileID,
			PaymentProfile: PaymentProfile{
				Payment: Payment{
					CreditCard: creditCard,
				},
			},
		},
	}

	var response struct {
		CustomerPaymentProfileId string `json:"customerPaymentProfileId"`
		Messages                 struct {
			ResultCode string `json:"resultCode"`
			Message    []struct {
				Code string `json:"code"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"messages"`
	}
	if err := c.makeRequest(requestWrapper, &response); err != nil {
		return "", err
	}
	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return "", fmt.Errorf("API error: unknown error")
	}
	return response.CustomerPaymentProfileId, nil
}
