package auth

import (
	"sort"
	"testing"
)

func TestHasPermission(t *testing.T) {
	cases := []struct {
		name     string
		userPerm []string
		required string
		want     bool
	}{
		{"exact module match", []string{"accounting"}, "accounting", true},
		{"parent implies entity", []string{"accounting"}, "accounting.invoices", true},
		{"parent implies action leaf", []string{"accounting"}, "accounting.invoices.create", true},
		{"entity implies its action", []string{"accounting.invoices"}, "accounting.invoices.view", true},
		{"sibling does not match", []string{"accounting.invoices"}, "accounting.vendors.view", false},
		{"child does not imply parent", []string{"accounting.invoices.create"}, "accounting", false},
		{"action does not imply sibling action", []string{"accounting.invoices.create"}, "accounting.invoices.view", false},
		{"empty user perms denies", []string{}, "accounting", false},
		{"empty required denies", []string{"accounting"}, "", false},
		{"unrelated module denies", []string{"hr"}, "accounting.invoices.view", false},
		{"ancestor several levels up", []string{"a.b.c"}, "a.b.c.d.e", true},
		{"near miss prefix is not a hierarchical ancestor", []string{"accountin"}, "accounting.invoices", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HasPermission(tc.userPerm, tc.required)
			if got != tc.want {
				t.Fatalf("HasPermission(%v, %q) = %v, want %v",
					tc.userPerm, tc.required, got, tc.want)
			}
		})
	}
}

func TestHasAllPermissions(t *testing.T) {
	user := []string{"accounting", "drivers.balance.view"}
	if !HasAllPermissions(user, []string{"accounting.invoices.view", "drivers.balance.view"}) {
		t.Fatal("expected all-permissions to pass for granted parent + leaf")
	}
	if HasAllPermissions(user, []string{"accounting.invoices.view", "drivers.scheduled_payments.view"}) {
		t.Fatal("expected all-permissions to fail when one required perm is missing")
	}
}

func TestHasAnyPermission(t *testing.T) {
	user := []string{"drivers.balance.view"}
	if !HasAnyPermission(user, []string{"accounting", "drivers.balance.view"}) {
		t.Fatal("expected any-permission to pass on a single grant")
	}
	if HasAnyPermission(user, []string{"accounting", "fleet"}) {
		t.Fatal("expected any-permission to fail when none granted")
	}
}

func TestCompactHierarchy(t *testing.T) {
	in := []string{
		"accounting",
		"accounting.invoices",
		"accounting.invoices.create",
		"drivers.balance.view",
		"drivers.balance.view", // dupe
		"",                     // empty filtered
	}
	got := CompactHierarchy(in)
	sort.Strings(got)
	want := []string{"accounting", "drivers.balance.view"}
	if len(got) != len(want) {
		t.Fatalf("CompactHierarchy len = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CompactHierarchy[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
