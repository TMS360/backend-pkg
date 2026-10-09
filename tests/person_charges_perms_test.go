package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2510 / BL-6 §6.PC6: person-charge rights. view / void / manage are actions
// on `teams.person_charges`; apply is one sibling entity per department.

var (
	pcView   = string(enums.PermPersonChargesView)
	pcVoid   = string(enums.PermPersonChargesVoid)
	pcManage = string(enums.PermPersonChargesManage)

	pcApplyDispatch   = string(enums.PermPersonChargeApplyDispatch)
	pcApplyFleet      = string(enums.PermPersonChargeApplyFleet)
	pcApplyUpdate     = string(enums.PermPersonChargeApplyUpdate)
	pcApplyAccounting = string(enums.PermPersonChargeApplyAccounting)
	pcApplyDriver     = string(enums.PermPersonChargeApplyDriver)
	pcApplyBonus      = string(enums.PermPersonChargeApplyBonus)

	allPersonChargeLeaves = []string{
		pcView, pcVoid, pcManage,
		pcApplyDispatch, pcApplyFleet, pcApplyUpdate, pcApplyAccounting, pcApplyDriver, pcApplyBonus,
	}
)

func TestPersonCharges_CodesAreRegistered(t *testing.T) {
	for _, code := range allPersonChargeLeaves {
		assert.Truef(t, enums.IsValidPermissionCode(code), "%s must be in the catalog", code)
	}
}

// Pin: `teams.person_charges` is an ENTITY, not a module that swallows the apply
// siblings. Expanding it yields exactly view/void/manage — never apply, never
// fewer than the three (the shipments.audit trap).
func TestPersonCharges_EntityIsNotAModuleThatSwallowsApply(t *testing.T) {
	assert.ElementsMatch(t, []string{pcView, pcVoid, pcManage},
		enums.ExpandPermissions([]string{"teams.person_charges"}))

	for _, e := range enums.PermissionCatalog {
		assert.NotEqualf(t, "teams.person_charges", e.ParentCode,
			"%s must be a sibling under `teams`, not a child of teams.person_charges", e.Code)
	}
	assert.False(t, middleware.HasPermission([]string{"teams.person_charges"}, pcApplyDispatch),
		"the entity grant must not imply any apply code")
}

func TestPersonCharges_EachApplySiblingCarriesOnlyApply(t *testing.T) {
	siblings := map[string]string{
		"teams.person_charges_dispatch":   pcApplyDispatch,
		"teams.person_charges_fleet":      pcApplyFleet,
		"teams.person_charges_update":     pcApplyUpdate,
		"teams.person_charges_accounting": pcApplyAccounting,
		"teams.person_charges_driver":     pcApplyDriver,
		"teams.person_charges_bonus":      pcApplyBonus,
	}
	for entity, leaf := range siblings {
		assert.Equalf(t, []string{leaf}, enums.ExpandPermissions([]string{entity}), entity)
	}
	assert.ElementsMatch(t,
		[]string{"teams.person_charges", "teams.person_charges_dispatch", "teams.person_charges_fleet",
			"teams.person_charges_update", "teams.person_charges_accounting", "teams.person_charges_driver",
			"teams.person_charges_bonus"},
		enums.PersonChargeEntities())
}

// Edge case: granting the parent `teams` still implies every person-charge code.
func TestPersonCharges_TeamsModuleImpliesAll(t *testing.T) {
	expanded := enums.ExpandPermissions([]string{enums.TeamsModuleCode})
	for _, code := range allPersonChargeLeaves {
		assert.Truef(t, middleware.HasPermission([]string{enums.TeamsModuleCode}, code), "teams grants %s", code)
		assert.Containsf(t, expanded, code, "ExpandPermissions(teams) contains %s", code)
	}
}

// AC: a user with only fleet apply is refused a dispatch type.
func TestPersonCharges_FleetApplyDoesNotGrantDispatchApply(t *testing.T) {
	onlyFleet := []string{pcApplyFleet}
	assert.True(t, middleware.HasPermission(onlyFleet, pcApplyFleet))
	for _, other := range []string{pcApplyDispatch, pcApplyUpdate, pcApplyAccounting, pcApplyDriver, pcApplyBonus, pcView, pcVoid, pcManage} {
		assert.Falsef(t, middleware.HasPermission(onlyFleet, other), "fleet apply must not grant %s", other)
	}
}

// The default bag table from the ticket, checked code by code on every built-in
// office role. track_and_trace inherits the dispatcher bag (BL-4 §4.17).
func TestDefaultRolePermissions_PersonChargeBags(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	bags := map[enums.UserRoleEnum][]string{
		enums.UserRoleAdmin:         allPersonChargeLeaves,
		enums.UserRoleDispatcher:    {pcView, pcApplyDispatch},
		enums.UserRoleTrackAndTrace: {pcView, pcApplyDispatch},
		enums.UserRoleManager:       {pcView, pcVoid, pcApplyDispatch, pcApplyBonus},
		enums.UserRoleFleet:         {pcView, pcApplyFleet},
		enums.UserRoleAccounting:    {pcView, pcVoid, pcManage, pcApplyAccounting, pcApplyBonus},
		enums.UserRoleSafety:        {pcView, pcApplyDriver},
		enums.UserRoleHr:            nil,
		enums.UserRoleAuditor:       nil,
		enums.UserRoleDriver:        nil,
		enums.UserRoleOther:         nil,
	}
	for role, want := range bags {
		perms, ok := defaults[role]
		require.Truef(t, ok, "%s must have default grants", role)
		for _, code := range allPersonChargeLeaves {
			assert.Equalf(t, contains(want, code), middleware.HasPermission(perms, code), "%s: %s", role, code)
		}
	}
}

// AC: dispatcher can apply a dispatch type but cannot edit the catalog.
func TestDefaultRolePermissions_DispatcherAppliesDispatchButCannotEditCatalog(t *testing.T) {
	perms := enums.DefaultRolePermissions()[enums.UserRoleDispatcher]
	assert.True(t, middleware.HasPermission(perms, pcApplyDispatch))
	assert.False(t, middleware.HasPermission(perms, pcManage))
}

// AC: accounting can edit the catalog and void a line.
func TestDefaultRolePermissions_AccountingEditsCatalogAndVoids(t *testing.T) {
	perms := enums.DefaultRolePermissions()[enums.UserRoleAccounting]
	assert.True(t, middleware.HasPermission(perms, pcManage))
	assert.True(t, middleware.HasPermission(perms, pcVoid))
}

// AC: a custom role holding the fleet bag's codes behaves exactly like fleet —
// the check reads codes, never the role name.
func TestPersonCharges_CustomRoleWithFleetCodesBehavesLikeFleet(t *testing.T) {
	fleet := enums.DefaultRolePermissions()[enums.UserRoleFleet]
	custom := []string{pcView, pcApplyFleet}
	for _, code := range allPersonChargeLeaves {
		assert.Equalf(t, middleware.HasPermission(fleet, code), middleware.HasPermission(custom, code), code)
	}
}

// The swap only narrows person charges: expanded to leaves, every narrowed role
// keeps the rest of the teams module (and everything else) exactly as before.
func TestDefaultRolePermissions_NarrowedRolesKeepOtherTeamsAccess(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace, enums.UserRoleManager, enums.UserRoleFleet,
		enums.UserRoleAccounting, enums.UserRoleSafety, enums.UserRoleHr, enums.UserRoleAuditor,
		enums.UserRoleOther, // not the driver: since DEV-2866 it holds no teams code at all
	} {
		perms := defaults[role]
		assert.NotContainsf(t, perms, enums.TeamsModuleCode, "%s must not hold the whole teams module", role)

		before := enums.ExpandPermissions(append([]string{enums.TeamsModuleCode}, perms...))
		assert.ElementsMatchf(t, without(before, allPersonChargeLeaves...),
			without(enums.ExpandPermissions(perms), allPersonChargeLeaves...),
			"%s: only person charges may change", role)

		for _, code := range []string{"teams.teams.view", "teams.crews.edit", "teams.dispatchers.view"} {
			assert.Truef(t, middleware.HasPermission(perms, code), "%s keeps %s", role, code)
		}
	}
}

func TestTeamsEntitiesWithoutPersonCharges(t *testing.T) {
	assert.Equal(t, []string{"teams.teams", "teams.crews", "teams.dispatchers"}, enums.TeamsEntitiesWithoutPersonCharges())
}

func contains(list []string, code string) bool {
	for _, c := range list {
		if c == code {
			return true
		}
	}
	return false
}
