package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
)

// DEV-181 — expenses and company cards are ordinary entities; approving an
// expense is flat so a built-in role is refused.
func TestExpensePerms_DottedEntitiesAndFlatApprove(t *testing.T) {
	assertDottedEntity(t, "accounting.expenses", "accounting", "view", "create", "edit", "delete")
	assertDottedEntity(t, "settings.company_cards", "settings", "view", "edit")

	for got, want := range map[enums.UserPermissionEnum]string{
		enums.PermExpensesView:     "accounting.expenses.view",
		enums.PermExpensesCreate:   "accounting.expenses.create",
		enums.PermExpensesEdit:     "accounting.expenses.edit",
		enums.PermExpensesDelete:   "accounting.expenses.delete",
		enums.PermCompanyCardsView: "settings.company_cards.view",
		enums.PermCompanyCardsEdit: "settings.company_cards.edit",
	} {
		assert.Equal(t, want, string(got))
	}

	assertFlatPerm(t, enums.PermExpenseApprove, "expense_approve",
		enums.UserRoleAdmin, enums.UserRoleAccounting)
}
