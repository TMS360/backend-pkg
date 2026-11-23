package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-2349 — "Truck & trailer status settings": the permission family that says
// who may edit the company's own list of truck/trailer statuses (the list
// itself is DEV-2350).
//
// It is a hierarchical catalog entity under `settings`, copied from
// settings.load_status on purpose. That choice is what makes the ticket's
// "existing companies get it without doing anything" true for free: every
// tenant's built-in roles were seeded with the `settings` MODULE row
// (enums.ModulePermissionCodes via SetDefaultRolePerms), and both
// middleware.HasPermission and the Roles page expand a module into its leaves —
// so the family shows up ticked for admin on companies created long before this
// release, with no back-fill migration to run. A flat custom code would have
// needed one; a dotted leaf does not (see tests/vendor_bill_perms_test.go for
// the flat counterpart).

// AC1 + AC2 — the family exists, sits under Settings next to the load status
// list, and carries exactly view/create/edit/delete.
func TestAssetStatusPerms_CatalogEntity(t *testing.T) {
	assertDottedEntity(t, "settings.asset_status", "settings", "view", "create", "edit", "delete")

	var label string
	for _, e := range enums.PermissionCatalog {
		if e.Code == "settings.asset_status" {
			label = e.Label
		}
	}
	assert.Equal(t, "Truck & trailer status settings", label, "the heading the Roles page renders")

	for got, want := range map[enums.UserPermissionEnum]string{
		enums.PermAssetStatusView:   "settings.asset_status.view",
		enums.PermAssetStatusCreate: "settings.asset_status.create",
		enums.PermAssetStatusEdit:   "settings.asset_status.edit",
		enums.PermAssetStatusDelete: "settings.asset_status.delete",
	} {
		assert.Equal(t, want, string(got), "the constant is the contract with DEV-2350's @hasPerm")
	}
}

// AC1 — an admin (and a manager) of a company that already exists holds the
// `settings` module, so every leaf is already satisfied. Same map is what a new
// company is seeded with at signup, which is AC2.
func TestAssetStatusPerms_ExistingAndNewCompaniesHoldItAlready(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleManager} {
		perms := defaults[role]
		assert.Containsf(t, perms, "settings", "role %s must hold the settings module row the back-fill-free grant relies on", role)
		for _, leaf := range []enums.UserPermissionEnum{
			enums.PermAssetStatusView, enums.PermAssetStatusCreate,
			enums.PermAssetStatusEdit, enums.PermAssetStatusDelete,
		} {
			assert.Truef(t, middleware.HasPermission(perms, string(leaf)), "role %s must satisfy %s", role, leaf)
		}
	}

	// The Roles page ticks a leaf when the role holds it or any ancestor
	// (expandToLeaves), so the same expansion is what QA sees on screen.
	assert.Contains(t, enums.ExpandPermissions([]string{"settings"}), "settings.asset_status.edit")
}

// "Nothing else changes" — the new leaves neither absorb nor are absorbed by the
// two permissions that already govern asset status elsewhere.
func TestAssetStatusPerms_DoesNotMoveTheNeighbouringGates(t *testing.T) {
	assert.False(t, middleware.HasPermission([]string{"settings.asset_status.edit"}, "fleet.maintenance.manage"),
		"setting a status ON a truck stays fleet.maintenance.manage")
	assert.False(t, middleware.HasPermission([]string{"settings.asset_status.edit"}, string(enums.PermAssetOutOfServiceOverride)),
		"the dispatch override keeps its own flat permission")
	assert.False(t, enums.IsCustomPermissionCode("settings.asset_status.edit"),
		"a dotted leaf must not also live in CustomPermissionCatalog")
}
