package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1409 — `auditor` becomes a GLOBAL built-in role.
//
// Why it has to be a built-in: BuildUserClaims (tms-auth) puts only
// roles.unique_name into the JWT `roles` claim, and custom tenant roles are
// pinned to unique_name NULL by a DB check constraint. @hasRole then matches
// that claim with an exact string compare. So a role that is not in this enum
// (and not seeded from it) can never satisfy a @hasRole gate — tms-audit's
// getAudits and backend-accounting's isAuditorActor would both fail closed.

// The claim string must be exactly "auditor" — @hasRole compares
// case-sensitively, and the seeder derives both roles.name and
// roles.unique_name from String().
func TestUserRoleAuditor_CanonicalString(t *testing.T) {
	assert.Equal(t, "auditor", string(enums.UserRoleAuditor))
	assert.Equal(t, "auditor", enums.UserRoleAuditor.String())
}

// IsValid gates role assignment; without this the role could be defined yet
// rejected everywhere it is used.
func TestUserRoleAuditor_IsValid(t *testing.T) {
	assert.True(t, enums.UserRoleAuditor.IsValid())
	assert.True(t, enums.UserRoleEnum("auditor").IsValid())
}

// Band 3 — the specialist band, alongside accounting / hr / fleet / safety /
// dispatcher. The auditor's authority is over records, not over people, so it
// must not sit above them: hierarchy drives the strictly-below check in
// createUser and assignPermissionsTo*.
func TestUserRoleAuditor_SitsInTheSpecialistBand(t *testing.T) {
	h, ok := enums.UserRoleHierarchy[enums.UserRoleAuditor]
	require.True(t, ok, "auditor must be in the office hierarchy, else it falls back to the schema default")
	assert.EqualValues(t, 3, h)

	assert.Equal(t, enums.UserRoleHierarchy[enums.UserRoleAccounting], h, "peer with accounting")
	assert.Greater(t, h, enums.UserRoleHierarchy[enums.UserRoleAdmin], "below admin")
	assert.Greater(t, h, enums.UserRoleHierarchy[enums.UserRoleManager], "below manager")
}

func TestUserRoleAuditor_EffectiveHierarchyResolves(t *testing.T) {
	assert.EqualValues(t, 3, enums.EffectiveHierarchy([]string{"auditor"}))
	// A user holding both keeps the stronger (lower) band.
	assert.EqualValues(t, 1, enums.EffectiveHierarchy([]string{"auditor", "admin"}))
}

// The auditor receives the same module baseline as its peers at company
// signup. Without an entry here the role would exist but grant nothing, and a
// user holding only it would see an empty product.
func TestUserRoleAuditor_GetsTheModuleBaseline(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	perms, ok := defaults[enums.UserRoleAuditor]
	require.True(t, ok, "auditor must have default grants")
	assert.Subset(t, perms, enums.ModulePermissionCodes())
	assert.ElementsMatch(t,
		append(enums.ModulePermissionCodes(), string(enums.PermInvoiceUnrecordPayment)),
		perms,
		"baseline plus the one governed correction DEV-2038 seeds; every other "+
			"auditor power is role-gated, not permission-gated")
}

// Most of the auditor's governed powers are gated by @hasRole, so the seeded
// flat codes stay a closed list — exactly one today. This fails if a later
// ticket quietly hands the role another custom permission (DEV-2094).
func TestUserRoleAuditor_HoldsOnlyTheUnrecordPaymentCustomCode(t *testing.T) {
	perms := enums.DefaultRolePermissions()[enums.UserRoleAuditor]

	assert.Contains(t, perms, string(enums.PermInvoiceUnrecordPayment),
		"DEV-2038 seeds invoice_unrecord_payment to admin and auditor")

	for _, code := range enums.CustomPermissionCodes() {
		if code == string(enums.PermInvoiceUnrecordPayment) {
			continue
		}
		assert.NotContainsf(t, perms, code, "auditor must not be default-seeded %q", code)
	}
}

// Adding a role must not disturb the existing bands or grants.
func TestUserRoleAuditor_DoesNotDisturbExistingRoles(t *testing.T) {
	assert.EqualValues(t, 0, enums.UserRoleHierarchy[enums.UserRoleSuperAdmin])
	assert.EqualValues(t, 1, enums.UserRoleHierarchy[enums.UserRoleAdmin])
	assert.EqualValues(t, 2, enums.UserRoleHierarchy[enums.UserRoleManager])

	defaults := enums.DefaultRolePermissions()
	_, ok := defaults[enums.UserRoleSuperAdmin]
	assert.False(t, ok, "super_admin still bypasses permission checks")
	assert.Contains(t, defaults[enums.UserRoleAdmin], string(enums.PermTripFinancialsEdit))
	assert.Contains(t, defaults[enums.UserRoleManager], string(enums.PermTripReassignCommitted))
}
