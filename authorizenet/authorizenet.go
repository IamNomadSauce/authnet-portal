package authorizenet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	if err := json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return nil
}

type CustomerProfile struct {
	Email       string `json:"email"`
	Description string `json:"description"`
}

type CreateCustomerProfileRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	Profile                CustomerProfile        `json:"profile"`
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

func (c *APIClient) CreateCustomerProfile(profile CustomerProfile) (string, error) {
	request := CreateCustomerProfileRequest{
		MerchantAuthentication: c.Auth,
		Profile:                profile,
	}
	var response CreateCustomerProfileResponse
	if err := c.makeRequest(request, &response); err != nil {
		return "", err
	}
	if response.Messages.ResultCode != "OK" {
		return "", fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
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
	request := GetCustomerProfileRequest{
		MerchantAuthentication: c.Auth,
		CustomerProfileId:      profileID,
	}
	var response GetCustomerProfileResponse
	if err := c.makeRequest(request, &response); err != nil {
		return nil, err
	}
	if response.Messages.ResultCode != "OK" {
		return nil, fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
	}
	return &response, nil
}

type TransactionRequestType struct {
	TransactionType string `json:"transactionType"`
	Amount          string `json:"amount"`
	Profile         struct {
		CustomerProfileID string `json:"customerProfileId"`
		PaymentProfile    struct {
			PaymentProfileId string `json:"paymentProfileId"`
		} `json:"paymentProfile"`
	} `json:"profile"`
}

type CreateTransactionRequest struct {
	MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
	TransactionRequest     TransactionRequestType `json:"transactionRequest"`
}

type CreateTransactionResponse struct {
	TransactionResponse struct {
		TransId string `json:"transId"`
	} `json:"transactionResponse"`
	Messages struct {
		ResultCode string `json:"resultCode"`
		Message    []struct {
			Code string `json:"code"`
			Text string `json:"text"`
		} `json:"message"`
	} `json:"messages"`
}

func (c *APIClient) ChargeCustomerProfile(profileID, paymentProfileID, amount string) (string, error) {
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

	request := CreateTransactionRequest{
		MerchantAuthentication: c.Auth,
		TransactionRequest:     transactionRequest,
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

func (c *APIClient) UpdateCustomerProfile(profileID, email, description string) error {
	request := struct {
		MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
		Profile                struct {
			CustomerProfileId string `json:"customerProfileId"`
			Email             string `json:"email"`
			Description       string `json:"description"`
		} `json:"profile"`
	}{
		MerchantAuthentication: c.Auth,
		Profile: struct {
			CustomerProfileId string `json:"customerProfileId"`
			Email             string `json:"email"`
			Description       string `json:"description"`
		}{
			CustomerProfileId: profileID,
			Email:             email,
			Description:       description,
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
	if err := c.makeRequest(request, &response); err != nil {
		return err
	}
	if response.Messages.ResultCode != "OK" {
		if len(response.Messages.Message) > 0 {
			return fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return fmt.Errorf("API error: unknown error")
	}

	return nil
}

type ShippingAddress struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Address   string `json:"address"`
	City      string `json:"city"`
	State     string `json:"state"`
	Zip       string `json:"zip"`
	Country   string `json:"country"`
}

func (c *APIClient) AddShippingAddress(profileID string, address ShippingAddress) (string, error) {
	request := struct {
		MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
		CustomerProfileId      string                 `json:"customerProfileId"`
		Address                ShippingAddress        `json:"address"`
	}{
		MerchantAuthentication: c.Auth,
		CustomerProfileId:      profileID,
		Address:                address,
	}

	var response struct {
		CustomerAddressId string `json:"customerAddressId"`
		Messages          struct {
			ResultCode string `json:"resultCode"`
			Message    []struct {
				Code string `json:"code"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"messages"`
	}
	if err := c.makeRequest(request, &response); err != nil {
		return "", err
	}
	if response.Messages.ResultCode != "OK" {
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return "", fmt.Errorf("API error: unknown error")
	}

	return response.CustomerAddressId, nil
}

type CreditCard struct {
	CardNumber     string `json:"cardNumber"`
	ExpirationDate string `json:"expirationDate"`
}

func (c *APIClient) AddPaymentProfile(profileID string, creditCard CreditCard) (string, error) {
	request := struct {
		MerchantAuthentication MerchantAuthentication `json:"merchantAuthentication"`
		CustomerProfileId      string                 `json:"customerProfileId"`
		PaymentProfile         struct {
			Payment struct {
				CreditCard CreditCard `json:"creditCard"`
			} `json:"payment"`
		} `json:"paymentProfile"`
	}{
		MerchantAuthentication: c.Auth,
		CustomerProfileId:      profileID,
		PaymentProfile: struct {
			Payment struct {
				CreditCard CreditCard `json:"creditCard"`
			} `json:"payment"`
		}{
			Payment: struct {
				CreditCard CreditCard `json:"creditCard"`
			}{
				CreditCard: creditCard,
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
	if err := c.makeRequest(request, &response); err != nil {
		return "", err
	}
	if response.Messages.ResultCode != "OK" {
		if len(response.Messages.Message) > 0 {
			return "", fmt.Errorf("API error: %s", response.Messages.Message[0].Text)
		}
		return "", fmt.Errorf("API error: unknown error")
	}
	return response.CustomerPaymentProfileId, nil
}
