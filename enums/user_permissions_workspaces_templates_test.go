package enums_test

import (
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1333 — finish the workspaces permission module: add the templates entity.

// AC: a person holding only the templates VIEW permission can list templates but
// cannot instantiate one — create is required.
func TestWorkspacesTemplatesView_CannotCreate(t *testing.T) {
	held := []string{string(enums.PermWorkspacesTemplatesView)}

	assert.True(t, middleware.HasPermission(held, string(enums.PermWorkspacesTemplatesView)),
		"view alone satisfies the templates view check (list the catalog)")
	assert.False(t, middleware.HasPermission(held, string(enums.PermWorkspacesTemplatesCreate)),
		"view alone must NOT satisfy the create check (cannot instantiate)")
	assert.False(t, middleware.HasPermission(held, string(enums.PermWorkspacesTemplatesEdit)))
}

// AC + edge: holding the whole "workspaces" module implicitly holds every entity
// beneath it — templates included — via hierarchical prefix matching, with no
// extra grant. A tenant granted the module BEFORE templates existed gains it
// automatically (no re-grant).
func TestWorkspacesModule_ImpliesTemplates(t *testing.T) {
	module := []string{"workspaces"}
	for _, leaf := range []enums.UserPermissionEnum{
		enums.PermWorkspacesTemplatesView,
		enums.PermWorkspacesTemplatesCreate,
		enums.PermWorkspacesTemplatesEdit,
	} {
		assert.Truef(t, middleware.HasPermission(module, string(leaf)),
			"granting the workspaces module implies %q", leaf)
	}
	// The module set every role receives on signup carries "workspaces", so the
	// implication holds for a default grant too.
	assert.True(t, middleware.HasPermission(enums.ModulePermissionCodes(), string(enums.PermWorkspacesTemplatesCreate)))
}

// The templates leaf codes are valid, grantable, three-segment codes following
// the module-then-entity-then-action shape of the sibling entities.
func TestWorkspacesTemplates_ShapeAndValidity(t *testing.T) {
	for _, leaf := range []enums.UserPermissionEnum{
		enums.PermWorkspacesTemplatesView,
		enums.PermWorkspacesTemplatesCreate,
		enums.PermWorkspacesTemplatesEdit,
	} {
		code := string(leaf)
		assert.True(t, enums.IsValidPermissionCode(code), "%q must be a valid grantable code", code)
		parts := strings.Split(code, ".")
		require.Len(t, parts, 3, "%q is module.entity.action (three segments, the shipped shape)", code)
		assert.Equal(t, "workspaces", parts[0])
		assert.Equal(t, "templates", parts[1])
	}
	assert.True(t, enums.IsValidPermissionCode("workspaces.templates"), "the entity code itself is valid")
}

// The catalog carries the templates entity beside the existing workspaces
// entities, with the view/create/edit actions and the right parent.
func TestWorkspacesTemplates_CatalogEntry(t *testing.T) {
	var found bool
	for _, e := range enums.PermissionCatalog {
		if e.Code == "workspaces.templates" {
			found = true
			assert.Equal(t, "workspaces", e.ParentCode)
			assert.Equal(t, []string{"view", "create", "edit"}, e.Actions)
		}
	}
	assert.True(t, found, "the catalog contains the workspaces.templates entity")
}

// Regression / integrity: after the addition the catalog has no duplicate codes
// (entity codes or expanded leaf codes), and every parent code exists.
func TestPermissionCatalog_NoDuplicatesAndParentsExist(t *testing.T) {
	codes := map[string]struct{}{}
	leaves := map[string]struct{}{}
	for _, e := range enums.PermissionCatalog {
		_, dup := codes[e.Code]
		require.Falsef(t, dup, "duplicate catalog code %q", e.Code)
		codes[e.Code] = struct{}{}
		for _, a := range e.Actions {
			leaf := e.Code + "." + a
			_, ldup := leaves[leaf]
			require.Falsef(t, ldup, "duplicate leaf code %q", leaf)
			leaves[leaf] = struct{}{}
		}
	}
	for _, e := range enums.PermissionCatalog {
		if e.ParentCode != "" {
			_, ok := codes[e.ParentCode]
			assert.Truef(t, ok, "entity %q references a parent %q that is not a catalog code", e.Code, e.ParentCode)
		}
	}
}

// Guard: the tasks permissions are still HERE — the task service is alive
// (DEV-1334 closed WONT DO), so nothing about tasks was deleted. What DID change
// in DEV-1335 is their spelling: the work-item leaves moved under the
// `tasks.tasks` entity so they are grantable at all. `tasks.teams` is untouched.
func TestTasksPermissions_NotDeletedAndGrantable(t *testing.T) {
	assert.Equal(t, "tasks.tasks.view", string(enums.PermTasksView))
	assert.Equal(t, "tasks.tasks.create", string(enums.PermTasksCreate))
	assert.Equal(t, "tasks.tasks.assign", string(enums.PermTasksAssign))
	assert.Equal(t, "tasks.tasks.transition", string(enums.PermTasksTransition))
	assert.Equal(t, "tasks.tasks.reopen", string(enums.PermTasksReopen))

	var hasTeams, hasTasks bool
	for _, e := range enums.PermissionCatalog {
		switch e.Code {
		case "tasks.teams":
			hasTeams = true
		case "tasks.tasks":
			hasTasks = true
		}
	}
	assert.True(t, hasTeams, "the tasks.teams entity is left intact")
	assert.True(t, hasTasks, "the tasks work-item entity is in the catalog")
}

// AC: the values.edit constant is exactly what the cell mutation enforces
// (backend-workspaces upsertBoardValue @hasPerm workspaces.values.edit), so the
// documented purpose matches enforcement.
func TestWorkspacesValues_EditMatchesCellMutation(t *testing.T) {
	assert.Equal(t, "workspaces.values.edit", string(enums.PermWorkspacesValuesEdit))
	assert.Equal(t, "workspaces.values.view", string(enums.PermWorkspacesValuesView))
	// values.edit is a leaf of the values entity, distinct from boards structure.
	assert.False(t, middleware.HasPermission([]string{string(enums.PermWorkspacesBoardsEdit)}, string(enums.PermWorkspacesValuesEdit)),
		"editing board STRUCTURE does not imply editing cell VALUES (sibling entities)")
}
