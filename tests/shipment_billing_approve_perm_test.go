package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TMS360/backend-pkg/auth"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1884 — "Approve for billing" is its own permission.
//
// Before: the office good-to-go step (verifyByBroker) was gated on
// `shipments.shipments.edit`, a code every office role holds via the `shipments`
// module, so anyone who could edit a load could approve it for billing. The FE
// hid the button behind the whole Accounting module, which is a different set
// again (and one that every role holds at signup, driver included).
//
// After: one FLAT code, seeded to admin / manager / accounting only.

func approveCode() string { return string(enums.PermShipmentBillingApprove) }

// AC1: the permission exists, validates, and is a flat custom code — that is
// what makes the role editor render it as its own checkbox (getPermissions in
// tms-auth walks CustomPermissionCatalog and sets custom:true).
func TestShipmentBillingApprove_IsRegisteredFlatCustomPerm(t *testing.T) {
	code := approveCode()
	assert.Equal(t, "shipment_billing_approve", code, "the code is part of the contract with tms-loads and the FE")
	assert.NotContains(t, code, ".", "a dotted code would be satisfied by an ancestor grant")

	assert.True(t, enums.IsValidPermissionCode(code), "must validate so assignPermissionsTo{Role,User} accepts it")
	assert.True(t, enums.IsCustomPermissionCode(code), "must be a flat custom code")
	assert.Contains(t, enums.CustomPermissionCodes(), code)

	var label string
	for _, c := range enums.CustomPermissionCatalog {
		if c.Code == code {
			label = c.Label
		}
	}
	assert.Equal(t, "Approve loads for billing", label, "the label the role editor shows")
}

// AC2 + AC4: a new company (SetDefaultRolePerms applies DefaultRolePermissions)
// gives it to admin, manager and accounting; dispatcher does not get it, and
// track_and_trace — defined as "whatever dispatcher gets" — does not either.
func TestShipmentBillingApprove_DefaultGrantMatrix(t *testing.T) {
	code := approveCode()
	defaults := enums.DefaultRolePermissions()

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting,
	} {
		assert.Containsf(t, defaults[role], code, "%s must hold %q by default", role, code)
	}

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace, enums.UserRoleFleet,
		enums.UserRoleSafety, enums.UserRoleHr, enums.UserRoleAuditor,
		enums.UserRoleDriver, enums.UserRoleOther,
		enums.UserRoleBrokerAdmin, enums.UserRoleBrokerUser,
	} {
		assert.NotContainsf(t, defaults[role], code, "role %s must NOT hold %q by default", role, code)
		assert.Falsef(t, middleware.HasPermission(defaults[role], code),
			"role %s must not resolve %q through anything in its default set", role, code)
	}
}

// AC4, the derivation itself: track_and_trace is not a hand-copied list, so a
// perm added to dispatcher later is inherited automatically. Guard that the two
// sets stay equal — that is the contract that keeps this AC true tomorrow.
func TestTrackAndTrace_StillMatchesDispatcher(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	assert.ElementsMatch(t, defaults[enums.UserRoleDispatcher], defaults[enums.UserRoleTrackAndTrace])
}

// Edge case: holding the Accounting MODULE is not this permission, and neither
// is holding Shipments (the two shapes the ticket says to avoid). No module code
// at all may imply it.
func TestShipmentBillingApprove_NoPrefixLeak(t *testing.T) {
	code := approveCode()

	for _, m := range enums.ModulePermissionCodes() {
		assert.Falsef(t, middleware.HasPermission([]string{m}, code), "module %q must not imply %q", m, code)
	}
	// The exact two the ticket calls out, including their entity/action leaves.
	for _, held := range []string{"accounting", "shipments", "shipments.shipments.edit", "accounting.invoices.view"} {
		assert.Falsef(t, middleware.HasPermission([]string{held}, code), "%q must not imply %q", held, code)
	}
	// Only the code itself grants it.
	assert.True(t, middleware.HasPermission([]string{code}, code))
}

// The permission must stay grantable AND revocable on a custom role: the set
// algebra the role editor runs (ExpandPermissions / RollupPermissions) must keep
// a flat code intact in both directions, otherwise a save silently drops it.
func TestShipmentBillingApprove_SurvivesRoleEditorSetAlgebra(t *testing.T) {
	code := approveCode()

	assert.Contains(t, enums.ExpandPermissions([]string{"shipments", code}), code,
		"expand must not swallow the flat code")
	assert.Contains(t, enums.RollupPermissions([]string{"shipments", code}), code,
		"rollup must not fold the flat code into a module")

	// Revoking is just its absence from the next save.
	assert.NotContains(t, enums.ExpandPermissions([]string{"shipments"}), code)
}

// AC5: super admin still bypasses permission checks — it holds no default grant
// (it is absent from the matrix on purpose) and passes anyway, because the
// bypass is evaluated before perms are looked at. A dispatcher with the same
// (empty) grant set is refused.
func TestShipmentBillingApprove_SuperAdminBypasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	code := approveCode()

	defaults := enums.DefaultRolePermissions()
	require.NotContains(t, defaults, enums.UserRoleSuperAdmin, "super_admin is deliberately absent from the default matrix")

	status := func(role enums.UserRoleEnum, perms []string) int {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/verify", nil)

		actor := &consts.Actor{ID: uuid.New(), Claims: &consts.UserClaims{Roles: []string{role.String()}}}
		ctx := middleware.WithActor(c.Request.Context(), actor)
		ctx = auth.WithUserPerms(ctx, perms)
		c.Request = c.Request.WithContext(ctx)

		middleware.RequirePerms(code)(c)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, status(enums.UserRoleSuperAdmin, nil),
		"super admin bypasses the check with no grants at all")
	assert.Equal(t, http.StatusForbidden, status(enums.UserRoleDispatcher, defaults[enums.UserRoleDispatcher]),
		"a default dispatcher is refused")
	assert.Equal(t, http.StatusOK, status(enums.UserRoleAdmin, defaults[enums.UserRoleAdmin]),
		"a default admin passes")
}
