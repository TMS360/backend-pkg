package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-4580 — "Edit company hierarchy" (org_chart_edit) is its own flat right.
// "Reports to" used to ride settings.office_users.edit, which every office role
// holds through the `settings` module baseline.

// The code exists, is flat, is listed in Settings › Roles, and nothing in the
// settings area covers it.
func TestDEV4580_OrgChartEdit_IsAFlatCustomCode(t *testing.T) {
	code := string(enums.PermOrgChartEdit)

	assert.Equal(t, "org_chart_edit", code)
	assert.True(t, enums.IsCustomPermissionCode(code), "must be tickable per role in Settings › Roles")
	assert.True(t, enums.IsValidPermissionCode(code))

	var label string
	for _, c := range enums.CustomPermissionCatalog {
		if c.Code == code {
			label = c.Label
		}
	}
	assert.Equal(t, "Edit company hierarchy", label)

	for _, held := range [][]string{
		{"settings"},
		{string(enums.PermSettingsOfficeUsersEdit)},
		{"settings.office_users"},
		enums.ModulePermissionCodes(),
	} {
		assert.Falsef(t, middleware.HasPermission(held, code), "%v must not cover %s", held, code)
	}
}

// Admin, manager and HR hold it by default; no other built-in role does.
func TestDEV4580_OrgChartEdit_DefaultHolders(t *testing.T) {
	code := string(enums.PermOrgChartEdit)
	holders := map[enums.UserRoleEnum]bool{
		enums.UserRoleAdmin: true, enums.UserRoleManager: true, enums.UserRoleHr: true,
	}

	for role, codes := range enums.DefaultRolePermissions() {
		assert.Equalf(t, holders[role], middleware.HasPermission(codes, code),
			"default holder of %s: %s", code, role)
	}
}
