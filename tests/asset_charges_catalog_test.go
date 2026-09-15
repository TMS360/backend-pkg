package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2240 — the permission half of the asset-charges epic.
//
// The epic's later tickets (asset schedules, one-time charges, incident
// resolution, the Finances tab) all gate on these codes, so they have to be
// grantable BEFORE any of them ships: a @hasPerm on a code the catalog does not
// derive is not "strict", it is unsatisfiable for everyone except super_admin.

// assetChargeCatalog is the (entity -> parent) shape the ticket asks for:
// getPermissions renders the leaves under Fleet and Tasks.
func TestAssetChargeEntities_AreInTheCatalogUnderFleetAndTasks(t *testing.T) {
	want := map[string]struct {
		parent  string
		actions []string
	}{
		"fleet.asset_charges": {parent: "fleet", actions: []string{"view", "manage", "adjudicate"}},
		"fleet.maintenance":   {parent: "fleet", actions: []string{"view", "manage"}},
		"tasks.incidents":     {parent: "tasks", actions: []string{"resolve"}},
	}

	seen := map[string]bool{}
	for _, e := range enums.PermissionCatalog {
		exp, ok := want[e.Code]
		if !ok {
			continue
		}
		seen[e.Code] = true
		assert.Equalf(t, exp.parent, e.ParentCode, "%s must hang off the %s module", e.Code, exp.parent)
		assert.ElementsMatchf(t, exp.actions, e.Actions, "%s actions", e.Code)
		assert.NotEmptyf(t, e.Label, "%s needs a label — it is what the Roles page shows", e.Code)
	}
	for code := range want {
		assert.Truef(t, seen[code], "catalog entry %q is missing", code)
	}
}

// Every code the epic will put in a @hasPerm must validate, or
// assignPermissionsTo{Role,User} refuses it and the Roles page cannot offer it.
func TestAssetChargeCodes_AreGrantable(t *testing.T) {
	for _, c := range []string{
		string(enums.PermAssetChargesView),
		string(enums.PermAssetChargesManage),
		string(enums.PermAssetChargesAdjudicate),
		string(enums.PermFleetMaintenanceView),
		string(enums.PermFleetMaintenanceManage),
		string(enums.PermTaskIncidentsResolve),
	} {
		assert.Truef(t, enums.IsValidPermissionCode(c), "%s must be grantable", c)
	}
}

// These are ordinary hierarchical codes, which is what makes them work for
// tenants that already exist: every company holds the `fleet` / `tasks` module
// row from signup, HasPermission matches ancestors, so the new leaves are live
// the moment the catalog ships — no back-fill migration, and none is possible
// to need (README §1 of tms-auth/database/migrations scopes that rule to the
// FLAT custom codes, which no module grant can imply).
func TestAssetChargeCodes_AreImpliedByTheModuleGrant(t *testing.T) {
	assert.True(t, middleware.HasPermission([]string{"fleet"}, string(enums.PermAssetChargesAdjudicate)))
	assert.True(t, middleware.HasPermission([]string{"fleet"}, string(enums.PermFleetMaintenanceManage)))
	assert.True(t, middleware.HasPermission([]string{"tasks"}, string(enums.PermTaskIncidentsResolve)))

	// The entity grant alone covers its own leaves…
	assert.True(t, middleware.HasPermission([]string{"fleet.asset_charges"}, string(enums.PermAssetChargesView)))
	// …and does not leak sideways into a neighbour.
	assert.False(t, middleware.HasPermission([]string{"fleet.trucks"}, string(enums.PermAssetChargesView)))
	assert.False(t, middleware.HasPermission([]string{"fleet.asset_charges"}, string(enums.PermFleetMaintenanceView)))
	assert.False(t, middleware.HasPermission([]string{"tasks.tasks"}, string(enums.PermTaskIncidentsResolve)))
}

// The ticket lists `fleet.assets.out_of_service_override` as a seventh code. It
// must NOT be added: DEV-2254 already shipped that power as the FLAT
// asset_out_of_service_override, deliberately, because a dotted code under
// `fleet` is satisfied by the module grant every role receives at signup — which
// would make "a user without the permission is refused" untestable.
func TestOutOfServiceOverride_StaysFlat(t *testing.T) {
	require.True(t, enums.IsCustomPermissionCode(string(enums.PermAssetOutOfServiceOverride)))
	assert.False(t, enums.IsValidPermissionCode("fleet.assets.out_of_service_override"),
		"do not add a dotted twin of asset_out_of_service_override — the module grant would satisfy it")
	assert.False(t, middleware.HasPermission([]string{"fleet"}, string(enums.PermAssetOutOfServiceOverride)),
		"the flat code must stay default-deny for a plain fleet-module holder")
}
