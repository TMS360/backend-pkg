package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2548 — approving or declining a pay review becomes a permission that can
// actually refuse someone. Admin and accounting hold it; nobody else does.
func TestStatementReviewResolve_FlatWithDefaults(t *testing.T) {
	assertFlatPerm(t, enums.PermStatementReviewResolve, "statement_review_resolve",
		enums.UserRoleAdmin, enums.UserRoleAccounting)
}

// The bug, stated as an executable claim: the old dotted gate passes for a
// dispatcher on the base permission set (the `accounting` module satisfies it by
// prefix), the new flat code does not.
func TestStatementReviewResolve_DispatcherRefused(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	dispatcher, ok := defaults[enums.UserRoleDispatcher]
	require.True(t, ok, "dispatcher must be in the default matrix")

	assert.True(t, middleware.HasPermission(dispatcher, "accounting.statement_balance_entries.create"),
		"the old gate refused nobody — this is the defect DEV-2548 fixes")
	assert.False(t, middleware.HasPermission(dispatcher, string(enums.PermStatementReviewResolve)),
		"a dispatcher must be refused on resolveStatementReview")
}

// Granting it to a role (Settings -> Roles) is enough to pass the gate again.
func TestStatementReviewResolve_GrantableInSettings(t *testing.T) {
	code := string(enums.PermStatementReviewResolve)
	assert.True(t, enums.IsCustomPermissionCode(code), "must be listed in Settings -> Roles")

	dispatcher := append([]string{}, enums.DefaultRolePermissions()[enums.UserRoleDispatcher]...)
	assert.True(t, middleware.HasPermission(append(dispatcher, code), code))
}
