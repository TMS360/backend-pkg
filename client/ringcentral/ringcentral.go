// Package ringcentral is a thin client for the RingCentral REST API, scoped to
// what a per-company integration needs: exchange the tenant's server-side
// credentials for an access token and report whether they still work.
//
// Authentication uses the JWT auth flow — the only server-to-server flow
// RingCentral still offers (the password grant is retired). The company stores
// a client id/secret pair plus a JWT credential minted in the RingCentral
// developer console; POST /restapi/oauth/token exchanges them for a short-lived
// bearer token. A revoked or expired JWT surfaces as ErrInvalidCredentials,
// which is exactly the "sign in again" signal the Settings screen shows.
//
// The client mirrors the usps/here/samsara packages: thin Client,
// AuthError/IsAuthError for credential failures, errors wrapped with %w, no
// logger inside (the caller decides what to log). Secrets are never put in a
// URL, an error message, or a log line.
package ringcentral

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultServerURL is the RingCentral production platform host. A company on
	// the free developer sandbox overrides it with the devtest host.
	DefaultServerURL = "https://platform.ringcentral.com"
	// SandboxServerURL is the developer sandbox host, accepted so a tenant can
	// rehearse the integration before going live.
	SandboxServerURL = "https://platform.devtest.ringcentral.com"

	tokenPath = "/restapi/oauth/token"
	// jwtGrantType is RingCentral's JWT auth flow grant (RFC 7523).
	jwtGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

	defaultTimeout = 30 * time.Second
)

// ErrInvalidCredentials is returned when RingCentral rejects the company's
// credentials — a wrong client id/secret, or a JWT that was revoked or expired.
var ErrInvalidCredentials = errors.New("ringcentral: invalid credentials")

// ErrInsufficientPermissions means the credential authenticates perfectly well
// but the RingCentral app itself was never granted the scope the call needs
// (ReadCallLog, ReadCallRecording). It is a different problem from a revoked
// credential and has a different fix: the app must be re-approved in the
// RingCentral console with the scope added. Telling the tenant to "reconnect"
// sends them to rotate secrets that were never wrong.
//
// Only FetchRecording distinguishes it today. The shared get helper still maps
// 403 to ErrInvalidCredentials, because ListPhoneNumbers (DEV-1757) and the
// Settings screen behind it classify on that sentinel and would fall through to
// "RingCentral could not be reached" if a new one appeared underneath them.
// Widening that is a change to DEV-1751/DEV-1757's own surface — its own ticket.
var ErrInsufficientPermissions = errors.New("ringcentral: app is missing a required permission")

// AuthError carries the rejected response so the caller can log the reason
// (never the credential) when RingCentral refuses the token exchange.
type AuthError struct {
	StatusCode int
	// Code is RingCentral's OAuth error slug (invalid_grant, invalid_client, …).
	Code string
	Body string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("ringcentral auth failed (status %d, error=%s): %s", e.StatusCode, e.Code, e.Body)
}

// IsAuthError reports whether err (or any error it wraps) is an *AuthError.
func IsAuthError(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// Cred holds one company's RingCentral server-side credentials. Stored as a
// JSON object at {company_id}:setting:ringcentral_credentials and consumed via
// provider.JSONClientProvider, the same shape USPS uses.
type Cred struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// JWT is the credential minted in the RingCentral developer console for
	// this app + this RingCentral account. It is long-lived but revocable.
	JWT string `json:"jwt"`
	// ServerURL selects production or sandbox. Empty means production.
	ServerURL string `json:"server_url,omitempty"`
}

// Validate reports the first missing required field, so a save can be refused
// before anything is written.
func (c Cred) Validate() error {
	switch {
	case strings.TrimSpace(c.ClientID) == "":
		return errors.New("ringcentral: client_id is required")
	case strings.TrimSpace(c.ClientSecret) == "":
		return errors.New("ringcentral: client_secret is required")
	case strings.TrimSpace(c.JWT) == "":
		return errors.New("ringcentral: jwt is required")
	}
	return nil
}

// tokenResponse is the raw OAuth2 token payload.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	OwnerID     string `json:"owner_id"`
}

// errorResponse is the OAuth error payload RingCentral returns on a rejection.
type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Message          string `json:"message"`
}

// Client is a low-level RingCentral API client for one company.
type Client struct {
	httpClient *http.Client
	serverURL  string
	cred       Cred
}

// NewClientWithCred builds a Client for the given company credentials. The
// credential is validated up front so a half-filled integration fails fast
// instead of producing a confusing RingCentral error.
func NewClientWithCred(cred Cred) (*Client, error) {
	if err := cred.Validate(); err != nil {
		return nil, err
	}
	server := strings.TrimSpace(cred.ServerURL)
	if server == "" {
		server = DefaultServerURL
	}
	return &Client{
		httpClient: &http.Client{Timeout: defaultTimeout},
		serverURL:  strings.TrimRight(server, "/"),
		cred:       cred,
	}, nil
}

// AccessToken exchanges the stored credentials for a short-lived bearer token.
// Callers that place calls or pull the call log use this; nothing is cached
// here, because the credential can be revoked in the RingCentral console at any
// moment and a stale token would hide that.
func (c *Client) AccessToken(ctx context.Context) (string, time.Duration, error) {
	form := url.Values{}
	form.Set("grant_type", jwtGrantType)
	form.Set("assertion", c.cred.JWT)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+tokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("ringcentral: failed to create token request: %w", err)
	}
	// Client id/secret go in the Basic header, never in the URL or the body,
	// so they cannot leak through a proxy access log.
	basic := base64.StdEncoding.EncodeToString([]byte(c.cred.ClientID + ":" + c.cred.ClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("ringcentral: network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er errorResponse
		_ = json.Unmarshal(body, &er)
		if isAuthRejection(resp.StatusCode, er.Error) {
			return "", 0, &AuthError{StatusCode: resp.StatusCode, Code: er.Error, Body: describe(er, body)}
		}
		return "", 0, fmt.Errorf("ringcentral: token request failed with status %d: %s", resp.StatusCode, describe(er, body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", 0, fmt.Errorf("ringcentral: failed to decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", 0, errors.New("ringcentral: token response missing access_token")
	}

	ttl := time.Duration(tok.ExpiresIn) * time.Second
	return tok.AccessToken, ttl, nil
}

// TestConnection reports whether the stored credentials still authenticate.
// ErrInvalidCredentials means RingCentral rejected them (wrong app credentials,
// or a revoked/expired JWT) — the tenant has to reconnect. Any other error is a
// transport or platform problem and says nothing about the credential.
func (c *Client) TestConnection(ctx context.Context) error {
	_, _, err := c.AccessToken(ctx)
	if IsAuthError(err) {
		return ErrInvalidCredentials
	}
	return err
}

// isAuthRejection separates "your credentials are wrong" from "the request or
// the platform was wrong". RingCentral answers a revoked or expired JWT with
// 400 invalid_grant, not 401, so status alone is not enough.
func isAuthRejection(status int, code string) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	if status == http.StatusBadRequest {
		switch code {
		case "invalid_grant", "invalid_client", "unauthorized_client", "invalid_request_credentials":
			return true
		}
	}
	return false
}

// describe prefers RingCentral's own error text and falls back to the raw body,
// truncated so a surprise HTML error page cannot flood a log line.
func describe(er errorResponse, body []byte) string {
	switch {
	case er.ErrorDescription != "":
		return er.ErrorDescription
	case er.Message != "":
		return er.Message
	}
	const max = 256
	s := strings.TrimSpace(string(body))
	if len(s) > max {
		return s[:max]
	}
	return s
}
