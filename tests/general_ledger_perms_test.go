package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// assertFlatPerm pins a governed flat code: its wire value, that it is a
// grantable custom code no module implies, and exactly which built-in roles
// hold it by default. Every role in the matrix is checked, so a code leaking to
// one more role fails as loudly as one missing from a holder.
func assertFlatPerm(t *testing.T, got enums.UserPermissionEnum, want string, holders ...enums.UserRoleEnum) {
	t.Helper()
	code := string(got)
	assert.Equal(t, want, code, "the code is the contract with the services, the FE and the backfill migration")
	assert.NotContains(t, code, ".", "a dotted code would be satisfied by a module grant")
	assert.Truef(t, enums.IsValidPermissionCode(code), "%s must be grantable", code)
	assert.Truef(t, enums.IsCustomPermissionCode(code), "%s must be in CustomPermissionCatalog", code)
	for _, m := range enums.ModulePermissionCodes() {
		assert.Falsef(t, middleware.HasPermission([]string{m}, code), "module %q must not imply %s", m, code)
	}

	held := map[enums.UserRoleEnum]bool{}
	for _, r := range holders {
		held[r] = true
	}
	for role, perms := range enums.DefaultRolePermissions() {
		assert.Equalf(t, held[role], middleware.HasPermission(perms, code), "default grant of %s to %s", code, role)
	}
}

// DEV-177 — the general ledger reads and manual journal entries are flat so a
// built-in role can actually be refused.
func TestGeneralLedgerPerms_FlatWithDefaults(t *testing.T) {
	assertFlatPerm(t, enums.PermGeneralLedgerView, "general_ledger_view",
		enums.UserRoleAdmin, enums.UserRoleAccounting, enums.UserRoleAuditor)
	assertFlatPerm(t, enums.PermJournalEntryManage, "journal_entry_manage",
		enums.UserRoleAdmin, enums.UserRoleAccounting)
	assert.False(t, middleware.HasPermission([]string{string(enums.PermJournalEntryManage)}, string(enums.PermGeneralLedgerView)),
		"manage does not imply view — the codes are independent checkboxes")
}
