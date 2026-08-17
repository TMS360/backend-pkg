package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1723: the Sheets connector reads a company credential, and this enum is the
// canonical list of integration keys — tms-auth drives its audit redaction off it
// (setting/redact.go isSecretSettingKey), so a key that is missing here would have
// its raw value written into an audit event.
func TestGoogleSheetsKeyIsARegisteredIntegrationKey(t *testing.T) {
	key := enums.CompanySettingsIntegrationKeyGoogleSheetsAPIKey

	require.Equal(t, "google_sheets_api_key", string(key),
		"the wire value is the Redis settings suffix the connector reads; renaming it orphans every saved credential")
	assert.True(t, key.IsValid(), "must pass enum validation on the settings save path")
	assert.Contains(t, enums.AllCompanySettingsIntegrationKey, key,
		"must be in the canonical list — audit redaction iterates it, so an absent key leaks the secret")
}

// The Sheets credential is its own key, NOT the Maps one. Sharing a single Google
// credential would tie two unrelated integrations together: revoking a
// spreadsheet's key would silently break geocoding.
func TestGoogleSheetsKeyIsSeparateFromGoogleMaps(t *testing.T) {
	assert.NotEqual(t,
		enums.CompanySettingsIntegrationKeyGoogleMapsAPIKey,
		enums.CompanySettingsIntegrationKeyGoogleSheetsAPIKey)
}

// Every declared key must validate and be listed — guards the next addition too.
func TestAllIntegrationKeysAreValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range enums.AllCompanySettingsIntegrationKey {
		assert.Truef(t, k.IsValid(), "%s is listed but does not validate", k)
		assert.Falsef(t, seen[string(k)], "%s is listed twice", k)
		seen[string(k)] = true
	}
}
