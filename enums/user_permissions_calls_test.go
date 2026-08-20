package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The regression guard for DEV-1753's central decision.
//
// The call log could not reuse settings.office_users.view, because `settings` is
// a top-level entry in PermissionCatalog: ModulePermissionCodes() therefore
// contains it, DefaultRolePermissions() hands that whole set to EVERY built-in
// role including driver, and middleware.HasPermission matches by dotted prefix.
// So every driver in every tenant already satisfies settings.office_users.view —
// which is asserted below, because it is the fact the whole design rests on.
//
// calls_view / calls_play are flat instead: no dots, exact match, nothing
// implies them. This test fails the moment someone "tidies" them into a `calls`
// module and quietly hands every driver the office's recorded conversations.
func TestCallsPerms_AreFlatAndDeniedToDrivers(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	driver, ok := defaults[enums.UserRoleDriver]
	require.True(t, ok, "driver must be in the default matrix")

	for _, code := range []string{string(enums.PermCallsView), string(enums.PermCallsPlay)} {
		assert.Falsef(t, middleware.HasPermission(driver, code),
			"driver must not hold %q by default — a recording is a named person's voice", code)
	}

	// The fact that forced the flat codes. If this ever starts failing, someone
	// removed `settings` from the module baseline and the call log could then
	// safely have reused settings.office_users.view after all.
	assert.True(t, middleware.HasPermission(driver, "settings.office_users.view"),
		"drivers satisfy settings.office_users.view via the `settings` module — "+
			"this is why calls_view exists as a flat code")

	// Flat means no ancestor can imply them.
	assert.False(t, middleware.HasPermission([]string{"calls"}, string(enums.PermCallsPlay)),
		"a `calls` module must not imply calls_play; the codes carry no dots")

	// Registered, therefore grantable through a custom role.
	assert.True(t, enums.IsCustomPermissionCode(string(enums.PermCallsView)))
	assert.True(t, enums.IsCustomPermissionCode(string(enums.PermCallsPlay)))

	// And default-granted to the dispatch desk, which is what the back-fill
	// migration in tms360-backend mirrors for pre-existing tenants.
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleDispatcher} {
		assert.Truef(t, middleware.HasPermission(defaults[role], string(enums.PermCallsView)),
			"%s should hold calls_view by default", role)
	}
}
