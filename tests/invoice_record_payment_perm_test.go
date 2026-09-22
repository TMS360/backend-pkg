package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2366 — recording a customer payment becomes a permission that can actually
// refuse someone.
//
// assertFlatPerm pins the wire value, the flatness, the catalog registration and
// the exact holder set across the whole matrix, so a code leaking to one more
// role fails as loudly as one missing from a holder.
func TestInvoiceRecordPayment_FlatWithDefaults(t *testing.T) {
	assertFlatPerm(t, enums.PermInvoiceRecordPayment, "invoice_record_payment",
		enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting)
}

// The bug this code exists to fix, stated as an executable claim.
//
// recordPayment shipped gated on `accounting.invoices.record_payment`. That
// string is not a leaf of the `accounting.invoices` catalog entry (its actions
// are view/create/edit), so it was never grantable — and it still refused
// nobody, because HasPermission splits the required code on "." and matches any
// prefix, and ModulePermissionCodes() hands the bare `accounting` module to
// every built-in role at signup. Dispatcher included.
//
// Both halves are asserted: the old spelling passes for a dispatcher, the new
// one does not. Re-dotting the code turns the second half red.
func TestInvoiceRecordPayment_FixesTheDottedCodeThatNeverRefused(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermInvoiceRecordPayment)

	dispatcher, ok := defaults[enums.UserRoleDispatcher]
	require.True(t, ok, "dispatcher must be in the default matrix")

	assert.True(t, middleware.HasPermission(dispatcher, "accounting.invoices.record_payment"),
		"the old dotted code was satisfied by the `accounting` module every office role holds — "+
			"this is the defect DEV-2366 fixes, and the reason the new code is flat")

	assert.False(t, middleware.HasPermission(dispatcher, code),
		"a dispatcher on the base permission set must be refused; the AC is written for this 403")
}

// Recording and un-recording stay two codes, so closing an invoice and reopening
// it are never the same person's unaided round trip (the DEV-2038 decision that
// DEV-2366 must not quietly undo).
func TestInvoiceRecordPayment_StaysSeparateFromUnrecording(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	record := string(enums.PermInvoiceRecordPayment)
	unrecord := string(enums.PermInvoiceUnrecordPayment)

	assert.NotEqual(t, record, unrecord)

	// Accounting collects money but does not reverse a collection.
	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAccounting], record))
	assert.False(t, middleware.HasPermission(defaults[enums.UserRoleAccounting], unrecord),
		"accounting records payments and must not reverse one unaided")

	// The auditor is the mirror image: it reverses, it does not record.
	assert.True(t, middleware.HasPermission(defaults[enums.UserRoleAuditor], unrecord))
	assert.False(t, middleware.HasPermission(defaults[enums.UserRoleAuditor], record),
		"the auditor's move is the correction, not the collection")

	// Neither code implies the other.
	assert.False(t, middleware.HasPermission([]string{record}, unrecord))
	assert.False(t, middleware.HasPermission([]string{unrecord}, record))
}
