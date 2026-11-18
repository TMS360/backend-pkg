package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/client/samsara"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2056: the extras key must be registered in the canonical integration-key
// list — tms-auth's audit redaction iterates it, so an absent key would write
// live Samsara tokens into an audit event.
func TestSamsaraExtraOrgsKeyIsARegisteredIntegrationKey(t *testing.T) {
	key := enums.CompanySettingsIntegrationKeySamsaraExtraOrgs

	require.Equal(t, "samsara_extra_orgs", string(key),
		"the wire value is the Redis settings suffix consumers read; renaming it orphans every saved organization")
	assert.True(t, key.IsValid(), "must pass enum validation on the settings save and delete paths")
	assert.Contains(t, enums.AllCompanySettingsIntegrationKey, key,
		"must be in the canonical list — audit redaction iterates it, so an absent key leaks the tokens")
	assert.NotEqual(t, enums.CompanySettingsIntegrationKeySamsaraAPIKey, key,
		"the extras list is a separate row: the primary token must stay readable by every existing consumer")
}

// A company that only ever saved the primary token has no extras row at all.
// That is the normal state, not an error.
func TestParseExtraOrgs_BlankIsNoOrgs(t *testing.T) {
	for _, raw := range []string{"", "   ", "[]"} {
		orgs, err := samsara.ParseExtraOrgs(raw)
		require.NoError(t, err, "raw %q", raw)
		assert.Empty(t, orgs)
	}
}

func TestParseExtraOrgs_RoundTrip(t *testing.T) {
	orgs, err := samsara.ParseExtraOrgs(
		`[{"id":"a","name":"West fleet","token":"tok-west-1234","is_active":true},` +
			`{"id":"b","name":"","token":"tok-east-5678","is_active":false}]`)
	require.NoError(t, err)
	require.Len(t, orgs, 2)

	assert.Equal(t, "West fleet", orgs[0].Name)
	assert.Equal(t, "tok-west-1234", orgs[0].Token)
	assert.True(t, orgs[0].IsActive)
	assert.False(t, orgs[1].IsActive)
}

func TestParseExtraOrgs_MalformedIsAnError(t *testing.T) {
	_, err := samsara.ParseExtraOrgs(`{"not":"a list"}`)
	require.Error(t, err)
}

// Only switched-on organizations get polled; an inactive one keeps its token.
func TestActiveExtraOrgs(t *testing.T) {
	active := samsara.ActiveExtraOrgs([]samsara.ExtraOrg{
		{ID: "a", Token: "tok-a-1234", IsActive: true},
		{ID: "b", Token: "tok-b-5678", IsActive: false},
		{ID: "c", Token: "tok-c-9012", IsActive: true},
	})
	require.Len(t, active, 2)
	assert.Equal(t, "a", active[0].ID)
	assert.Equal(t, "c", active[1].ID)
}

// An empty name is allowed on save; readers show "Org 2" / "Org 3" — the
// primary samsara_api_key is org 1, so the extras start numbering at 2.
func TestExtraOrgDisplayName(t *testing.T) {
	assert.Equal(t, "Org 2", samsara.ExtraOrg{}.DisplayName(0))
	assert.Equal(t, "Org 3", samsara.ExtraOrg{Name: "  "}.DisplayName(1))
	assert.Equal(t, "West fleet", samsara.ExtraOrg{Name: "West fleet"}.DisplayName(0))
}
