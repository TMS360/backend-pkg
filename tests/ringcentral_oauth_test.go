package tests

// DEV-2111 — the per-user authorization half of the RingCentral client: the
// browser redirect, the code exchange, the sliding refresh, the revoke, and a
// Client that authenticates as one person's own access token.
//
// Every test here is about a rule that, broken, silently disconnects a person:
// a rotated refresh token that is not returned, an auth rejection reported as
// an outage, or a bearer client that quietly falls back to the company
// credential and texts from the wrong number.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/client/ringcentral"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func oauthApp(server string) ringcentral.AppCred {
	return ringcentral.AppCred{ClientID: "app-id", ClientSecret: "app-secret", ServerURL: server}
}

// oauthServer records what reached the token endpoint so a test can assert on
// the wire form, which is where these bugs live.
type oauthCapture struct {
	paths   []string
	forms   []url.Values
	authHdr []string
}

func oauthServer(t *testing.T, status int, body string) (*httptest.Server, *oauthCapture) {
	t.Helper()
	cap := &oauthCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		cap.paths = append(cap.paths, r.URL.Path)
		cap.forms = append(cap.forms, form)
		cap.authHdr = append(cap.authHdr, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

const oauthTokenPair = `{"access_token":"access-1","refresh_token":"refresh-1",
	"expires_in":3600,"refresh_token_expires_in":604800,
	"owner_id":"4242","scope":"SMS ReadAccounts"}`

func TestAuthorizeURL_CarriesTheChallengeAndNeverTheSecret(t *testing.T) {
	client, err := ringcentral.NewOAuthClient(oauthApp("https://platform.example.com"))
	require.NoError(t, err)

	verifier, challenge, err := ringcentral.NewPKCE()
	require.NoError(t, err)

	raw := client.AuthorizeURL(ringcentral.AuthorizeParams{
		RedirectURI:   "https://tms.example.com/api/ringcentral/oauth/callback",
		State:         "signed-state",
		CodeChallenge: challenge,
	})

	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	q := parsed.Query()

	assert.Equal(t, "/restapi/oauth/authorize", parsed.Path)
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "app-id", q.Get("client_id"))
	assert.Equal(t, "https://tms.example.com/api/ringcentral/oauth/callback", q.Get("redirect_uri"))
	assert.Equal(t, "signed-state", q.Get("state"))
	assert.Equal(t, challenge, q.Get("code_challenge"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"),
		"plain would make PKCE decorative — the challenge must be the hash")

	assert.NotContains(t, raw, "app-secret",
		"the client secret goes only on the back channel; this URL is handed to a browser")
	assert.NotContains(t, raw, verifier,
		"the verifier must never leave the server — that is the whole point of it")
}

func TestPKCEChallenge_IsTheS256OfTheVerifier(t *testing.T) {
	verifier, challenge, err := ringcentral.NewPKCE()
	require.NoError(t, err)

	sum := sha256.Sum256([]byte(verifier))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), challenge)
	assert.Equal(t, challenge, ringcentral.PKCEChallenge(verifier))
	assert.GreaterOrEqual(t, len(verifier), 43, "RFC 7636 wants at least 43 characters of entropy")
}

func TestNewPKCE_IsDifferentEveryTime(t *testing.T) {
	first, _, err := ringcentral.NewPKCE()
	require.NoError(t, err)
	second, _, err := ringcentral.NewPKCE()
	require.NoError(t, err)
	assert.NotEqual(t, first, second, "a predictable verifier is no verifier")
}

func TestExchangeCode_SendsTheVerifierAndReadsTheWholePair(t *testing.T) {
	srv, cap := oauthServer(t, http.StatusOK, oauthTokenPair)

	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	tok, err := client.ExchangeCode(context.Background(), ringcentral.ExchangeParams{
		Code:         "one-time-code",
		RedirectURI:  "https://tms.example.com/cb",
		CodeVerifier: "the-verifier",
	})
	require.NoError(t, err)

	require.Len(t, cap.forms, 1)
	assert.Equal(t, "/restapi/oauth/token", cap.paths[0])
	assert.Equal(t, "authorization_code", cap.forms[0].Get("grant_type"))
	assert.Equal(t, "one-time-code", cap.forms[0].Get("code"))
	assert.Equal(t, "https://tms.example.com/cb", cap.forms[0].Get("redirect_uri"))
	assert.Equal(t, "the-verifier", cap.forms[0].Get("code_verifier"))
	assert.True(t, strings.HasPrefix(cap.authHdr[0], "Basic "),
		"app credentials belong in the Basic header, never in the body or the URL")

	assert.Equal(t, "access-1", tok.AccessToken)
	assert.Equal(t, "refresh-1", tok.RefreshToken)
	assert.Equal(t, time.Hour, tok.ExpiresIn)
	assert.Equal(t, 7*24*time.Hour, tok.RefreshExpiresIn,
		"the refresh lifetime is what decides when an absent person must reconnect")
	assert.Equal(t, "4242", tok.OwnerID,
		"owner_id is how we learn WHICH extension connected, without asking the user")
}

func TestExchangeCode_StaleCallbackAsksForAFreshConnect(t *testing.T) {
	srv, _ := oauthServer(t, http.StatusBadRequest,
		`{"error":"invalid_grant","error_description":"Invalid authorization code"}`)

	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	_, err = client.ExchangeCode(context.Background(), ringcentral.ExchangeParams{Code: "used-already"})
	assert.ErrorIs(t, err, ringcentral.ErrReauthRequired)
	assert.NotErrorIs(t, err, ringcentral.ErrInvalidCredentials,
		"a replayed code says nothing about the company credential and must not send an admin to Settings")
}

func TestRefresh_RotatesTheRefreshToken(t *testing.T) {
	srv, cap := oauthServer(t, http.StatusOK,
		`{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600,"refresh_token_expires_in":604800,"owner_id":"4242"}`)

	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	tok, err := client.Refresh(context.Background(), "refresh-1")
	require.NoError(t, err)

	assert.Equal(t, "refresh_token", cap.forms[0].Get("grant_type"))
	assert.Equal(t, "refresh-1", cap.forms[0].Get("refresh_token"))
	assert.Equal(t, "access-2", tok.AccessToken)
	assert.Equal(t, "refresh-2", tok.RefreshToken,
		"RingCentral rotates on every refresh; dropping the new value logs the user out on the next call")
}

func TestRefresh_ExpiredOrRevokedMeansReconnect(t *testing.T) {
	srv, _ := oauthServer(t, http.StatusBadRequest,
		`{"error":"invalid_grant","error_description":"Token not found"}`)

	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	_, err = client.Refresh(context.Background(), "long-dead")
	assert.ErrorIs(t, err, ringcentral.ErrReauthRequired)
}

func TestRefresh_EmptyTokenNeverReachesTheNetwork(t *testing.T) {
	srv, cap := oauthServer(t, http.StatusOK, oauthTokenPair)

	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	_, err = client.Refresh(context.Background(), "   ")
	assert.ErrorIs(t, err, ringcentral.ErrReauthRequired)
	assert.Empty(t, cap.forms, "a user with no refresh token is already disconnected; do not ask RingCentral")
}

func TestRefresh_OutageIsNotAReconnectPrompt(t *testing.T) {
	srv, _ := oauthServer(t, http.StatusInternalServerError, `{"message":"backend down"}`)

	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	_, err = client.Refresh(context.Background(), "refresh-1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ringcentral.ErrReauthRequired,
		"a 500 must not throw away a perfectly good refresh token")
}

func TestRevoke_PostsTheTokenAndTreatsAnUnknownOneAsDone(t *testing.T) {
	srv, cap := oauthServer(t, http.StatusOK, `{}`)
	client, err := ringcentral.NewOAuthClient(oauthApp(srv.URL))
	require.NoError(t, err)

	require.NoError(t, client.Revoke(context.Background(), "refresh-1"))
	require.Len(t, cap.forms, 1)
	assert.Equal(t, "/restapi/oauth/revoke", cap.paths[0])
	assert.Equal(t, "refresh-1", cap.forms[0].Get("token"))

	gone, _ := oauthServer(t, http.StatusBadRequest, `{"error":"invalid_grant"}`)
	client2, err := ringcentral.NewOAuthClient(oauthApp(gone.URL))
	require.NoError(t, err)
	assert.NoError(t, client2.Revoke(context.Background(), "already-dead"),
		"the goal is that the access is gone; a token RingCentral forgot already satisfies it")
}

func TestNewOAuthClient_RefusesAHalfConfiguredApp(t *testing.T) {
	_, err := ringcentral.NewOAuthClient(ringcentral.AppCred{ClientSecret: "s"})
	require.Error(t, err)
	_, err = ringcentral.NewOAuthClient(ringcentral.AppCred{ClientID: "i"})
	require.Error(t, err)
}

func TestBearerClient_SendsAsThePersonAndNeverExchangesACredential(t *testing.T) {
	var seenAuth, seenPath string
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/restapi/oauth/token" {
			tokenCalls++
		}
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"direction":"Outbound","messageStatus":"Queued","from":{"phoneNumber":"+15551110000"},"to":[{"phoneNumber":"+15552220000"}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := ringcentral.NewClientWithBearer(srv.URL, "nancys-token", "4242", 55*time.Minute)
	require.NoError(t, err)

	res, err := client.SendSMS(context.Background(), ringcentral.SMSRequest{
		From: "+15551110000", To: []string{"+15552220000"}, Text: "on my way",
	})
	require.NoError(t, err)

	assert.Equal(t, "99", res.ID)
	assert.Equal(t, "Bearer nancys-token", seenAuth,
		"the whole feature is that the request is authenticated as the person, not as the tenant")
	assert.Equal(t, "/restapi/v1.0/account/~/extension/~/sms", seenPath)
	assert.Zero(t, tokenCalls,
		"a bearer client has no credential to exchange — a token call here would mean a silent fallback")
}

func TestBearerClient_ReportsItsRemainingLifeAndItsOwner(t *testing.T) {
	client, err := ringcentral.NewClientWithBearer("https://platform.example.com", "tok", "4242", 20*time.Minute)
	require.NoError(t, err)

	token, ttl, err := client.AccessToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "tok", token)
	assert.Equal(t, 20*time.Minute, ttl,
		"the in-app phone is told when its token dies; guessing an hour would strand it")

	owner, err := client.TokenOwnerExtensionID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "4242", owner)
}

func TestNewClientWithBearer_RefusesAnEmptyToken(t *testing.T) {
	_, err := ringcentral.NewClientWithBearer("https://platform.example.com", "  ", "4242", time.Hour)
	require.Error(t, err)
}

func TestSelfExtensionInfo_NamesTheAccountBehindTheToken(t *testing.T) {
	srv, paths, _ := probeServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":4242,"account":{"id":808808},"extensionNumber":"104",
			"name":"Nancy Drew","contact":{"email":"nancy@carrier.example"},"type":"User","status":"Enabled"}`))
	})

	client, err := ringcentral.NewClientWithBearer(srv.URL, "nancys-token", "4242", time.Hour)
	require.NoError(t, err)

	info, err := client.SelfExtensionInfo(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "/restapi/v1.0/account/~/extension/~", (*paths)[0])
	assert.Equal(t, "4242", info.ID)
	assert.Equal(t, "808808", info.AccountID,
		"the token exchange never says which account the extension lives in — this call is the only source")
	assert.Equal(t, "104", info.ExtensionNumber)
	assert.Equal(t, "Nancy Drew", info.Name)
	assert.Equal(t, "nancy@carrier.example", info.Email)
}
