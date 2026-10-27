package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-3428: the three migration codes are flat, admin holds all three, the
// auditor holds rollback and unlock, and every other role is default-deny.
func TestMigrationPermissions_DefaultGrants(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	rollback := string(enums.PermMigrationRollback)
	unlock := string(enums.PermMigrationUnlock)
	signoff := string(enums.PermMigrationSignoff)

	for _, code := range []string{rollback, unlock, signoff} {
		assert.True(t, enums.IsCustomPermissionCode(code), "%s must be grantable", code)
		assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAdmin], code),
			"admin should hold %s by default", code)
	}

	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAuditor], rollback))
	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAuditor], unlock))
	assert.False(t, middleware.HasPermission(defaults[enums.UserRoleAuditor], signoff),
		"sign-off is the customer admin's approval, not the auditor's")

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleManager, enums.UserRoleAccounting, enums.UserRoleFleet,
		enums.UserRoleSafety, enums.UserRoleHr, enums.UserRoleDispatcher,
		enums.UserRoleDriver, enums.UserRoleOther,
	} {
		for _, code := range []string{rollback, unlock, signoff} {
			assert.Falsef(t, middleware.HasPermission(defaults[role], code),
				"%s must not hold %s by default", role, code)
		}
	}
}
