package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-3433: restricted documents are opened by admin and safety only; every
// other role is default-deny.
func TestComplianceRestrictedView_DefaultGrants(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermComplianceRestrictedView)

	assert.True(t, enums.IsCustomPermissionCode(code), "must be grantable")
	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAdmin], code))
	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleSafety], code))

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleManager, enums.UserRoleAccounting, enums.UserRoleFleet,
		enums.UserRoleHr, enums.UserRoleDispatcher, enums.UserRoleAuditor,
		enums.UserRoleDriver, enums.UserRoleOther,
	} {
		assert.Falsef(t, middleware.HasPermission(defaults[role], code),
			"%s must not hold %s by default", role, code)
	}
}
