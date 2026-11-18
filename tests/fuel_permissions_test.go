package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-2692: backend-fuel gates on these codes. A @hasPerm on a code the catalog
// does not derive is unsatisfiable for everyone but super_admin.
func TestFuelCodes_AreGrantableAndImpliedByTheModule(t *testing.T) {
	codes := []string{
		"fuel.plans.view", "fuel.plans.create", "fuel.plans.send",
		"fuel.settings.view", "fuel.settings.edit",
	}
	for _, c := range codes {
		assert.Truef(t, enums.IsValidPermissionCode(c), "%s must be grantable", c)
		assert.Truef(t, middleware.HasPermission([]string{"fuel"}, c), "module fuel must imply %s", c)
	}
	assert.Contains(t, enums.ModulePermissionCodes(), "fuel")
	// The entity grant covers its own leaves and does not leak sideways.
	assert.True(t, middleware.HasPermission([]string{"fuel.plans"}, "fuel.plans.send"))
	assert.False(t, middleware.HasPermission([]string{"fuel.plans"}, "fuel.settings.edit"))
}
