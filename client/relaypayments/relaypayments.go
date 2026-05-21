// Package relaypayments is an HTTP client for the Relay Payments TMS Fuel API
// (https://app.relaypayments.com/api/integrations).
//
// Each TMS company uses its own Relay account, so the API key is passed in by
// the caller (typically through client/provider.ClientProvider, which fetches
// it from Redis at {company_id}:setting:relay_api_key).
//
//	relayProvider := provider.New("relay_api_key", relaypayments.NewClientWithToken)
//	client, err := relayProvider.Get(ctx)
package relaypayments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

const defaultProductionHost = "https://app.relaypayments.com/api/integrations"

// ErrInvalidCredentials is returned when Relay rejects the configured API key
// with a 401 or 403 status.
var ErrInvalidCredentials = errors.New("relaypayments: invalid credentials")

// AuthError is returned by doRequest when Relay responds with 401 or 403.
// Callers use IsAuthError to detect this case and disable the integration for
// the affected company.
type AuthError struct {
	StatusCode int
	Body       string
	RequestID  string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("relaypayments auth failed (status %d, request_id=%s): %s", e.StatusCode, e.RequestID, e.Body)
}

// IsAuthError reports whether err (or any error it wraps) is a *AuthError.
func IsAuthError(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// Client is a thin HTTP client for the Relay Payments integrations API.
type Client struct {
	httpClient *http.Client
	host       string
	apiKey     string
}

// NewClient builds a Relay Payments client. cfg.Host overrides the default
// production base URL ("https://app.relaypayments.com/api/integrations"); set
// it to "https://staging.relaypayments.com/api/integrations" for QA.
func NewClient(cfg config.RelayConfig, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("relaypayments: apiKey is empty")
	}
	host := strings.TrimRight(cfg.Host, "/")
	if host == "" {
		host = defaultProductionHost
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		host:       host,
		apiKey:     apiKey,
	}, nil
}

// NewClientWithToken is a convenience constructor used with the per-company
// provider.ClientProvider — base URL falls back to production.
func NewClientWithToken(apiKey string) (*Client, error) {
	return NewClient(config.RelayConfig{}, apiKey)
}

// TestConnection validates the API key by making a lightweight authenticated
// request (GET /drivers/?limit=1). Returns nil on success, ErrInvalidCredentials
// on 401/403, or a wrapped error otherwise.
func (c *Client) TestConnection(ctx context.Context) error {
	q := url.Values{}
	q.Set("limit", "1")
	resp, err := c.doRequest(ctx, http.MethodGet, "/drivers/", q, nil)
	if err != nil {
		if IsAuthError(err) {
			return ErrInvalidCredentials
		}
		return err
	}
	defer resp.Body.Close()
	return nil
}

// doRequest performs an HTTP request with Relay's ApiKeyAuth scheme.
// The Relay OpenAPI defines auth as `Authorization: <api_key>` (no Bearer prefix).
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("relaypayments: marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	endpoint := c.host + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("relaypayments: build request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("relaypayments: execute request: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &AuthError{
			StatusCode: resp.StatusCode,
			Body:       string(bodyBytes),
			RequestID:  resp.Header.Get("X-Request-Id"),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, decodeAPIError(resp.StatusCode, bodyBytes)
	}

	return resp, nil
}

// decodeJSON reads resp.Body into out, ensuring the body is always closed.
// out may be nil for endpoints that don't return a body (e.g. 200 No Content).
func decodeJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("relaypayments: decode response: %w", err)
	}
	return nil
}

func decodeAPIError(status int, body []byte) error {
	var apiErr Error
	if len(body) > 0 && json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("relaypayments: status %d: %s", status, apiErr.Message)
	}
	return fmt.Errorf("relaypayments: status %d: %s", status, string(body))
}

// Error is the generic error envelope returned by Relay.
type Error struct {
	Message string `json:"message"`
}

// Driver is a Relay driver record. Used by both create/update requests and
// list/get responses.
type Driver struct {
	ID            string            `json:"id,omitempty"`
	FirstName     string            `json:"first_name"`
	LastName      string            `json:"last_name"`
	Phone         string            `json:"phone"`
	Email         string            `json:"email,omitempty"`
	DataFields    []DriverDataField `json:"data_fields,omitempty"`
	IntegrationID string            `json:"integration_id,omitempty"`
	CreatedAt     string            `json:"created_at,omitempty"`
	UpdatedAt     string            `json:"updated_at,omitempty"`
}

// DriverDataField is a configurable per-driver attribute (e.g. truck number).
// `field_name` must match a field configured in Relay or it is ignored.
type DriverDataField struct {
	FieldName  string `json:"field_name"`
	FieldValue string `json:"field_value"`
}

// FuelTransactionDriver is the driver subset embedded in Transaction.
type FuelTransactionDriver struct {
	ID            string `json:"id"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Phone         string `json:"phone"`
	Email         string `json:"email,omitempty"`
	IntegrationID string `json:"integration_id,omitempty"`
}

// OneTimeCode is a single-use fuel or cash code.
type OneTimeCode struct {
	ID                                string `json:"id,omitempty"`
	Code                              string `json:"code,omitempty"`
	ConstraintAmount                  string `json:"constraint_amount"`
	Type                              string `json:"type"` // "fuel" | "cash"
	DriverID                          string `json:"driver_id,omitempty"`
	Note                              string `json:"note,omitempty"`
	LocationID                        string `json:"location_id,omitempty"`
	AllowProhibitedLocationRedemption bool   `json:"allow_prohibited_location_redemption,omitempty"`
	CreatedAt                         string `json:"created_at,omitempty"`
	UpdatedAt                         string `json:"updated_at,omitempty"`
	DeletedAt                         string `json:"deleted_at,omitempty"`
}

// FuelPolicy defines fueling limits assignable to drivers. Read-only via API.
type FuelPolicy struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	ELDEnabled           bool                  `json:"eld_enabled,omitempty"`
	AuthorizationPrompts []AuthorizationPrompt `json:"authorization_prompts,omitempty"`
	Limits               []FuelPolicyLimit     `json:"limits"`
}

// AuthorizationPrompt is a driver prompt required to unlock a fuel code.
type AuthorizationPrompt struct {
	Name                string `json:"name"`
	ValidationMode      string `json:"validation_mode"`
	ELDIntegrationField string `json:"eld_integration_fiel,omitempty"` // typo preserved from spec
}

// FuelPolicyLimit constrains how much of a product/service can be purchased.
type FuelPolicyLimit struct {
	Type      string `json:"type"`
	Mode      string `json:"mode"`
	Amount    string `json:"amount"`
	Unit      string `json:"unit"`
	Frequency string `json:"frequency"`
}

// DriverPolicyAssignment binds a driver to a fuel policy.
type DriverPolicyAssignment struct {
	ID       string `json:"id,omitempty"`
	DriverID string `json:"driver_id"`
	PolicyID string `json:"policy_id"`
}

// UpdatePolicyAssignment is the PUT body for /fuel/policies/policy-assignments/{id}.
type UpdatePolicyAssignment struct {
	Enabled bool `json:"enabled"`
}

// PolicyAssignmentFuelPolicy is the policy assignment response, including
// remaining usage on each limit.
type PolicyAssignmentFuelPolicy struct {
	ID         string                            `json:"id"`
	PolicyID   string                            `json:"policy_id"`
	PolicyName string                            `json:"policy_name"`
	Enabled    bool                              `json:"enabled"`
	Limits     []PolicyAssignmentFuelPolicyLimit `json:"limits"`
}

// PolicyAssignmentFuelPolicyLimit extends FuelPolicyLimit with usage data.
type PolicyAssignmentFuelPolicyLimit struct {
	Type      string `json:"type"`
	Mode      string `json:"mode"`
	Amount    string `json:"amount"`
	Unit      string `json:"unit"`
	Frequency string `json:"frequency"`
	Usage     string `json:"usage"`
	Remaining string `json:"remaining"`
}

// Merchant identifies the fuel merchant of a transaction.
type Merchant struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number string `json:"number"`
}

// Location is a fuel merchant's physical location.
type Location struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name"`
	FuelMerchantLocationID string  `json:"fuel_merchant_location_id"`
	Address                string  `json:"address"`
	City                   string  `json:"city"`
	State                  string  `json:"state"`
	ZipCode                string  `json:"zip_code"`
	Latitude               float64 `json:"latitude"`
	Longitude              float64 `json:"longitude"`
	OPISID                 string  `json:"opis_id,omitempty"`
	Timezone               string  `json:"timezone"`
}

// Prompt is a driver-entered prompt value captured at fueling time.
type Prompt struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// FuelItem is a single fuel product purchased on a transaction.
type FuelItem struct {
	FuelType               string `json:"fuel_type"`
	FuelTypeDescription    string `json:"fuel_type_description"`
	FuelProductCode        string `json:"fuel_product_code"`
	RetailPricePerUnit     string `json:"retail_price_per_unit"`
	DiscountedPricePerUnit string `json:"discounted_price_per_unit"`
	Volume                 string `json:"volume"`
	VolumeUOM              string `json:"volume_uom"`
	TotalRetailPrice       string `json:"total_retail_price"`
	TotalDiscountedPrice   string `json:"total_discounted_price"`
	Fees                   []Fee  `json:"fees,omitempty"`
}

// Product is a non-fuel item purchased on a transaction.
type Product struct {
	ProductType            string `json:"product_type"`
	ProductTypeDescription string `json:"product_type_description,omitempty"`
	PurchasePriceTotal     string `json:"purchase_price_total"`
	Quantity               string `json:"quantity"`
	PricePerUnit           string `json:"price_per_unit,omitempty"`
	Fees                   []Fee  `json:"fees,omitempty"`
}

// Fee is a transaction or per-line fee.
type Fee struct {
	Type   string `json:"type"`
	Amount string `json:"amount"`
}

// TransactionFuelPolicy is the policy summary embedded in a Transaction.
type TransactionFuelPolicy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Transaction is a single fuel transaction reported by Relay.
type Transaction struct {
	TransactionID    string                 `json:"transaction_id"`
	CreatedAt        time.Time              `json:"created_at"`
	RelayFuelCode    string                 `json:"relay_fuel_code,omitempty"`
	TotalAmountPaid  string                 `json:"total_amount_paid"`
	TotalRetailPrice string                 `json:"total_retail_price"`
	TotalAmountSaved string                 `json:"total_amount_saved"`
	IsDirectBill     bool                   `json:"is_direct_bill"`
	CurrencyCode     string                 `json:"currency_code"`
	CashAdvance      string                 `json:"cash_advance,omitempty"`
	Driver           FuelTransactionDriver  `json:"driver"`
	Merchant         Merchant               `json:"merchant"`
	Location         Location               `json:"location"`
	Prompts          []Prompt               `json:"prompts,omitempty"`
	FuelItems        []FuelItem             `json:"fuel_items,omitempty"`
	Products         []Product              `json:"products,omitempty"`
	Fees             []Fee                  `json:"fees,omitempty"`
	FuelPolicy       *TransactionFuelPolicy `json:"fuel_policy,omitempty"`
	FuelCodeType     string                 `json:"fuel_code_type,omitempty"` // "policy" | "one_time"
}
