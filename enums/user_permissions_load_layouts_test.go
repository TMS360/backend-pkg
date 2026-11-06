package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-3773: company Load page layouts are managed by admin only by default;
// every other role is default-deny, and the `settings` module does not imply it.
func TestLoadLayoutsManage_DefaultGrants(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermLoadLayoutsManage)

	assert.Equal(t, "load_layouts_manage", code)
	assert.True(t, enums.IsCustomPermissionCode(code), "must be grantable")
	assert.True(t, enums.IsValidPermissionCode(code))
	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAdmin], code))
	assert.False(t, middleware.HasPermission([]string{"settings"}, code), "the settings module must not imply it")

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleManager, enums.UserRoleAccounting, enums.UserRoleFleet,
		enums.UserRoleSafety, enums.UserRoleHr, enums.UserRoleDispatcher,
		enums.UserRoleAuditor, enums.UserRoleTrackAndTrace, enums.UserRoleDriver,
		enums.UserRoleOther, enums.UserRoleBrokerAdmin, enums.UserRoleBrokerUser,
	} {
		assert.Falsef(t, middleware.HasPermission(defaults[role], code),
			"%s must not hold %s by default", role, code)
	}
}
