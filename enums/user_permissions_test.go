package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom (flat) permission codes must validate, so the assignPermissionsTo
// {User,Role} mutations accept them and the frontend can grant/revoke them.
func TestCustomPermissions_AreValidGrantableCodes(t *testing.T) {
	for _, code := range enums.CustomPermissionCodes() {
		assert.Truef(t, enums.IsValidPermissionCode(code), "custom code %q must validate", code)
		assert.Truef(t, enums.IsCustomPermissionCode(code), "%q must be reported as custom", code)
	}
	assert.Contains(t, enums.CustomPermissionCodes(), string(enums.PermTripFinancialsEdit))
	assert.Contains(t, enums.CustomPermissionCodes(), string(enums.PermTripFinancialsApprove))
}

// Standard hierarchical codes must NOT be misreported as custom.
func TestIsCustomPermissionCode_StandardCodesAreNotCustom(t *testing.T) {
	for _, code := range []string{"shipments", "accounting", "accounting.statement_trips.edit"} {
		assert.Falsef(t, enums.IsCustomPermissionCode(code), "%q must not be custom", code)
	}
}

// A flat custom code resolves by EXACT match in HasPermission: no module
// implies it via prefix, and holding it grants exactly itself.
func TestCustomPermission_ResolvesByExactMatchOnly(t *testing.T) {
	edit := string(enums.PermTripFinancialsEdit)

	// Holder of the exact code passes; @hasPerm(["trip_financials_edit"]) works.
	assert.True(t, middleware.HasPermission([]string{edit}, edit))

	// No standard module (or the whole default set) can imply it.
	for _, m := range enums.ModulePermissionCodes() {
		assert.Falsef(t, middleware.HasPermission([]string{m}, edit),
			"module %q must not imply custom %q", m, edit)
	}
	assert.False(t, middleware.HasPermission(enums.ModulePermissionCodes(), edit))

	// It implies nothing else, and is not implied by the sibling custom code.
	assert.False(t, middleware.HasPermission([]string{edit}, string(enums.PermTripFinancialsApprove)))
}

// A flat code has no ancestors, so CompactHierarchy must keep it intact (never
// dropped as a redundant child) — otherwise a grant would silently vanish.
func TestCompactHierarchy_KeepsFlatCustomCodes(t *testing.T) {
	in := []string{"accounting", "accounting.invoices.create", string(enums.PermTripFinancialsEdit)}
	got := middleware.CompactHierarchy(in)
	assert.Contains(t, got, string(enums.PermTripFinancialsEdit))
	assert.Contains(t, got, "accounting")
	assert.NotContains(t, got, "accounting.invoices.create") // child collapsed under its module
}

// Custom codes are NOT auto-granted: they are absent from the module set every
// role receives on signup.
func TestCustomPermissions_NotInDefaultModuleGrant(t *testing.T) {
	modules := enums.ModulePermissionCodes()
	require.NotEmpty(t, modules)
	for _, code := range enums.CustomPermissionCodes() {
		assert.NotContains(t, modules, code)
	}
}

// AllUserPermissions surfaces custom codes too, so any "everything" enumeration
// stays complete.
func TestAllUserPermissions_IncludesCustom(t *testing.T) {
	all := enums.AllUserPermissions()
	for _, code := range enums.CustomPermissionCodes() {
		assert.Contains(t, all, code)
	}
}

// DEV-1256 / BL §7.5 grant matrix: trip_financials_edit is held by default by
// admin and accounting; the regular dispatcher does NOT get it; and
// trip_financials_approve is not seeded to anyone.
func TestDefaultRolePermissions_TripFinancialsMatrix(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	modules := enums.ModulePermissionCodes()
	edit := string(enums.PermTripFinancialsEdit)
	approve := string(enums.PermTripFinancialsApprove)

	// admin + accounting: modules + edit.
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleAccounting} {
		assert.Subset(t, defaults[role], modules, "%s keeps module defaults", role)
		assert.Containsf(t, defaults[role], edit, "%s must hold trip_financials_edit", role)
	}

	// dispatcher: modules only, NO edit (the divergence being fixed).
	assert.Subset(t, defaults[enums.UserRoleDispatcher], modules)
	assert.NotContains(t, defaults[enums.UserRoleDispatcher], edit,
		"dispatcher must NOT hold trip_financials_edit by default")

	// approve is not default-seeded to any role (kept grantable via custom roles).
	for role, perms := range defaults {
		assert.NotContainsf(t, perms, approve, "%s must not be default-seeded trip_financials_approve", role)
	}

	// super_admin bypasses checks → intentionally has no default grant.
	_, ok := defaults[enums.UserRoleSuperAdmin]
	assert.False(t, ok, "super_admin must not be seeded (it bypasses permission checks)")
}

// DEV-1226 / DEV-1227: trip_reassign_committed is a registered custom code held
// by manager (dispatch manager) and admin by default, but NOT by the regular
// dispatcher.
func TestTripReassignCommitted_RegisteredAndManagerDefault(t *testing.T) {
	code := string(enums.PermTripReassignCommitted)
	assert.Equal(t, "trip_reassign_committed", code)
	assert.True(t, enums.IsValidPermissionCode(code), "must validate so custom roles can grant it")
	assert.True(t, enums.IsCustomPermissionCode(code))

	defaults := enums.DefaultRolePermissions()
	assert.Contains(t, defaults[enums.UserRoleManager], code, "manager holds it by default")
	assert.Contains(t, defaults[enums.UserRoleAdmin], code, "admin holds it by default")
	assert.NotContains(t, defaults[enums.UserRoleDispatcher], code, "dispatcher must NOT hold it")

	// Flat code → exact match only: no module grant implies it.
	for _, m := range enums.ModulePermissionCodes() {
		assert.Falsef(t, middleware.HasPermission([]string{m}, code), "module %q must not imply %q", m, code)
	}
	assert.True(t, middleware.HasPermission([]string{code}, code))
}

// The returned slices must be independent copies — mutating one role's grant
// must not leak into another's shared module baseline.
func TestDefaultRolePermissions_SlicesAreIndependent(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	disp := defaults[enums.UserRoleDispatcher]
	if len(disp) > 0 {
		disp[0] = "mutated"
	}
	assert.NotContains(t, enums.DefaultRolePermissions()[enums.UserRoleAccounting], "mutated")
}
