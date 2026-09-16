package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-176 — the chart of accounts gates on settings.chart_of_accounts.view|edit.
// The codes must be grantable (assignPermissionsTo{Role,User} and the Roles page
// read the catalog) and implied by the `settings` module row every existing
// tenant already holds, so no back-fill migration is needed.
func TestChartOfAccountsPerms_GrantableAndImpliedBySettings(t *testing.T) {
	for _, c := range []enums.UserPermissionEnum{enums.PermChartOfAccountsView, enums.PermChartOfAccountsEdit} {
		assert.Truef(t, enums.IsValidPermissionCode(string(c)), "%s must be grantable", c)
		assert.Truef(t, middleware.HasPermission([]string{"settings"}, string(c)), "%s must follow the settings module grant", c)
	}
	assert.False(t, middleware.HasPermission([]string{"settings.accounting_types"}, string(enums.PermChartOfAccountsEdit)),
		"a neighbouring settings entity must not imply the chart of accounts")
}
