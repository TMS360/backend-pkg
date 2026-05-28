package auth

import (
	"sort"
	"strings"
)

// HasPermission reports whether userPerms grants required via hierarchical
// prefix matching. A perm grants itself and every descendant — "accounting"
// implies "accounting.invoices.create" — but never the reverse: holding
// "accounting.invoices.create" does NOT imply "accounting". Sibling perms
// do not imply each other.
func HasPermission(userPerms []string, required string) bool {
	if required == "" {
		return false
	}
	parts := strings.Split(required, ".")
	for i := 1; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], ".")
		for _, p := range userPerms {
			if p == prefix {
				return true
			}
		}
	}
	return false
}

// HasAnyPermission reports whether userPerms grants at least one of required.
func HasAnyPermission(userPerms []string, required []string) bool {
	for _, req := range required {
		if HasPermission(userPerms, req) {
			return true
		}
	}
	return false
}

// HasAllPermissions reports whether userPerms grants every required perm.
func HasAllPermissions(userPerms []string, required []string) bool {
	for _, req := range required {
		if !HasPermission(userPerms, req) {
			return false
		}
	}
	return true
}

// CompactHierarchy removes child perms whose ancestor is already in the set,
// returning a deduped, hierarchy-cleaned slice. Used at write-time so we never
// persist redundant rows.
func CompactHierarchy(perms []string) []string {
	seen := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		if p == "" {
			continue
		}
		seen[p] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		if hasAncestorIn(p, seen) {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func hasAncestorIn(key string, set map[string]struct{}) bool {
	parts := strings.Split(key, ".")
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], ".")
		if _, ok := set[prefix]; ok {
			return true
		}
	}
	return false
}
