package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
)

// DEV-4881: every office role could link a message before the right existed,
// so each one holds it by default; the driver never could.
func TestChatMessagesLink_DefaultForOfficeRolesOnly(t *testing.T) {
	code := string(enums.PermChatMessagesLink)
	assert.True(t, enums.IsKnownPermission(code), "the role editor must offer it")

	defaults := enums.DefaultRolePermissions()
	office := []enums.UserRoleEnum{
		enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting,
		enums.UserRoleFleet, enums.UserRoleSafety, enums.UserRoleHr,
		enums.UserRoleAuditor, enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace,
		enums.UserRoleOther,
	}
	for _, role := range office {
		assert.Containsf(t, defaults[role], code, "office role %q must keep linking", role)
	}
	// The broker portal roles have their own set and no load chats.
	for _, role := range []enums.UserRoleEnum{enums.UserRoleDriver, enums.UserRoleBrokerAdmin, enums.UserRoleBrokerUser} {
		assert.NotContainsf(t, defaults[role], code, "%q must not link messages", role)
	}
}
