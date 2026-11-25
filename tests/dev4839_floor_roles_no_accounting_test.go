package tests

import (
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-4839 — the `accounting` module passes every money action by prefix, so
// the floor desks get no Accounting right at sign-up (the AZSAFI shape).

var dev4839FloorRoles = []enums.UserRoleEnum{
	enums.UserRoleDispatcher,
	enums.UserRoleTrackAndTrace,
	enums.UserRoleFleet,
	enums.UserRoleSafety,
	enums.UserRoleHr,
}

var dev4839MoneyRoles = []enums.UserRoleEnum{
	enums.UserRoleAdmin,
	enums.UserRoleAccounting,
	enums.UserRoleManager,
	enums.UserRoleAuditor,
	enums.UserRoleOther,
}

// AC1/AC2 (new company): the five floor roles hold no code equal to or under
// `accounting`, so voidRevisedInvoice and reversePayBatch are refused.
func TestDEV4839_FloorRolesHoldNoAccounting(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	for _, role := range dev4839FloorRoles {
		codes, ok := defaults[role]
		require.True(t, ok, "role %s missing from defaults", role)
		for _, c := range codes {
			assert.False(t, c == "accounting" || strings.HasPrefix(c, "accounting."),
				"role %s still holds %q", role, c)
		}
		assert.False(t, middleware.HasPermission(codes, "accounting.invoices.void"), "role %s can void an invoice", role)
		assert.False(t, middleware.HasPermission(codes, "accounting.pay_batches.unpost"), "role %s can unpost a pay batch", role)
	}
}

// AC3: admin, accounting, manager, auditor and other keep the whole module.
func TestDEV4839_MoneyRolesKeepAccounting(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	for _, role := range dev4839MoneyRoles {
		codes := defaults[role]
		assert.Contains(t, codes, "accounting", "role %s lost the accounting module", role)
		assert.True(t, middleware.HasPermission(codes, "accounting.invoices.void"), "role %s", role)
		assert.True(t, middleware.HasPermission(codes, "accounting.pay_batches.unpost"), "role %s", role)
	}
}

// AC5: dropping Accounting removes nothing else — every other module the
// dispatcher held before still passes (load list, dispatch, stops, expenses).
func TestDEV4839_OnlyAccountingIsDropped(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	for _, role := range dev4839FloorRoles {
		codes := defaults[role]
		for _, m := range enums.ModulePermissionCodes() {
			if m == "accounting" {
				continue
			}
			// fleet/teams/settings may be swapped for their entities on these
			// roles; then codes under the module must remain.
			assert.True(t, middleware.HasPermission(codes, m) || hasAnyUnder(codes, m),
				"role %s lost module %s", role, m)
		}
	}
	// Track and Trace still mirrors the dispatcher exactly.
	assert.ElementsMatch(t, defaults[enums.UserRoleDispatcher], defaults[enums.UserRoleTrackAndTrace])
}

func hasAnyUnder(codes []string, module string) bool {
	for _, c := range codes {
		if strings.HasPrefix(c, module+".") {
			return true
		}
	}
	return false
}

func isDEV4839FloorRole(role enums.UserRoleEnum) bool {
	for _, r := range dev4839FloorRoles {
		if r == role {
			return true
		}
	}
	return false
}
