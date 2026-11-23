package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-2385 — routines ("standing duties") are managed through a dotted entity
// under the `tasks` module, so the grant every tenant's admin and manager
// already holds implies it. No back-fill migration exists, and per the rule
// pinned in assertDottedEntity none is needed.
func TestRoutinePerms_DutiesManageIsADottedEntity(t *testing.T) {
	assertDottedEntity(t, "tasks.duties", "tasks", "manage")

	assert.Equal(t, "tasks.duties.manage", string(enums.PermTasksDutiesManage))

	// Reading routines is the ordinary task read, NOT the manage code: a member
	// who may not edit a duty still sees it.
	assert.True(t, middleware.HasPermission([]string{string(enums.PermTasksView)}, string(enums.PermTasksView)))
	assert.False(t, middleware.HasPermission([]string{string(enums.PermTasksView)}, string(enums.PermTasksDutiesManage)))

	// Holding the whole tasks.tasks entity is still not authority over routines.
	assert.False(t, middleware.HasPermission([]string{"tasks.tasks"}, string(enums.PermTasksDutiesManage)))
}
