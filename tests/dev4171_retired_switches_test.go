package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
)

// DEV-4171: switches no server checks are taken out of the role editor.

// AC1/AC3: every retired switch is gone from the list the role editor reads
// (getPermissions = catalog + custom list), cannot be saved, is not seeded, and
// is not "known" — so a service that starts checking it fails permcheck.
func TestDEV4171_RetiredSwitchesAreNotOffered(t *testing.T) {
	assert.NotEmpty(t, enums.RetiredPermissionCodes)
	offered := map[string]struct{}{}
	for _, c := range enums.AllUserPermissions() {
		offered[c] = struct{}{}
	}
	for code, why := range enums.RetiredPermissionCodes {
		assert.NotEmptyf(t, why, "%s needs a reason", code)
		assert.Falsef(t, enums.IsValidPermissionCode(code), "%s must not be grantable", code)
		assert.Falsef(t, enums.IsKnownPermission(code), "%s must not be known", code)
		assert.Falsef(t, enums.IsCustomPermissionCode(code), "%s must not be a standalone switch", code)
		for c := range offered {
			assert.Falsef(t, enums.IsRetiredPermission(c), "offered code %s sits under retired %s", c, code)
		}
	}
	for role, codes := range enums.DefaultRolePermissions() {
		for _, c := range codes {
			assert.Falsef(t, enums.IsRetiredPermission(c), "%s is seeded retired %s", role, c)
		}
	}
	for _, leaf := range enums.ExpandPermissions(enums.ModulePermissionCodes()) {
		assert.Falsef(t, enums.IsRetiredPermission(leaf), "a module still expands to retired %s", leaf)
	}
}

// Leaves of a retired entity count as retired; a sibling does not.
func TestDEV4171_IsRetiredPermission(t *testing.T) {
	assert.True(t, enums.IsRetiredPermission("accounting.credit_memos.view"))
	assert.True(t, enums.IsRetiredPermission("drivers.one_time_charges"))
	assert.True(t, enums.IsRetiredPermission("reports_manage"))
	assert.False(t, enums.IsRetiredPermission("accounting.invoices.view"))
	assert.False(t, enums.IsRetiredPermission("accounting"))
	assert.False(t, enums.IsRetiredPermission("accounting.invoice_batches.view"))
}

// Out of scope here, wired by other tickets: kept in the editor.
func TestDEV4171_SwitchesOtherTicketsWireAreKept(t *testing.T) {
	for _, c := range []string{"settings.office_roles.view", "settings.office_users.create"} {
		assert.Truef(t, enums.IsValidPermissionCode(c), "%s stays (DEV-3906 / office-user split)", c)
	}
	// The ones the old spec census called dead are checked on main now.
	for _, c := range []string{
		"accounting.expenses.view", "expense_approve", "settings.company_cards.edit",
		string(enums.PermLoadLayoutsManage), "accounting.report_configs.edit",
	} {
		assert.Truef(t, enums.IsValidPermissionCode(c), "%s is checked by a server and stays", c)
	}
}

// AC2: an old grant is never dropped by the roll-up the services and auth run,
// and the remaining leaves still pass. Old rows stay inert, nothing is lost.
func TestDEV4171_OldGrantsStayAndRollUp(t *testing.T) {
	held := []string{"accounting.credit_memos.view", "accounting.invoices.view", "reports_manage"}
	assert.ElementsMatch(t, held, enums.RollupPermissions(held))
	assert.Contains(t, enums.ExpandPermissions([]string{"accounting.comments"}), "accounting.comments",
		"a stale entity row is kept as is, not expanded or dropped")
}

// Edge case: removing a leaf makes the rest of its entity "the whole entity".
// A role holding invoice batches view+create now rolls up to the entity — it
// passes exactly the same checks, because no server checks the removed edit.
func TestDEV4171_EntityRollUpAfterRemoval(t *testing.T) {
	got := enums.RollupPermissions([]string{"accounting.invoice_batches.view", "accounting.invoice_batches.create"})
	assert.Equal(t, []string{"accounting.invoice_batches"}, got)
	got = enums.RollupPermissions([]string{"dashboard.stats.view"})
	assert.Equal(t, []string{"dashboard"}, got, "stats is the only dashboard entity left")
}
