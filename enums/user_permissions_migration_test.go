package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-3488 (reverses DEV-3428's grants): the migration is a dev-team job, so
// the three migration codes are not grantable and no role holds them by
// default. The constants stay — three services still gate on migration_unlock,
// which only platform staff pass now.
func TestMigrationPermissions_NotGrantableNotDefaulted(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	codes := []string{
		string(enums.PermMigrationRollback),
		string(enums.PermMigrationUnlock),
		string(enums.PermMigrationSignoff),
	}

	for _, code := range codes {
		assert.Falsef(t, enums.IsCustomPermissionCode(code), "%s must not be grantable", code)
		assert.Falsef(t, enums.IsValidPermissionCode(code), "%s must be refused as an unknown code", code)
		for role, perms := range defaults {
			assert.Falsef(t, middleware.HasPermission(perms, code),
				"%s must not hold %s by default", role, code)
		}
	}
}
