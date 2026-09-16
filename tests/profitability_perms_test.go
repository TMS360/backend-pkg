package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-180 — load profitability and overhead allocation are ordinary entities
// implied by the accounting / settings module rows every tenant already holds,
// so they need no backfill and refuse only a custom role.
func TestProfitabilityPerms_DottedEntities(t *testing.T) {
	for entity, module := range map[string]string{
		"accounting.profitability":     "accounting",
		"settings.overhead_allocation": "settings",
	} {
		var found *enums.PermissionCatalogEntry
		for i := range enums.PermissionCatalog {
			if enums.PermissionCatalog[i].Code == entity {
				found = &enums.PermissionCatalog[i]
			}
		}
		if !assert.NotNilf(t, found, "%s must be in PermissionCatalog", entity) {
			continue
		}
		assert.Equal(t, module, found.ParentCode)
		assert.Equal(t, []string{"view", "edit"}, found.Actions)
		for _, a := range found.Actions {
			leaf := entity + "." + a
			assert.Truef(t, enums.IsValidPermissionCode(leaf), "%s must be grantable", leaf)
			assert.Truef(t, middleware.HasPermission([]string{module}, leaf), "the %s module must imply %s", module, leaf)
		}
	}

	for got, want := range map[enums.UserPermissionEnum]string{
		enums.PermLoadProfitabilityView:  "accounting.profitability.view",
		enums.PermLoadProfitabilityEdit:  "accounting.profitability.edit",
		enums.PermOverheadAllocationView: "settings.overhead_allocation.view",
		enums.PermOverheadAllocationEdit: "settings.overhead_allocation.edit",
	} {
		assert.Equal(t, want, string(got))
	}
}
