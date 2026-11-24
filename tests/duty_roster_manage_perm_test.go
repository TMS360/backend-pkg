package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-4653 — "Manage duty roster (any team)" (duty_roster_manage) is its own
// flat right. A dotted tasks.* code would be held by every office role that
// holds the `tasks` module.

// The code exists, is flat, is listed in Settings › Roles, and nothing in the
// tasks area covers it.
func TestDEV4653_DutyRosterManage_IsAFlatCustomCode(t *testing.T) {
	code := string(enums.PermDutyRosterManage)

	assert.Equal(t, "duty_roster_manage", code)
	assert.True(t, enums.IsCustomPermissionCode(code), "must be tickable per role in Settings › Roles")
	assert.True(t, enums.IsValidPermissionCode(code))

	var label string
	for _, c := range enums.CustomPermissionCatalog {
		if c.Code == code {
			label = c.Label
		}
	}
	assert.Equal(t, "Manage duty roster (any team)", label)

	for _, held := range [][]string{
		{"tasks"},
		{"tasks.teams"},
		{string(enums.PermTasksDutiesManage)},
		enums.ModulePermissionCodes(),
	} {
		assert.Falsef(t, middleware.HasPermission(held, code), "%v must not cover %s", held, code)
	}
}

// Only admin holds it by default.
func TestDEV4653_DutyRosterManage_DefaultHolders(t *testing.T) {
	code := string(enums.PermDutyRosterManage)
	for role, codes := range enums.DefaultRolePermissions() {
		assert.Equalf(t, role == enums.UserRoleAdmin, middleware.HasPermission(codes, code),
			"default holder of %s: %s", code, role)
	}
}
