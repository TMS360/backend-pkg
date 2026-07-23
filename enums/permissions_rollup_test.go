package enums_test

import (
	"sort"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullAccountingLeaves is every concrete leaf under the accounting module,
// derived from the catalog itself so the test tracks the catalog.
func fullAccountingLeaves() []string { return enums.ExpandPermissions([]string{"accounting"}) }

// AC: when every entity+action of a module is held, the whole module collapses
// to the single module code — the exact symptom being fixed (getMe returned the
// 33-line expanded accounting list instead of just "accounting").
func TestRollup_FullModuleCollapsesToModuleCode(t *testing.T) {
	leaves := fullAccountingLeaves()
	require.NotEmpty(t, leaves)

	got := enums.RollupPermissions(leaves)
	assert.Equal(t, []string{"accounting"}, got)
}

// AC: a partial module does NOT collapse to the module; complete entities roll
// up to their entity code, and the incomplete entity's actions stay as leaves.
// Mirrors the real payload where shipments lacked other_pay & driver_expense.
func TestRollup_PartialModuleRollsUpCompleteEntitiesOnly(t *testing.T) {
	full := enums.ExpandPermissions([]string{"shipments"})
	// Drop every leaf of two entities so shipments is incomplete but the rest
	// of its entities are still fully held.
	keep := make([]string, 0, len(full))
	for _, leaf := range full {
		if hasPrefix(leaf, "shipments.other_pay.") || hasPrefix(leaf, "shipments.driver_expense.") {
			continue
		}
		keep = append(keep, leaf)
	}

	got := enums.RollupPermissions(keep)

	assert.NotContains(t, got, "shipments", "partial module must not collapse")
	// Complete entities rolled up to their entity code.
	assert.Contains(t, got, "shipments.shipments")
	assert.Contains(t, got, "shipments.legs")
	assert.Contains(t, got, "shipments.audit")
	// The dropped entities appear nowhere (neither entity nor leaf).
	for _, c := range got {
		assert.NotContains(t, c, "other_pay")
		assert.NotContains(t, c, "driver_expense")
		// No raw leaf survives for a complete entity.
		assert.NotEqual(t, "shipments.shipments.view", c)
	}
}

// AC: a lone complete entity under an otherwise-incomplete module rolls up to
// the entity code (not the module, not the leaves).
func TestRollup_SingleCompleteEntity(t *testing.T) {
	got := enums.RollupPermissions(enums.ExpandPermissions([]string{"accounting.invoices"}))
	assert.Equal(t, []string{"accounting.invoices"}, got)
}

// AC: subsumes the old prefix-only CompactHierarchy — a leaf whose ancestor is
// already present is dropped as redundant.
func TestRollup_DropsChildWhenAncestorPresent(t *testing.T) {
	got := enums.RollupPermissions([]string{"accounting", "accounting.invoices.create"})
	assert.Equal(t, []string{"accounting"}, got)
}

// AC: flat custom codes (no catalog parent) are always preserved, never folded
// away and never dropped — otherwise a governed grant would silently vanish.
func TestRollup_PreservesFlatCustomCodes(t *testing.T) {
	flat := string(enums.PermTripReassignCommitted)
	in := append(fullAccountingLeaves(), flat)

	got := enums.RollupPermissions(in)
	assert.Contains(t, got, "accounting")
	assert.Contains(t, got, flat)
}

// AC: unrecognised codes are passed through, never silently dropped.
func TestRollup_PreservesUnknownCodes(t *testing.T) {
	got := enums.RollupPermissions([]string{"accounting.invoices.view", "made.up.code"})
	assert.Contains(t, got, "made.up.code")
}

// AC: rollup never loses authority — every input leaf is still granted (via
// prefix implication) by the rolled-up result.
func TestRollup_NeverDropsAuthority(t *testing.T) {
	in := append(fullAccountingLeaves(), enums.ExpandPermissions([]string{"drivers"})...)
	in = append(in, "shipments.shipments.view", string(enums.PermTripReassignCommitted))

	got := enums.RollupPermissions(in)
	for _, leaf := range in {
		assert.Truef(t, middleware.HasPermission(got, leaf),
			"rolled-up set must still grant %q", leaf)
	}
}

// AC: Expand resolves module -> all leaves, entity -> its leaves, and leaves /
// flat / unknown codes unchanged.
func TestExpand_ResolvesToLeaves(t *testing.T) {
	// module expands to a superset of one of its entities' leaves.
	mod := enums.ExpandPermissions([]string{"accounting"})
	assert.Contains(t, mod, "accounting.invoices.view")
	assert.Contains(t, mod, "accounting.billing.view")
	for _, c := range mod {
		assert.NotEqual(t, "accounting", c, "module code itself must not appear in the expansion")
	}

	// entity expands to exactly its own leaves.
	ent := enums.ExpandPermissions([]string{"accounting.invoices"})
	assert.ElementsMatch(t, []string{
		"accounting.invoices.view", "accounting.invoices.create", "accounting.invoices.edit",
	}, ent)

	// leaf / flat / unknown pass through unchanged.
	pass := enums.ExpandPermissions([]string{
		"accounting.invoices.view", string(enums.PermTripReassignCommitted), "made.up.code",
	})
	assert.ElementsMatch(t, []string{
		"accounting.invoices.view", string(enums.PermTripReassignCommitted), "made.up.code",
	}, pass)
}

// AC: Expand and Rollup are inverses on catalog codes, so storing rolled-up and
// reading expanded (or vice-versa) round-trips losslessly.
func TestExpandRollup_RoundTrip(t *testing.T) {
	// Rollup(Expand(module)) == module
	assert.Equal(t, []string{"accounting"},
		enums.RollupPermissions(enums.ExpandPermissions([]string{"accounting"})))

	// Expand(Rollup(all leaves)) == all leaves (sorted)
	leaves := fullAccountingLeaves()
	rolled := enums.RollupPermissions(leaves)
	back := enums.ExpandPermissions(rolled)
	want := append([]string(nil), leaves...)
	sort.Strings(want)
	assert.Equal(t, want, back)
}

// AC: empty and whitespace-ish inputs are handled without panics and drop
// empty codes.
func TestRollupExpand_EmptyInputs(t *testing.T) {
	assert.Empty(t, enums.RollupPermissions(nil))
	assert.Empty(t, enums.RollupPermissions([]string{"", ""}))
	assert.Empty(t, enums.ExpandPermissions([]string{""}))
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
