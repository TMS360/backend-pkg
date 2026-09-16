package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
)

// DEV-183 — tax settings and tax filings are ordinary entities implied by the
// settings / accounting module rows every tenant holds.
func TestTaxPerms_DottedEntities(t *testing.T) {
	assertDottedEntity(t, "settings.tax", "settings", "view", "edit")
	assertDottedEntity(t, "accounting.tax_filings", "accounting", "view", "edit")

	for got, want := range map[enums.UserPermissionEnum]string{
		enums.PermTaxSettingsView: "settings.tax.view",
		enums.PermTaxSettingsEdit: "settings.tax.edit",
		enums.PermTaxFilingsView:  "accounting.tax_filings.view",
		enums.PermTaxFilingsEdit:  "accounting.tax_filings.edit",
	} {
		assert.Equal(t, want, string(got))
	}
}
