package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TMS360/backend-pkg/keycrypt"
)

const (
	testSecret      = "0000000000000000000000000000000000000000000000000000000000000001"
	otherSecret     = "0000000000000000000000000000000000000000000000000000000000000002"
	testCompanyID   = "11111111-1111-1111-1111-111111111111"
	secondCompanyID = "22222222-2222-2222-2222-222222222222"
)

func useSecret(t *testing.T, secret string) {
	t.Helper()
	c, err := keycrypt.NewCipher(secret)
	require.NoError(t, err)
	keycrypt.SetDefault(c)
	t.Cleanup(func() { keycrypt.SetDefault(nil) })
}

func recordReports(t *testing.T) *[]error {
	t.Helper()
	var reported []error
	prev := reportOpenFailure
	reportOpenFailure = func(_ context.Context, err error) { reported = append(reported, err) }
	t.Cleanup(func() { reportOpenFailure = prev })
	return &reported
}

// cached is the Redis blob tms-auth writes: cache.Set json.Marshals the value.
func cached(t *testing.T, value string) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	require.NoError(t, err)
	return b
}

func sealFor(t *testing.T, companyID, settingKey, plain string) string {
	t.Helper()
	sealed, err := keycrypt.SealSetting(companyID, settingKey, plain)
	require.NoError(t, err)
	require.True(t, keycrypt.IsSealed(sealed))
	return sealed
}

// AC1: today's plain keys keep working, with or without the secret loaded.
func TestDecodeAPIKey_PlainKeyWorksAsBefore(t *testing.T) {
	reported := recordReports(t)

	key, err := decodeAPIKey(context.Background(), testCompanyID, "here_api_key", cached(t, "plain-here-key"))

	require.NoError(t, err)
	assert.Equal(t, "plain-here-key", key)
	assert.Empty(t, *reported)
}

// AC2: a key replaced by its sealed form still opens to the same key, while a
// second company on a plain key keeps working at the same time.
func TestDecodeAPIKey_SealedKeyOpens_PlainNeighbourUntouched(t *testing.T) {
	useSecret(t, testSecret)
	sealed := sealFor(t, testCompanyID, "samsara_api_key", "samsara-token")

	key, err := decodeAPIKey(context.Background(), testCompanyID, "samsara_api_key", cached(t, sealed))
	require.NoError(t, err)
	assert.Equal(t, "samsara-token", key)

	neighbour, err := decodeAPIKey(context.Background(), secondCompanyID, "samsara_api_key", cached(t, "plain-token"))
	require.NoError(t, err)
	assert.Equal(t, "plain-token", neighbour)
}

// AC3 + edge case: a sealed value opened with a different secret (two services
// out of sync) fails closed as "not connected", is reported, and the error
// carries neither the sealed nor the plain value.
func TestDecodeAPIKey_WrongSecretFailsClosedAndReports(t *testing.T) {
	useSecret(t, testSecret)
	sealed := sealFor(t, testCompanyID, "here_api_key", "real-here-key")
	useSecret(t, otherSecret)
	reported := recordReports(t)

	key, err := decodeAPIKey(context.Background(), testCompanyID, "here_api_key", cached(t, sealed))

	require.Error(t, err)
	assert.Empty(t, key)
	require.Len(t, *reported, 1)
	for _, e := range []error{err, (*reported)[0]} {
		assert.NotContains(t, e.Error(), "real-here-key")
		assert.NotContains(t, e.Error(), sealed)
		assert.Contains(t, e.Error(), "here_api_key")
	}
}

// AC3: a damaged value fails the same way.
func TestDecodeAPIKey_DamagedValueFailsClosed(t *testing.T) {
	useSecret(t, testSecret)
	sealed := sealFor(t, testCompanyID, "here_api_key", "real-here-key")
	damaged := sealed[:len(sealed)-4] + "AAAA"
	reported := recordReports(t)

	_, err := decodeAPIKey(context.Background(), testCompanyID, "here_api_key", cached(t, damaged))

	require.Error(t, err)
	assert.Len(t, *reported, 1)
}

// A sealed key copied into another company's slot does not open there.
func TestDecodeAPIKey_SealedKeyIsBoundToItsCompany(t *testing.T) {
	useSecret(t, testSecret)
	sealed := sealFor(t, testCompanyID, "here_api_key", "real-here-key")
	recordReports(t)

	_, err := decodeAPIKey(context.Background(), secondCompanyID, "here_api_key", cached(t, sealed))

	require.Error(t, err)
}

type uspsLikeCred struct {
	UserID   string `json:"user_id"`
	Password string `json:"password"`
}

type extraOrg struct {
	ID       string `json:"id"`
	Token    string `json:"token"`
	IsActive bool   `json:"is_active"`
}

// Edge case: multi-field values (USPS, RingCentral, Samsara extra orgs) open
// to the same JSON as before.
func TestDecodeCredential_SealedMultiFieldValueOpensToSameJSON(t *testing.T) {
	useSecret(t, testSecret)
	credJSON := `{"user_id":"u1","password":"p1"}`
	orgsJSON := `[{"id":"org-2","token":"t2","is_active":true}]`

	plainCred, err := decodeCredential[uspsLikeCred](context.Background(), testCompanyID, "usps_credentials", cached(t, credJSON))
	require.NoError(t, err)
	sealedCred, err := decodeCredential[uspsLikeCred](context.Background(), testCompanyID, "usps_credentials",
		cached(t, sealFor(t, testCompanyID, "usps_credentials", credJSON)))
	require.NoError(t, err)
	assert.Equal(t, plainCred, sealedCred)
	assert.Equal(t, uspsLikeCred{UserID: "u1", Password: "p1"}, sealedCred)

	orgs, err := decodeCredential[[]extraOrg](context.Background(), testCompanyID, "samsara_extra_orgs",
		cached(t, sealFor(t, testCompanyID, "samsara_extra_orgs", orgsJSON)))
	require.NoError(t, err)
	assert.Equal(t, []extraOrg{{ID: "org-2", Token: "t2", IsActive: true}}, orgs)
}

// A raw JSON object written directly (no outer quotes) still decodes.
func TestDecodeCredential_RawJSONStillDecodes(t *testing.T) {
	cred, err := decodeCredential[uspsLikeCred](context.Background(), testCompanyID, "usps_credentials",
		[]byte(`{"user_id":"u1","password":"p1"}`))
	require.NoError(t, err)
	assert.Equal(t, "u1", cred.UserID)
}

// AC3: a sealed multi-field value that cannot be opened fails closed.
func TestDecodeCredential_UnopenableFailsClosed(t *testing.T) {
	useSecret(t, testSecret)
	sealed := sealFor(t, testCompanyID, "ringcentral_credentials", `{"client_id":"x"}`)
	useSecret(t, otherSecret)
	reported := recordReports(t)

	_, err := decodeCredential[map[string]string](context.Background(), testCompanyID, "ringcentral_credentials", cached(t, sealed))

	require.Error(t, err)
	assert.Len(t, *reported, 1)
}
