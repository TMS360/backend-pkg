package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1335 — every permission code the library DECLARES must be a code the
// library can GRANT.
//
// The class of bug this closes: a Perm* constant is a string, and a grantable
// code is derived from PermissionCatalog (an entry's Code plus one leaf per
// Action). Nothing tied the two together, so `tasks.view` / `tasks.create` /
// `tasks.assign` / `tasks.transition` / `tasks.reopen` sat in the constants and
// in backend-tasks' @hasPerm directives while IsValidPermissionCode said false —
// assignPermissionsTo{Role,User} refused them, the role matrix could not show
// them, and a per-action deny was impossible. Access only worked because the
// whole `tasks` module was granted and HasPermission matches ancestors.
//
// This test reads the constants out of the SOURCE rather than a hand-kept list,
// so a constant added next year is covered without anybody remembering to add it
// here.

// permConstants parses enums/user_permissions.go and returns every
// `PermXxx UserPermissionEnum = "code"` declaration as name → code.
func permConstants(t *testing.T) map[string]string {
	t.Helper()

	path := filepath.Join("..", "enums", "user_permissions.go")
	src, err := os.ReadFile(path)
	require.NoError(t, err, "the permission source must be readable — this test is the guardrail over it")

	file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	require.NoError(t, err)

	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Perm") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				code, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				out[name.Name] = code
			}
		}
	}
	return out
}

// AC1 (library half): no code exists on one side only — every declared constant
// is a grantable catalog code.
func TestEveryPermConstantIsGrantable(t *testing.T) {
	consts := permConstants(t)
	require.NotEmpty(t, consts, "the parser must find the Perm* constants, or this guardrail is vacuous")

	for name, code := range consts {
		assert.Truef(t, enums.IsValidPermissionCode(code),
			"%s = %q is declared but not grantable: add it to PermissionCatalog (an entity's Actions) "+
				"or fix the constant — a code nobody can grant is a phantom", name, code)
	}
}

// The tasks work-item codes specifically: declared, grantable, and satisfied by
// the module grant every tenant already holds (so no migration is needed).
func TestTasksWorkItemCodesAreGrantableAndImpliedByTheModule(t *testing.T) {
	codes := []string{
		string(enums.PermTasksView), string(enums.PermTasksCreate), string(enums.PermTasksAssign),
		string(enums.PermTasksTransition), string(enums.PermTasksReopen),
	}
	for _, c := range codes {
		assert.Truef(t, enums.IsValidPermissionCode(c), "%s must be grantable", c)
		assert.Truef(t, strings.HasPrefix(c, "tasks.tasks."), "%s must live under the tasks.tasks entity", c)
		// What every existing tenant actually holds is the module row.
		assert.Truef(t, middleware.HasPermission([]string{"tasks"}, c),
			"the `tasks` module grant must still satisfy %s — otherwise this change needs a data migration", c)
		// The entity grant alone is enough for its own leaves.
		assert.Truef(t, middleware.HasPermission([]string{"tasks.tasks"}, c), "the entity grant must satisfy %s", c)
		// A neighbouring entity must NOT satisfy them.
		assert.Falsef(t, middleware.HasPermission([]string{"tasks.teams"}, c),
			"task-team rights must not leak into work items (%s)", c)
	}
}

// Granting the module still expands to every leaf below it, and now that the
// work-item entity exists those leaves are part of the expansion — so a granular
// deny (module minus one action) becomes expressible, which is the point.
func TestTasksModuleExpandsToWorkItemLeaves(t *testing.T) {
	leaves := enums.ExpandPermissions([]string{"tasks"})
	assert.Contains(t, leaves, "tasks.tasks.view")
	assert.Contains(t, leaves, "tasks.tasks.create")
	assert.Contains(t, leaves, "tasks.teams.view", "the task-teams leaves stay in the expansion")

	// A complete set still rolls back up to the module, so the compact shape every
	// service caches does not grow.
	assert.Equal(t, []string{"tasks"}, enums.RollupPermissions(leaves))

	// Minus one action it does NOT roll up to the module — the user keeps the rest
	// and the denied action stays denied.
	withoutCreate := make([]string, 0, len(leaves))
	for _, l := range leaves {
		if l != "tasks.tasks.create" {
			withoutCreate = append(withoutCreate, l)
		}
	}
	rolled := enums.RollupPermissions(withoutCreate)
	assert.NotContains(t, rolled, "tasks", "an incomplete module must not roll up to the module code")
	assert.False(t, middleware.HasPermission(rolled, "tasks.tasks.create"), "the denied action stays denied")
	assert.True(t, middleware.HasPermission(rolled, "tasks.tasks.view"), "the rest is kept")
}
