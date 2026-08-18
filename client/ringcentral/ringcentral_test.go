package ringcentral

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWT = "JWT-SECRET-DO-NOT-LOG"

// capturedRequest records what the client actually sent to the token endpoint,
// so tests can assert secrets travel where they should.
type capturedRequest struct {
	authHeader  string
	contentType string
	form        map[string]string
	path        string
}

func stubServer(t *testing.T, status int, body string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{form: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		captured.path = r.URL.Path
		captured.authHeader = r.Header.Get("Authorization")
		captured.contentType = r.Header.Get("Content-Type")
		for k := range r.PostForm {
			captured.form[k] = r.PostForm.Get(k)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func credFor(server string) Cred {
	return Cred{ClientID: "app-id", ClientSecret: "app-secret", JWT: testJWT, ServerURL: server}
}

// A well-formed credential authenticates through the JWT auth flow: the app
// credentials ride in the Basic header (never the body or URL) and the JWT is
// sent as the assertion.
func TestTestConnection_Success(t *testing.T) {
	srv, req := stubServer(t, http.StatusOK, `{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)

	c, err := NewClientWithCred(credFor(srv.URL))
	require.NoError(t, err)
	require.NoError(t, c.TestConnection(context.Background()))

	assert.Equal(t, "/restapi/oauth/token", req.path)
	assert.Equal(t, jwtGrantType, req.form["grant_type"])
	assert.Equal(t, testJWT, req.form["assertion"])
	assert.Equal(t, "application/x-www-form-urlencoded", req.contentType)

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("app-id:app-secret"))
	assert.Equal(t, want, req.authHeader, "app credentials must travel in the Basic header")
	assert.NotContains(t, req.form, "client_secret", "secret must not be in the form body")
}

// AC4 / expired-token edge case: RingCentral answers a revoked or expired JWT
// with 400 invalid_grant, which must read as "your credentials are wrong", not
// as a transport failure.
func TestTestConnection_RejectedCredentials(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"expired or revoked jwt", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Token is expired"}`},
		{"wrong client secret", http.StatusUnauthorized, `{"error":"invalid_client","error_description":"Invalid client"}`},
		{"app not permitted", http.StatusBadRequest, `{"error":"unauthorized_client","error_description":"Unauthorized for this grant type"}`},
		{"forbidden", http.StatusForbidden, `{"error":"insufficient_permissions"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := stubServer(t, tc.status, tc.body)
			c, err := NewClientWithCred(credFor(srv.URL))
			require.NoError(t, err)

			err = c.TestConnection(context.Background())
			assert.ErrorIs(t, err, ErrInvalidCredentials)
		})
	}
}

// A platform outage must NOT be reported as bad credentials — otherwise a
// RingCentral 500 would tell the tenant to reconnect a working integration.
func TestTestConnection_PlatformFailureIsNotCredentialFailure(t *testing.T) {
	srv, _ := stubServer(t, http.StatusInternalServerError, `{"message":"Internal error"}`)

	c, err := NewClientWithCred(credFor(srv.URL))
	require.NoError(t, err)

	err = c.TestConnection(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidCredentials)
	assert.Contains(t, err.Error(), "500")
}

// A 400 that is not an OAuth credential rejection (malformed request) also
// stays out of the credential bucket.
func TestTestConnection_BadRequestWithoutAuthCodeIsNotCredentialFailure(t *testing.T) {
	srv, _ := stubServer(t, http.StatusBadRequest, `{"error":"invalid_request","error_description":"Missing parameter"}`)

	c, err := NewClientWithCred(credFor(srv.URL))
	require.NoError(t, err)

	err = c.TestConnection(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidCredentials)
}

// Secrets must never reach an error string — errors get logged and shown.
func TestErrors_DoNotLeakSecrets(t *testing.T) {
	srv, _ := stubServer(t, http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Token is expired"}`)

	c, err := NewClientWithCred(credFor(srv.URL))
	require.NoError(t, err)

	_, _, err = c.AccessToken(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testJWT)
	assert.NotContains(t, err.Error(), "app-secret")
}

// A half-filled credential is refused before any network call.
func TestNewClientWithCred_Validation(t *testing.T) {
	cases := map[string]Cred{
		"no client id":     {ClientSecret: "s", JWT: "j"},
		"no client secret": {ClientID: "c", JWT: "j"},
		"no jwt":           {ClientID: "c", ClientSecret: "s"},
		"blank jwt":        {ClientID: "c", ClientSecret: "s", JWT: "   "},
	}
	for name, cred := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewClientWithCred(cred)
			require.Error(t, err)
		})
	}
}

// An empty server_url means production; a sandbox tenant keeps its own host.
func TestNewClientWithCred_ServerURLDefaulting(t *testing.T) {
	c, err := NewClientWithCred(Cred{ClientID: "c", ClientSecret: "s", JWT: "j"})
	require.NoError(t, err)
	assert.Equal(t, DefaultServerURL, c.serverURL)

	c, err = NewClientWithCred(Cred{ClientID: "c", ClientSecret: "s", JWT: "j", ServerURL: SandboxServerURL + "/"})
	require.NoError(t, err)
	assert.Equal(t, SandboxServerURL, c.serverURL, "trailing slash trimmed so paths do not double up")
}

// AccessToken returns the token and its lifetime for callers that will place
// calls or pull the call log (DEV-1753).
func TestAccessToken_ReturnsTokenAndTTL(t *testing.T) {
	srv, _ := stubServer(t, http.StatusOK, `{"access_token":"tok-123","expires_in":3600}`)

	c, err := NewClientWithCred(credFor(srv.URL))
	require.NoError(t, err)

	tok, ttl, err := c.AccessToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "tok-123", tok)
	assert.Equal(t, int64(3600), int64(ttl.Seconds()))
}

// A 200 with no token is a broken response, not a success.
func TestAccessToken_MissingTokenIsError(t *testing.T) {
	srv, _ := stubServer(t, http.StatusOK, `{"expires_in":3600}`)

	c, err := NewClientWithCred(credFor(srv.URL))
	require.NoError(t, err)

	_, _, err = c.AccessToken(context.Background())
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "access_token"))
}
