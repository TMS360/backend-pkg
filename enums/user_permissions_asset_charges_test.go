package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2244 asks for `fleet.asset_charges.view` / `.manage`. Spelled that way the
// gate does not exist: `fleet` is a top-level module, DefaultRolePermissions
// hands every built-in office role the module, and HasPermission tries the
// SHORTEST prefix first — so the very first candidate for
// `fleet.asset_charges.view` is `fleet`, and it matches. Everyone would be able
// to put money on someone else's truck.
//
// The second half of the trap is that `fleet.asset_charges` appears in no
// catalog, so the code could be neither granted nor revoked — a permission the
// system does not know cannot be taken away either.
//
// This test fails the moment someone "aligns" these codes with the FE ticket.
func TestAssetCharges_AreFlatAndNotImpliedByFleet(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	view := string(enums.PermAssetChargesView)
	manage := string(enums.PermAssetChargesManage)

	fleet, ok := defaults[enums.UserRoleFleet]
	require.True(t, ok, "fleet must be in the default matrix")

	// The fact the design rests on: the fleet role already satisfies any dotted
	// code under the module, so the dotted spelling would come free with it.
	assert.True(t, middleware.HasPermission(fleet, "fleet.trucks.view"),
		"the fleet role satisfies dotted codes under `fleet` — this is why the "+
			"asset-charge codes are flat")
	assert.True(t, middleware.HasPermission(fleet, "fleet.asset_charges.manage"),
		"the dotted spelling DEV-2244 asks for is satisfied by the bare module — "+
			"it is a hole, not a gate")

	// Flat: no ancestor can imply either code.
	for _, code := range []string{view, manage} {
		assert.False(t, middleware.HasPermission([]string{"fleet"}, code), code)
		assert.False(t, middleware.HasPermission([]string{"fleet.asset_charges"}, code), code)
		assert.False(t, middleware.HasPermission(fleet, code),
			code+": default-deny until granted, including for the fleet role")

		// Registered, therefore grantable to a custom role and revocable the same way.
		assert.True(t, enums.IsCustomPermissionCode(code), code)
	}

	// Seeded to admin and manager, so the Finances tab is reachable on a fresh
	// company without a custom role.
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleManager} {
		assert.True(t, middleware.HasPermission(defaults[role], view), string(role))
		assert.True(t, middleware.HasPermission(defaults[role], manage), string(role))
	}

	// Viewing does not imply managing: the two are separate flat codes.
	assert.False(t, middleware.HasPermission([]string{view}, manage))
}
