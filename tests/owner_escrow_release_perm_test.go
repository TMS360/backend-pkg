package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
)

// DEV-2821 — releasing a truck owner's escrow is refused to an ordinary
// accountant: only admin holds it by default.
func TestOwnerEscrowRelease_FlatAdminOnly(t *testing.T) {
	assertFlatPerm(t, enums.PermOwnerEscrowRelease, "owner_escrow_release", enums.UserRoleAdmin)
}
