package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-4240 — seeing hidden calls is its own right, calls_view_hidden, instead of
// the admin role. Hidden calls do not play for other office users (BL §8.96), so
// holding calls_view / calls_play must not imply it.

// AC1: the code exists, is flat and grantable through a custom role.
func TestDEV4240_CallsViewHidden_IsAFlatCustomCode(t *testing.T) {
	code := string(enums.PermCallsViewHidden)

	assert.Equal(t, "calls_view_hidden", code)
	assert.True(t, enums.IsCustomPermissionCode(code), "must be grantable through a custom role")
	assert.Contains(t, enums.CustomPermissionCodes(), code)

	// calls_view is NOT an ancestor: no dots, exact match.
	assert.False(t, middleware.HasPermission([]string{string(enums.PermCallsView)}, code))
	assert.False(t, middleware.HasPermission([]string{string(enums.PermCallsPlay)}, code))
	assert.False(t, middleware.HasPermission([]string{"calls"}, code))
}

// AC1/AC4: admin holds it by default; the other office roles that hold the call
// log do not.
func TestDEV4240_CallsViewHidden_AdminOnlyByDefault(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermCallsViewHidden)

	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAdmin], code), "admin keeps today's behaviour")

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleManager, enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace, enums.UserRoleDriver,
	} {
		assert.Falsef(t, middleware.HasPermission(defaults[role], code),
			"%s must not see hidden calls by default (BL §8.96)", role)
	}
}
