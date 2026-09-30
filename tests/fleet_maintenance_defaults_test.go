package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2502 / BL-15: the default grant table for fleet maintenance.
//
//	admin, manager, fleet, safety                     view + manage
//	dispatcher, track and trace, accounting, auditor  view only
//	hr, driver, other                                 none
func TestDefaultRolePermissions_FleetMaintenanceTable(t *testing.T) {
	view := string(enums.PermFleetMaintenanceView)
	manage := string(enums.PermFleetMaintenanceManage)
	defaults := enums.DefaultRolePermissions()

	cases := []struct {
		role         enums.UserRoleEnum
		view, manage bool
	}{
		{enums.UserRoleAdmin, true, true},
		{enums.UserRoleManager, true, true},
		{enums.UserRoleFleet, true, true},
		{enums.UserRoleSafety, true, true},
		{enums.UserRoleDispatcher, true, false},
		{enums.UserRoleTrackAndTrace, true, false},
		{enums.UserRoleAccounting, true, false},
		{enums.UserRoleAuditor, true, false},
		{enums.UserRoleHr, false, false},
		{enums.UserRoleDriver, false, false},
		{enums.UserRoleOther, false, false},
	}
	for _, tc := range cases {
		perms, ok := defaults[tc.role]
		require.Truef(t, ok, "%s must have default grants", tc.role)
		assert.Equalf(t, tc.view, middleware.HasPermission(perms, view), "%s: %s", tc.role, view)
		assert.Equalf(t, tc.manage, middleware.HasPermission(perms, manage), "%s: %s", tc.role, manage)
	}
}

// "These roles keep every other fleet page they have today": expanded to leaves,
// a narrowed role's set is the old `fleet`-module set minus the maintenance
// leaves it lost — nothing else moves, fleet or otherwise.
func TestDefaultRolePermissions_NarrowedRolesKeepOtherFleetAccess(t *testing.T) {
	view := string(enums.PermFleetMaintenanceView)
	manage := string(enums.PermFleetMaintenanceManage)
	defaults := enums.DefaultRolePermissions()

	lost := map[enums.UserRoleEnum][]string{
		enums.UserRoleDispatcher:    {manage},
		enums.UserRoleTrackAndTrace: {manage},
		enums.UserRoleAccounting:    {manage},
		enums.UserRoleAuditor:       {manage},
		enums.UserRoleHr:            {view, manage},
		enums.UserRoleDriver:        {view, manage},
		enums.UserRoleOther:         {view, manage},
	}
	for role, gone := range lost {
		perms := defaults[role]
		assert.NotContainsf(t, perms, enums.FleetModuleCode, "%s must not hold the whole fleet module", role)

		// What the role held before DEV-2502: the same list with `fleet` back.
		before := append([]string{enums.FleetModuleCode}, withoutFleetCodes(perms)...)
		want := without(enums.ExpandPermissions(before), gone...)
		assert.ElementsMatchf(t, want, enums.ExpandPermissions(perms), "%s: only maintenance may change", role)

		for _, code := range []string{"fleet.trucks.view", "fleet.trucks.edit", "fleet.trailers.view", "fleet.asset_charges.view"} {
			assert.Truef(t, middleware.HasPermission(perms, code), "%s keeps %s", role, code)
		}
	}
}

// A fleet entity added to the catalog later reaches the narrowed roles without
// touching this list; only maintenance is ever left out.
func TestFleetEntitiesWithoutMaintenance_IsEveryFleetEntityButMaintenance(t *testing.T) {
	ents := enums.FleetEntitiesWithoutMaintenance()
	assert.NotContains(t, ents, "fleet.maintenance")
	assert.Subset(t, ents, []string{"fleet.trucks", "fleet.trailers", "fleet.asset_charges"})
	assert.ElementsMatch(t,
		without(enums.ExpandPermissions([]string{enums.FleetModuleCode}),
			string(enums.PermFleetMaintenanceView), string(enums.PermFleetMaintenanceManage)),
		enums.ExpandPermissions(ents))
}

// The narrowed shape survives a role-editor save: RollupPermissions keeps it as
// is and does not collapse it back into `fleet`.
func TestDefaultRolePermissions_NarrowedFleetSurvivesRollup(t *testing.T) {
	perms := enums.DefaultRolePermissions()[enums.UserRoleAccounting]
	rolled := enums.RollupPermissions(enums.ExpandPermissions(perms))
	assert.NotContains(t, rolled, enums.FleetModuleCode)
	assert.False(t, middleware.HasPermission(rolled, string(enums.PermFleetMaintenanceManage)))
	assert.True(t, middleware.HasPermission(rolled, string(enums.PermFleetMaintenanceView)))
}

func withoutFleetCodes(codes []string) []string {
	var out []string
	for _, c := range codes {
		if !middleware.HasPermission([]string{enums.FleetModuleCode}, c) {
			out = append(out, c)
		}
	}
	return out
}

func without(codes []string, drop ...string) []string {
	skip := make(map[string]bool, len(drop))
	for _, d := range drop {
		skip[d] = true
	}
	var out []string
	for _, c := range codes {
		if !skip[c] {
			out = append(out, c)
		}
	}
	return out
}
