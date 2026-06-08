package usps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uspsTestServer mounts the OAuth token endpoint and the addresses endpoint.
// tokenHits counts how many times a fresh token was minted so tests can assert
// the package-level cache collapses repeated calls.
type uspsTestServer struct {
	srv       *httptest.Server
	tokenHits int32
}

func newTestServer(t *testing.T, addrStatus int, addrBody string) *uspsTestServer {
	t.Helper()
	ts := &uspsTestServer{}
	mux := http.NewServeMux()
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&ts.tokenHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-123","token_type":"Bearer","expires_in":28799}`))
	})
	mux.HandleFunc(addressPath, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			http.Error(w, "missing bearer", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(addrStatus)
		_, _ = w.Write([]byte(addrBody))
	})
	ts.srv = httptest.NewServer(mux)
	t.Cleanup(ts.srv.Close)
	return ts
}

// newClient wires a Client at the test server with unique credentials so the
// package-level token cache never bleeds between tests.
func (ts *uspsTestServer) newClient(consumerKey string) *Client {
	return &Client{
		httpClient: ts.srv.Client(),
		baseURL:    ts.srv.URL,
		oauthURL:   ts.srv.URL,
		cred:       Cred{ConsumerKey: consumerKey, ConsumerSecret: "secret"},
	}
}

func TestVerifyAddress_Verified(t *testing.T) {
	body := `{"address":{"streetAddress":"475 LENFANT PLZ SW","city":"WASHINGTON","state":"DC","ZIPCode":"20260","ZIPPlus4":"0004"},"additionalInfo":{"DPVConfirmation":"Y"}}`
	ts := newTestServer(t, http.StatusOK, body)
	c := ts.newClient("verified-key")

	res, err := c.VerifyAddress(context.Background(), AddressRequest{
		StreetAddress: "475 LEnfant Plaza SW",
		City:          "Washington",
		State:         "DC",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Verified)
	assert.Equal(t, "Y", res.DPV)
	assert.Equal(t, "475 LENFANT PLZ SW", res.Standardized.StreetAddress)
	assert.Equal(t, "20260", res.Standardized.ZIPCode)
	assert.Equal(t, "0004", res.Standardized.ZIPPlus4)
}

func TestVerifyAddress_NotVerified(t *testing.T) {
	body := `{"address":{"streetAddress":"123 NOWHERE ST","city":"NOWHERE","state":"XX"},"additionalInfo":{"DPVConfirmation":"N"}}`
	ts := newTestServer(t, http.StatusOK, body)
	c := ts.newClient("notverified-key")

	res, err := c.VerifyAddress(context.Background(), AddressRequest{StreetAddress: "123 Nowhere St"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Verified)
	assert.Equal(t, "N", res.DPV)
}

func TestVerifyAddress_TokenCached(t *testing.T) {
	body := `{"address":{"streetAddress":"1 MAIN ST"},"additionalInfo":{"DPVConfirmation":"Y"}}`
	ts := newTestServer(t, http.StatusOK, body)
	c := ts.newClient("cached-key")

	for i := 0; i < 3; i++ {
		_, err := c.VerifyAddress(context.Background(), AddressRequest{StreetAddress: "1 Main St"})
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&ts.tokenHits), "token should be fetched once and cached")
}

func TestVerifyAddress_AuthErrorRetriesOnce(t *testing.T) {
	// Address endpoint always 401 → client invalidates token, retries once,
	// then surfaces an *AuthError.
	ts := newTestServer(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)
	c := ts.newClient("auth-key")

	_, err := c.VerifyAddress(context.Background(), AddressRequest{StreetAddress: "1 Main St"})
	require.Error(t, err)
	assert.True(t, IsAuthError(err), "expected an AuthError, got %v", err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&ts.tokenHits), "token fetched once, then re-fetched on retry")
}

func TestVerifyAddress_ServerErrorNoPanic(t *testing.T) {
	ts := newTestServer(t, http.StatusInternalServerError, `boom`)
	c := ts.newClient("err-key")

	res, err := c.VerifyAddress(context.Background(), AddressRequest{StreetAddress: "1 Main St"})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.False(t, IsAuthError(err))
}

func TestService_VerifyUSAddress_ValidatesInput(t *testing.T) {
	ts := newTestServer(t, http.StatusOK, `{"additionalInfo":{"DPVConfirmation":"Y"}}`)
	svc := NewService(ts.newClient("svc-key"))

	_, err := svc.VerifyUSAddress(context.Background(), AddressRequest{})
	require.Error(t, err, "empty streetAddress must be rejected before any network call")
	assert.Equal(t, int32(0), atomic.LoadInt32(&ts.tokenHits))
}
