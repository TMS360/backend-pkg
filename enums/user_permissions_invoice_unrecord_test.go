package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The regression guard for DEV-2038's central decision: whoever records a
// customer payment must NOT be able to take it back off on their own.
//
// That separation only holds because the code is flat. HasPermission matches by
// dotted prefix, and `accounting` is a top-level entry in PermissionCatalog that
// DefaultRolePermissions hands to every built-in role — so a dotted
// `accounting.invoices.unrecord_payment` would be satisfied by anyone holding
// `accounting`, which is exactly the population meant to be excluded. The whole
// second permission would be decorative.
//
// This test fails the moment someone "tidies" it into the accounting module.
func TestInvoiceUnrecordPayment_IsFlatAndSeparateFromRecording(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermInvoiceUnrecordPayment)

	accounting, ok := defaults[enums.UserRoleAccounting]
	require.True(t, ok, "accounting must be in the default matrix")

	// The fact the design rests on: accounting already satisfies the dotted
	// record-payment code, so a dotted un-record code would come free with it.
	assert.True(t, middleware.HasPermission(accounting, "accounting.invoices.record_payment"),
		"accounting satisfies the dotted record-payment code via the `accounting` module — "+
			"this is why un-recording is a flat code")

	assert.False(t, middleware.HasPermission(accounting, code),
		"accounting records payments and must NOT be able to reverse one unaided — "+
			"that second pair of eyes is the point of the separate code")

	// Flat: no ancestor can imply it.
	assert.False(t, middleware.HasPermission([]string{"accounting"}, code),
		"the `accounting` module must not imply un-recording; the code carries no dots")
	assert.False(t, middleware.HasPermission([]string{"accounting.invoices"}, code))

	// Seeded to the two supervisory roles.
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleAuditor} {
		assert.Truef(t, middleware.HasPermission(defaults[role], code),
			"%s should hold %s by default", role, code)
	}

	// Registered, therefore grantable to a custom role by a company that wants
	// it wider — revocable the same way.
	assert.True(t, enums.IsCustomPermissionCode(code))
}

// Every other role is default-deny, driver included. Asserted separately from
// the accounting case above because it is a different claim: accounting is
// excluded on purpose despite being the closest role, while these have simply
// never been near an invoice.
func TestInvoiceUnrecordPayment_DefaultDenyEverywhereElse(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermInvoiceUnrecordPayment)

	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleManager, enums.UserRoleFleet, enums.UserRoleSafety,
		enums.UserRoleHr, enums.UserRoleDispatcher, enums.UserRoleDriver,
		enums.UserRoleOther,
	} {
		assert.Falsef(t, middleware.HasPermission(defaults[role], code),
			"%s must not hold %s by default", role, code)
	}
}
