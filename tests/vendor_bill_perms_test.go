package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// assertDottedEntity pins a hierarchical catalog entity: exact action list under
// its module, every leaf grantable and implied by the module row every existing
// tenant holds (so no backfill is needed).
func assertDottedEntity(t *testing.T, entity, module string, actions ...string) {
	t.Helper()
	var found *enums.PermissionCatalogEntry
	for i := range enums.PermissionCatalog {
		if enums.PermissionCatalog[i].Code == entity {
			found = &enums.PermissionCatalog[i]
		}
	}
	if !assert.NotNilf(t, found, "%s must be in PermissionCatalog", entity) {
		return
	}
	assert.Equal(t, module, found.ParentCode)
	assert.Equal(t, actions, found.Actions)
	for _, a := range actions {
		leaf := entity + "." + a
		assert.Truef(t, enums.IsValidPermissionCode(leaf), "%s must be grantable", leaf)
		assert.Truef(t, middleware.HasPermission([]string{module}, leaf), "the %s module must imply %s", module, leaf)
	}
}

// DEV-174 — vendors and vendor bills are ordinary accounting entities; the
// approval chain and the payment void are flat so a built-in role is refused.
func TestVendorBillPerms_DottedEntities(t *testing.T) {
	assertDottedEntity(t, "accounting.vendors", "accounting", "view", "create", "edit", "delete")
	assertDottedEntity(t, "accounting.vendor_bills", "accounting", "view", "create", "edit", "delete", "pay")

	for got, want := range map[enums.UserPermissionEnum]string{
		enums.PermVendorsView:       "accounting.vendors.view",
		enums.PermVendorsCreate:     "accounting.vendors.create",
		enums.PermVendorsEdit:       "accounting.vendors.edit",
		enums.PermVendorsDelete:     "accounting.vendors.delete",
		enums.PermVendorBillsView:   "accounting.vendor_bills.view",
		enums.PermVendorBillsCreate: "accounting.vendor_bills.create",
		enums.PermVendorBillsEdit:   "accounting.vendor_bills.edit",
		enums.PermVendorBillsDelete: "accounting.vendor_bills.delete",
		enums.PermVendorBillsPay:    "accounting.vendor_bills.pay",
	} {
		assert.Equal(t, want, string(got))
	}
}

func TestVendorBillPerms_FlatApprovalsWithDefaults(t *testing.T) {
	assertFlatPerm(t, enums.PermVendorBillApproveLevel1, "vendor_bill_approve_level_1",
		enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting)
	assertFlatPerm(t, enums.PermVendorBillApproveLevel2, "vendor_bill_approve_level_2",
		enums.UserRoleAdmin, enums.UserRoleManager)
	assertFlatPerm(t, enums.PermVendorBillApproveLevel3, "vendor_bill_approve_level_3",
		enums.UserRoleAdmin)
	assertFlatPerm(t, enums.PermVendorBillPaymentVoid, "vendor_bill_payment_void",
		enums.UserRoleAdmin, enums.UserRoleAuditor)

	levels := []string{string(enums.PermVendorBillApproveLevel1), string(enums.PermVendorBillApproveLevel2), string(enums.PermVendorBillApproveLevel3)}
	for _, held := range levels {
		for _, need := range levels {
			if held != need {
				assert.Falsef(t, middleware.HasPermission([]string{held}, need), "%s must not imply %s", held, need)
			}
		}
	}
	assert.False(t, middleware.HasPermission([]string{string(enums.PermVendorBillsPay)}, string(enums.PermVendorBillPaymentVoid)),
		"recording a payment must not imply voiding one")
}
