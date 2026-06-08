// Package usps is a thin client for the modern USPS APIs (apis.usps.com).
//
// Authentication is OAuth2 client_credentials: the per-company Consumer Key /
// Consumer Secret (issued in the USPS developer portal) are exchanged at
// POST /oauth2/v3/token for a short-lived bearer token (~8h), which is then
// sent on every Addresses 3.0 call. Tokens are cached package-wide keyed by a
// hash of the Consumer Key, because the credential provider rebuilds the Client
// on every Get — an instance-level cache would be thrown away each call.
//
// The client mirrors the here/samsara packages: thin Client + Service wrapper,
// AuthError/IsAuthError for 401/403 detection, errors wrapped with %w, no
// logger inside (the caller decides what to log).
package usps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

const (
	defaultBaseURL = "https://apis.usps.com"

	// tokenPath is the OAuth2 client_credentials token endpoint.
	tokenPath = "/oauth2/v3/token"
	// addressPath is the Addresses 3.0 standardize/verify endpoint.
	addressPath = "/addresses/v3/address"

	// tokenRefreshSkew refreshes the cached token slightly before it expires so
	// an in-flight request never races the expiry.
	tokenRefreshSkew = 60 * time.Second
	// defaultTokenTTL is used when the token response omits expires_in.
	defaultTokenTTL = 8 * time.Hour
)

// ErrInvalidCredentials is returned when USPS rejects the Consumer Key/Secret
// with a 401 or 403.
var ErrInvalidCredentials = errors.New("usps: invalid credentials")

// AuthError is returned by the client when USPS responds with 401 or 403.
// Callers use IsAuthError to detect this case and deactivate the integration.
type AuthError struct {
	StatusCode int
	Body       string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("usps auth failed (status %d): %s", e.StatusCode, e.Body)
}

// IsAuthError reports whether err (or any error it wraps) is a *AuthError.
func IsAuthError(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// Cred holds the per-company OAuth2 client credentials. Stored in Redis as a
// JSON object at {company_id}:setting:usps_credentials and consumed via
// provider.JSONClientProvider.
type Cred struct {
	ConsumerKey    string `json:"consumer_key"`
	ConsumerSecret string `json:"consumer_secret"`
}

// AddressRequest is the input to VerifyAddress. Only the fields USPS needs for
// standardization are sent; empty fields are omitted from the query.
type AddressRequest struct {
	StreetAddress    string
	SecondaryAddress string
	City             string
	State            string
	ZIPCode          string
	ZIPPlus4         string
}

// StandardizedAddress is the USPS-corrected address returned by the API.
type StandardizedAddress struct {
	StreetAddress    string `json:"streetAddress"`
	SecondaryAddress string `json:"secondaryAddress"`
	City             string `json:"city"`
	State            string `json:"state"`
	ZIPCode          string `json:"ZIPCode"`
	ZIPPlus4         string `json:"ZIPPlus4"`
}

// AddressResult is the verification outcome. Verified is true only on an exact,
// deliverable single match (DPVConfirmation == "Y").
type AddressResult struct {
	Verified     bool
	DPV          string
	Standardized StandardizedAddress
}

// addressResponse is the raw Addresses 3.0 payload.
type addressResponse struct {
	Firm           string              `json:"firm"`
	Address        StandardizedAddress `json:"address"`
	AdditionalInfo struct {
		DPVConfirmation string `json:"DPVConfirmation"`
		Vacant          string `json:"vacant"`
		Business        string `json:"business"`
	} `json:"additionalInfo"`
}

// tokenResponse is the raw OAuth2 token payload.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Client is a low-level USPS API client.
type Client struct {
	httpClient *http.Client
	baseURL    string
	oauthURL   string
	cred       Cred
}

// NewClient builds a Client from config defaults + per-company credentials.
func NewClient(cfg config.UspsConfig, cred Cred) (*Client, error) {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	oauth := cfg.OAuthHost
	if oauth == "" {
		oauth = base
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(base, "/"),
		oauthURL:   strings.TrimRight(oauth, "/"),
		cred:       cred,
	}, nil
}

// NewClientWithCred builds a Client with the production hosts and the given
// credentials. Convenience constructor for the provider factory.
func NewClientWithCred(cred Cred) (*Client, error) {
	return NewClient(config.UspsConfig{}, cred)
}

// VerifyAddress standardizes and verifies a single US address. A nil error with
// Verified=false means USPS could not confirm an exact deliverable match.
func (c *Client) VerifyAddress(ctx context.Context, req AddressRequest) (result *AddressResult, err error) {
	q := url.Values{}
	q.Set("streetAddress", req.StreetAddress)
	if req.SecondaryAddress != "" {
		q.Set("secondaryAddress", req.SecondaryAddress)
	}
	if req.City != "" {
		q.Set("city", req.City)
	}
	if req.State != "" {
		q.Set("state", req.State)
	}
	if req.ZIPCode != "" {
		q.Set("ZIPCode", req.ZIPCode)
	}
	if req.ZIPPlus4 != "" {
		q.Set("ZIPPlus4", req.ZIPPlus4)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, addressPath, q, true)
	if err != nil {
		return nil, fmt.Errorf("failed to verify address: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var raw addressResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode address response: %w", err)
	}

	return &AddressResult{
		Verified:     raw.AdditionalInfo.DPVConfirmation == "Y",
		DPV:          raw.AdditionalInfo.DPVConfirmation,
		Standardized: raw.Address,
	}, nil
}

// doRequest performs a bearer-authenticated request. On 401/403 it invalidates
// the cached token and retries once (covering an expired/revoked token), then
// surfaces an *AuthError.
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, retryOnAuth bool) (*http.Response, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}

	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if retryOnAuth {
			c.invalidateToken()
			return c.doRequest(ctx, method, path, query, false)
		}
		return nil, &AuthError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("usps request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// ---- OAuth2 token cache ----------------------------------------------------

// cachedToken holds a token and its expiry behind a mutex. The mutex is held
// across the network fetch so concurrent callers collapse onto a single
// refresh (singleflight) instead of stampeding the token endpoint.
type cachedToken struct {
	mu        sync.Mutex
	value     string
	expiresAt time.Time
}

var (
	tokenStoreMu sync.Mutex
	tokenStore   = map[string]*cachedToken{}
)

// credKey is the per-credential cache key. Hashing keeps the raw secret out of
// the in-memory map keys.
func (c *Client) credKey() string {
	sum := sha256.Sum256([]byte(c.cred.ConsumerKey + ":" + c.cred.ConsumerSecret))
	return hex.EncodeToString(sum[:])
}

func (c *Client) tokenSlot() *cachedToken {
	key := c.credKey()
	tokenStoreMu.Lock()
	defer tokenStoreMu.Unlock()
	ct := tokenStore[key]
	if ct == nil {
		ct = &cachedToken{}
		tokenStore[key] = ct
	}
	return ct
}

// token returns a valid bearer token, refreshing it if absent or near expiry.
func (c *Client) token(ctx context.Context) (string, error) {
	ct := c.tokenSlot()
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.value != "" && time.Until(ct.expiresAt) > tokenRefreshSkew {
		return ct.value, nil
	}

	value, ttl, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	ct.value = value
	ct.expiresAt = time.Now().Add(ttl)
	return ct.value, nil
}

// invalidateToken clears the cached token so the next call refetches.
func (c *Client) invalidateToken() {
	ct := c.tokenSlot()
	ct.mu.Lock()
	ct.value = ""
	ct.mu.Unlock()
}

func (c *Client) fetchToken(ctx context.Context) (string, time.Duration, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.cred.ConsumerKey)
	form.Set("client_secret", c.cred.ConsumerSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL+tokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to fetch token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, &AuthError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("usps token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", 0, fmt.Errorf("failed to decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", 0, fmt.Errorf("usps token response missing access_token")
	}

	ttl := defaultTokenTTL
	if tok.ExpiresIn > 0 {
		ttl = time.Duration(tok.ExpiresIn) * time.Second
	}
	return tok.AccessToken, ttl, nil
}
