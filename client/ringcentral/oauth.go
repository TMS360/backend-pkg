package ringcentral

// Per-user authorization (DEV-2111).
//
// Everything else in this package authenticates as ONE credential belonging to
// one RingCentral extension: the company pastes a JWT, and every call the whole
// tenant makes is that person. DEV-1895 proved with live requests that this can
// never text from anybody else's number — MSG-304 for a foreign "from", CMN-419
// even for an account admin reaching into the owner's extension, with the
// needed permission marked assignable:false so it cannot be granted.
//
// So a dispatcher who is to text from THEIR number needs THEIR OWN credential.
// The authorization-code flow is how RingCentral hands one out without the
// person ever seeing a client secret or opening the developer console: our app
// (one client id/secret for the whole platform) sends them to RingCentral, they
// log in, and RingCentral hands back a code we exchange for a token pair.
//
// Two properties of that pair drive every design decision downstream:
//
//   - the access token lives ~1 hour, so it is refreshed, never stored as "the"
//     credential;
//   - the refresh token is SLIDING and ROTATES — each refresh returns a new one
//     and invalidates the old. Losing the new value logs the user out, which is
//     why the caller must persist a refresh atomically.
//
// This file is transport only: build the URL, exchange, refresh, revoke. Where
// the tokens live, how they are encrypted and who may use them is the service's
// business.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	authorizePath = "/restapi/oauth/authorize"
	revokePath    = "/restapi/oauth/revoke"

	authCodeGrantType = "authorization_code"
	refreshGrantType  = "refresh_token"

	// pkceMethod is the only challenge method worth using; "plain" defeats the
	// point of PKCE.
	pkceMethod = "S256"
)

// ErrReauthRequired means RingCentral will not renew this user's access: the
// refresh token expired, was already rotated away, or the person revoked our
// app in their own RingCentral settings.
//
// It is deliberately distinct from ErrInvalidCredentials, which is a statement
// about the COMPANY credential. They lead to different screens: one asks a
// dispatcher to press "Connect RingCentral" again, the other sends an admin to
// Settings → Integrations to fix something for the whole tenant.
var ErrReauthRequired = errors.New("ringcentral: the user must connect RingCentral again")

// AppCred is our application's own identity — the same client id/secret for
// every tenant, because the app is one app. It says nothing about who is
// logging in; that is what the authorization code establishes.
type AppCred struct {
	ClientID     string
	ClientSecret string
	// ServerURL selects production or sandbox. Empty means production.
	ServerURL string
}

// Validate reports the first missing field so a misconfigured deployment fails
// at startup rather than at the moment a dispatcher presses Connect.
func (a AppCred) Validate() error {
	switch {
	case strings.TrimSpace(a.ClientID) == "":
		return errors.New("ringcentral: oauth client_id is required")
	case strings.TrimSpace(a.ClientSecret) == "":
		return errors.New("ringcentral: oauth client_secret is required")
	}
	return nil
}

// OAuthClient performs the authorization-code flow for one application.
//
// It is separate from Client on purpose: Client is "a credential talking to the
// API", while this is "an application obtaining a credential". Merging them
// would mean a Client that exists before it has anything to authenticate with.
type OAuthClient struct {
	httpClient *http.Client
	serverURL  string
	app        AppCred
}

func NewOAuthClient(app AppCred) (*OAuthClient, error) {
	if err := app.Validate(); err != nil {
		return nil, err
	}
	server := strings.TrimSpace(app.ServerURL)
	if server == "" {
		server = DefaultServerURL
	}
	return &OAuthClient{
		httpClient: &http.Client{Timeout: defaultTimeout},
		serverURL:  strings.TrimRight(server, "/"),
		app:        app,
	}, nil
}

// ServerURL is the platform host this client talks to. Callers store it with
// the resulting tokens so a later refresh cannot be pointed at another host.
func (c *OAuthClient) ServerURL() string { return c.serverURL }

// AuthorizeParams is what the browser redirect needs.
type AuthorizeParams struct {
	// RedirectURI must match one registered on the app, character for
	// character — RingCentral compares it literally.
	RedirectURI string
	// State is echoed back untouched. It is the only thing tying the callback to
	// the person who started it, so it must be unguessable and verified.
	State string
	// CodeChallenge is the S256 hash of the verifier kept on our side.
	CodeChallenge string
	// Scopes is optional. Leaving it empty asks for the app's full configured
	// set, which is what we want: the scope list lives in the RingCentral
	// console (DEV-2110), not scattered across callers.
	Scopes []string
}

// AuthorizeURL builds the address the user's browser opens. No secret goes into
// it: the client secret is used only on the back-channel token exchange.
func (c *OAuthClient) AuthorizeURL(p AuthorizeParams) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.app.ClientID)
	q.Set("redirect_uri", p.RedirectURI)
	if p.State != "" {
		q.Set("state", p.State)
	}
	if p.CodeChallenge != "" {
		q.Set("code_challenge", p.CodeChallenge)
		q.Set("code_challenge_method", pkceMethod)
	}
	if len(p.Scopes) > 0 {
		q.Set("scope", strings.Join(p.Scopes, " "))
	}
	return c.serverURL + authorizePath + "?" + q.Encode()
}

// UserToken is one person's access to their own RingCentral.
type UserToken struct {
	AccessToken  string
	RefreshToken string
	// ExpiresIn / RefreshExpiresIn are durations, not deadlines: the caller
	// stamps them against its own clock, which is the clock the expiry will be
	// compared with later.
	ExpiresIn        time.Duration
	RefreshExpiresIn time.Duration
	// OwnerID is the extension the token belongs to — the answer to "who is
	// this", read from the token itself rather than from anything the user
	// typed.
	OwnerID string
	// AccountID identifies the RingCentral account (the tenant's phone system).
	// It is what makes "this person connected a personal account, not the
	// company's" a decidable question.
	AccountID string
	Scope     string
}

// ExchangeParams carries the callback's half of the flow.
type ExchangeParams struct {
	Code string
	// RedirectURI must be byte-identical to the one used on the authorize call.
	RedirectURI string
	// CodeVerifier is the secret behind the challenge, never sent to the
	// browser.
	CodeVerifier string
}

// ExchangeCode turns the one-time code from the callback into a token pair.
//
// A rejection here is ErrReauthRequired rather than ErrInvalidCredentials: the
// code is single-use and short-lived, so "invalid_grant" almost always means a
// stale or replayed callback, and the cure is to start the flow again.
func (c *OAuthClient) ExchangeCode(ctx context.Context, p ExchangeParams) (*UserToken, error) {
	if strings.TrimSpace(p.Code) == "" {
		return nil, errors.New("ringcentral: authorization code is required")
	}
	form := url.Values{}
	form.Set("grant_type", authCodeGrantType)
	form.Set("code", p.Code)
	form.Set("redirect_uri", p.RedirectURI)
	if p.CodeVerifier != "" {
		form.Set("code_verifier", p.CodeVerifier)
	}
	return c.tokenCall(ctx, form)
}

// Refresh renews a user's access and ROTATES the refresh token: the value that
// comes back replaces the one sent, and the old one stops working immediately.
// A caller that fails to persist the new value has logged the user out.
func (c *OAuthClient) Refresh(ctx context.Context, refreshToken string) (*UserToken, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, ErrReauthRequired
	}
	form := url.Values{}
	form.Set("grant_type", refreshGrantType)
	form.Set("refresh_token", refreshToken)
	return c.tokenCall(ctx, form)
}

// Revoke invalidates a token (either half of the pair) at RingCentral, so
// disconnecting in TMS actually ends the access rather than only forgetting it.
//
// A token RingCentral no longer knows is reported as success: the caller's goal
// is "this access is gone", and it is.
func (c *OAuthClient) Revoke(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	form := url.Values{}
	form.Set("token", token)

	_, err := c.formPost(ctx, revokePath, form)
	if IsAuthError(err) {
		return nil
	}
	return err
}

// tokenCall performs a token-endpoint request and decodes the pair.
func (c *OAuthClient) tokenCall(ctx context.Context, form url.Values) (*UserToken, error) {
	body, err := c.formPost(ctx, tokenPath, form)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrReauthRequired
		}
		return nil, err
	}

	var decoded struct {
		AccessToken           string `json:"access_token"`
		RefreshToken          string `json:"refresh_token"`
		ExpiresIn             int    `json:"expires_in"`
		RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
		OwnerID               string `json:"owner_id"`
		Scope                 string `json:"scope"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to decode token response: %w", err)
	}
	if decoded.AccessToken == "" {
		return nil, errors.New("ringcentral: token response missing access_token")
	}

	return &UserToken{
		AccessToken:      decoded.AccessToken,
		RefreshToken:     decoded.RefreshToken,
		ExpiresIn:        time.Duration(decoded.ExpiresIn) * time.Second,
		RefreshExpiresIn: time.Duration(decoded.RefreshTokenExpiresIn) * time.Second,
		OwnerID:          decoded.OwnerID,
		Scope:            decoded.Scope,
	}, nil
}

// formPost is the shared back-channel call: form body, Basic app credentials,
// never a secret in the URL.
func (c *OAuthClient) formPost(ctx context.Context, path string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to create oauth request: %w", err)
	}
	basic := base64.StdEncoding.EncodeToString([]byte(c.app.ClientID + ":" + c.app.ClientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er errorResponse
		_ = json.Unmarshal(body, &er)
		if isAuthRejection(resp.StatusCode, er.Error) {
			return nil, &AuthError{StatusCode: resp.StatusCode, Code: er.Error, Body: describe(er, body)}
		}
		return nil, fmt.Errorf("ringcentral: oauth request failed with status %d: %s", resp.StatusCode, describe(er, body))
	}
	return body, nil
}

// pkceVerifierBytes is 32 random bytes — 43 base64url characters, the length
// RFC 7636 recommends and well inside its 43..128 range.
const pkceVerifierBytes = 32

// NewPKCE mints a verifier and its S256 challenge.
//
// We are a confidential client (the secret never leaves the server), so PKCE is
// not strictly required here. It is used anyway because it costs one hash and
// closes the one hole a confidential client still has: an authorization code
// stolen out of the redirect — from a browser history, a proxy log, a referrer
// header — is useless without the verifier, which never left this process.
func NewPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, pkceVerifierBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("ringcentral: failed to generate a pkce verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	return verifier, PKCEChallenge(verifier), nil
}

// PKCEChallenge is the S256 transformation RingCentral verifies the code
// against.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
