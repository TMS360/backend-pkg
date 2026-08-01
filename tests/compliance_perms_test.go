package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1466 — one set of compliance permission codes, seeded and enforced.
//
// The system used to carry three naming schemes for the same capability:
//   - GraphQL/REST:   settings.compliance.view | settings.compliance.edit  (canonical)
//   - Go constants:   compliance.view | compliance.upload | compliance.renew (orphans)
//   - FE-invented:    compliance.configure | compliance.self-view          (phantoms)
//
// These tests pin the single agreed vocabulary and prove it is both catalog-valid
// and enforced by the same hierarchical rule every other settings.* entity uses.

const (
	complianceView = "settings.compliance.view"
	complianceEdit = "settings.compliance.edit"
)

// AC: the Go constants match the catalog leaves (no third naming scheme in code).
func TestCompliance_ConstantsMatchCatalogLeaves(t *testing.T) {
	assert.Equal(t, complianceView, string(enums.PermComplianceView))
	assert.Equal(t, complianceEdit, string(enums.PermComplianceEdit))

	assert.True(t, enums.IsValidPermissionCode(complianceView), "view leaf must be a grantable catalog code")
	assert.True(t, enums.IsValidPermissionCode(complianceEdit), "edit leaf must be a grantable catalog code")
	assert.True(t, enums.PermComplianceView.IsValid())
	assert.True(t, enums.PermComplianceEdit.IsValid())
}

// AC: the catalog lists ONLY the agreed codes — the orphan/phantom names are gone
// and can never be granted (so nothing can slip in under a different code).
func TestCompliance_NoThirdNamingScheme(t *testing.T) {
	for _, dead := range []string{
		"compliance.view",
		"compliance.upload",
		"compliance.renew",
		"compliance.configure", // FE-invented
		"compliance.self-view", // FE-invented, self-service not shipped
	} {
		assert.False(t, enums.IsValidPermissionCode(dead),
			"%q must NOT be a grantable code — only settings.compliance.* exists", dead)
	}

	// Exactly the two leaves live under settings.compliance in the catalog.
	assert.ElementsMatch(t, []string{complianceView, complianceEdit},
		enums.ExpandPermissions([]string{"settings.compliance"}))
}

// AC: a `settings` grant (what every built-in role is seeded) implies BOTH
// compliance leaves via HasPermission prefix-matching — so view/edit are enforced
// without any separate per-role leaf seeding, exactly like settings.company.* etc.
func TestCompliance_SettingsModuleImpliesCompliance(t *testing.T) {
	settingsGrant := []string{"settings"}
	assert.True(t, middleware.HasPermission(settingsGrant, complianceView))
	assert.True(t, middleware.HasPermission(settingsGrant, complianceEdit))

	// The leaf codes themselves also satisfy the checks (granted-role succeeds).
	assert.True(t, middleware.HasPermission([]string{complianceView}, complianceView))
	assert.True(t, middleware.HasPermission([]string{complianceEdit}, complianceEdit))
}

// AC: a role WITHOUT the grant is refused by the API-level check. view does not
// imply edit, and an unrelated module implies neither.
func TestCompliance_UngrantedRoleRefused(t *testing.T) {
	assert.False(t, middleware.HasPermission([]string{"dashboard"}, complianceView))
	assert.False(t, middleware.HasPermission([]string{"dashboard"}, complianceEdit))

	// Holding only view must not grant edit (upload/renew/mute stay gated).
	assert.False(t, middleware.HasPermission([]string{complianceView}, complianceEdit),
		"view must not imply edit")

	// A compliance leaf must not roll UP to imply the whole settings module.
	assert.False(t, middleware.HasPermission([]string{complianceEdit}, "settings.company.edit"),
		"a compliance grant must not leak to sibling settings entities")
}

// AC: the fresh-tenant / seed path grants view+edit to admin (and the roles product
// specified — Safety). DefaultRolePermissions is exactly what the tms-auth seeder
// writes at company signup, so asserting on it proves the seed outcome.
func TestCompliance_SeedDefaultsGrantAdminAndSafety(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleSafety} {
		seeded := defaults[role]
		require.NotEmpty(t, seeded, "role %s must receive default grants", role)
		assert.True(t, middleware.HasPermission(seeded, complianceView),
			"seeded %s must be able to view compliance", role)
		assert.True(t, middleware.HasPermission(seeded, complianceEdit),
			"seeded %s must be able to edit compliance", role)
	}
}
