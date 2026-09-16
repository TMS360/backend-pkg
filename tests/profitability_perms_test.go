package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
)

// DEV-180 — load profitability and overhead allocation are ordinary entities
// implied by the accounting / settings module rows every tenant holds.
func TestProfitabilityPerms_DottedEntities(t *testing.T) {
	assertDottedEntity(t, "accounting.profitability", "accounting", "view", "edit")
	assertDottedEntity(t, "settings.overhead_allocation", "settings", "view", "edit")

	for got, want := range map[enums.UserPermissionEnum]string{
		enums.PermLoadProfitabilityView:  "accounting.profitability.view",
		enums.PermLoadProfitabilityEdit:  "accounting.profitability.edit",
		enums.PermOverheadAllocationView: "settings.overhead_allocation.view",
		enums.PermOverheadAllocationEdit: "settings.overhead_allocation.edit",
	} {
		assert.Equal(t, want, string(got))
	}
}
