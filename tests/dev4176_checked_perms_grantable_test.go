package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/permcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-4176 — every right a server checks can be ticked in the role editor.

var dev4176Dotted = []enums.UserPermissionEnum{
	enums.PermAccountingInvoicesVoid,
	enums.PermAccountingPayBatchesUnpost,
	enums.PermAccountingPayBatchesRestore,
	enums.PermAccountingRelayFuelView,
	enums.PermAccountingReportConfigsView,
	enums.PermAccountingReportConfigsEdit,
	enums.PermSettingsDocTypesDelete,
}

var dev4176Standalone = []enums.UserPermissionEnum{
	enums.PermComplianceReceiveAlerts,
	enums.PermChatRoomsCreateSecret,
	enums.PermChatRoomsAudit,
}

// AC1: the new switches are served by getPermissions (catalog + custom list)
// under their module: accounting/settings as catalog leaves, compliance/chat as
// standalone switches.
func TestDEV4176_NewRightsAreGrantable(t *testing.T) {
	for _, c := range append(append([]enums.UserPermissionEnum{}, dev4176Dotted...), dev4176Standalone...) {
		assert.Truef(t, enums.IsValidPermissionCode(string(c)), "%q must be grantable in Settings -> Roles", c)
	}
	for _, c := range dev4176Standalone {
		assert.Truef(t, enums.IsCustomPermissionCode(string(c)), "%q must be a standalone switch", c)
	}
	parents := map[string]string{}
	for _, e := range enums.PermissionCatalog {
		parents[e.Code] = e.ParentCode
	}
	assert.Equal(t, "accounting", parents["accounting.relay_fuel"])
	assert.Equal(t, "accounting", parents["accounting.report_configs"])
}

// AC2: a role with only invoices view + void can void; without void it is refused.
// A role holding the whole Invoices entity also passes (edge case in the MR).
func TestDEV4176_InvoiceVoidIsItsOwnRight(t *testing.T) {
	void := string(enums.PermAccountingInvoicesVoid)
	assert.True(t, middleware.HasPermission([]string{"accounting.invoices.view", void}, void))
	assert.False(t, middleware.HasPermission([]string{"accounting.invoices.view", "accounting.invoices.edit"}, void))
	assert.True(t, middleware.HasPermission([]string{"accounting.invoices"}, void))
	assert.True(t, middleware.HasPermission([]string{"accounting"}, void))
}

// AC3: holding only the alerts right satisfies the tms-files recipient check;
// office modules do not.
func TestDEV4176_ReceiveAlertsExactMatch(t *testing.T) {
	code := string(enums.PermComplianceReceiveAlerts)
	assert.Equal(t, "compliance.receive-alerts", code, "tms-files checks this exact spelling")
	assert.True(t, middleware.HasPermission([]string{code}, code))
	for _, m := range enums.ModulePermissionCodes() {
		assert.Falsef(t, middleware.HasPermission([]string{m}, code), "module %q must not imply %q", m, code)
	}
	for _, c := range dev4176Standalone {
		assert.Falsef(t, middleware.HasPermission(enums.ModulePermissionCodes(), string(c)),
			"the office module baseline must not imply standalone %q", c)
	}
}

// AC4: the built-in roles that passed before keep passing.
func TestDEV4176_DefaultHoldersKeepTheirActions(t *testing.T) {
	d := enums.DefaultRolePermissions()
	holds := func(role enums.UserRoleEnum, code enums.UserPermissionEnum) bool {
		return middleware.HasPermission(d[role], string(code))
	}
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting} {
		for _, c := range dev4176Dotted {
			assert.Truef(t, holds(role, c), "%s must hold %q", role, c)
		}
	}
	want := map[enums.UserPermissionEnum][]enums.UserRoleEnum{
		enums.PermComplianceReceiveAlerts: {enums.UserRoleAdmin, enums.UserRoleAccounting},
		enums.PermChatRoomsCreateSecret:   {enums.UserRoleAdmin, enums.UserRoleHr},
		enums.PermChatRoomsAudit:          {enums.UserRoleAdmin, enums.UserRoleHr, enums.UserRoleAuditor},
	}
	for code, roles := range want {
		got := []enums.UserRoleEnum{}
		for role := range d {
			if holds(role, code) {
				got = append(got, role)
			}
		}
		assert.ElementsMatchf(t, roles, got, "default holders of %q", code)
	}
}

func TestDEV4176_IsKnownPermission(t *testing.T) {
	assert.True(t, enums.IsKnownPermission("accounting.invoices.void"))
	assert.True(t, enums.IsKnownPermission("shipments"))
	assert.True(t, enums.IsKnownPermission(string(enums.PermMigrationUnlock)), "dev-team-only codes are known")
	assert.False(t, enums.IsValidPermissionCode(string(enums.PermMigrationUnlock)), "...but never grantable")
	assert.False(t, enums.IsKnownPermission("dispatch.loads.edit"))
}

// Every Perm* constant the package declares is known — a code defined here but
// missing from the list is the same bug one layer down.
func TestDEV4176_EveryPermConstantIsKnown(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join("..", "enums", "user_permissions.go"), nil, 0)
	require.NoError(t, err)
	seen := 0
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		id, ok := vs.Type.(*ast.Ident)
		if !ok || id.Name != "UserPermissionEnum" {
			return true
		}
		for i, v := range vs.Values {
			lit, ok := v.(*ast.BasicLit)
			if !ok {
				continue
			}
			code, err := strconv.Unquote(lit.Value)
			require.NoError(t, err)
			seen++
			assert.Truef(t, enums.IsKnownPermission(code), "%s = %q is not in the role editor list", vs.Names[i].Name, code)
		}
		return true
	})
	assert.Greater(t, seen, 100)
}

// The build check: unknown codes in a schema or a Go permission call are found
// (hyphenated codes included); known codes, comments and generated files are not.
func TestDEV4176_PermcheckFindsUnknownCodes(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("schema/a.graphqls", `type Query {
  ok: Int @hasPerm(perms: ["accounting.invoices.void", "shipments"])
  bad: Int @hasPerm(perms: [
     "dispatch.loads.edit"
  ])
}`)
	write("internal/svc.go", `package svc
func f() {
	_ = middleware.HasPermission(p, "chat.rooms.audit")
	_ = middleware.HasPermission(p, "chat.rooms.nope")
	// middleware.HasPermission(p, "ignored.in.comment")
	_ = perms.GetUsersByPermission(ctx, id, "compliance.receive-alertz", nil)
}`)
	write("internal/generated/generated.go", `x := HasPermission(p, "gen.only")`)

	found, err := permcheck.Unknown(root)
	require.NoError(t, err)
	codes := []string{}
	for _, f := range found {
		codes = append(codes, f.Code)
	}
	assert.ElementsMatch(t, []string{"dispatch.loads.edit", "chat.rooms.nope", "compliance.receive-alertz"}, codes)
}
