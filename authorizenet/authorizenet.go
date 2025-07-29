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

	bom := []byte{0xef, 0xbb, 0xbf}
	body = bytes.TrimPrefix(body, bom)

	if err := json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return nil
}

type CustomerProfile struct {
	CustomerProfileId  string           `json:"customerProfileId,omitempty"`
	MerchantCustomerId string           `json:"merchantCustomerId,omitempty"`
	Description        string           `json:"description"`
	Email              string           `json:"email"`
	PaymentProfiles    []PaymentProfile `json:"paymentProfiles,omitempty"`
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

func (c *APIClient) GetCustomerProfile(profileID string) (*GetCustomerProfileResponse, error) {
	requestWrapper := struct {
		Request GetCustomerProfileRequest `json:"getCustomerProfileRequest"`
	}{
		Request: GetCustomerProfileRequest{
			MerchantAuthentication: c.Auth,
			CustomerProfileId:      profileID,
		},
	}

	var responseWrapper struct {
		Response GetCustomerProfileResponse `json:"getCustomerProfileResponse"`
	}

	if err := c.makeRequest(requestWrapper, &responseWrapper); err != nil {
		return nil, err
	}

	response := responseWrapper.Response
	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return nil, fmt.Errorf("API error: unknown error")
	}
	return &response, nil
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
	ids, err := c.GetAllCustomerProfileIds()
	if err != nil {
		return nil, err
	}

	var profiles []CustomerProfile
	for _, id := range ids {
		profile, err := c.GetCustomerProfile(id)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile.Profile)
	}
	return profiles, nil
}

type TransactionRequestType struct {
	TransactionType string `json:"transactionType"`
	Amount          string `json:"amount"`
	RefTransId      string `json:"refTransId,omitempty"`
	Profile         struct {
		CustomerProfileID string `json:"customerProfileId"`
		PaymentProfile    struct {
			PaymentProfileId string `json:"paymentProfileId"`
		} `json:"paymentProfile"`
	} `json:"profile"`
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

func (c *APIClient) ChargeCustomerProfile(profileID, paymentProfileID, amount string) (*FullTransactionResponse, error) {
	transactionRequest := TransactionRequestType{
		TransactionType: "authCaptureTransaction",
		Amount:          amount,
		Profile: struct {
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
		},
	}
	request := struct {
		Request CreateTransactionRequest `json:"createTransactionRequest"`
	}{
		Request: CreateTransactionRequest{
			MerchantAuthentication: c.Auth,
			TransactionRequest:     transactionRequest,
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

func (c *APIClient) AuthorizeCustomerProfile(profileID, paymentProfileID, amount string) (string, error) {
	transactionRequst := TransactionRequestType{
		TransactionType: "authOnlyTransaction",
		Amount:          amount,
		Profile: struct {
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
		},
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
		return "", err
	}
	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return "", fmt.Errorf("API error: unknown error")
	}
	return response.TransactionResponse.TransId, nil
}

func (c *APIClient) CapturePriorAuthTransaction(refTransId, amount string) (string, error) {
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
		return "", err
	}

	if response.Messages.ResultCode != "Ok" {
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return "", fmt.Errorf("API error: unknown error")
	}
	return response.TransactionResponse.TransId, nil
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
	CustomerType             string           `json:"customerType"`
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
