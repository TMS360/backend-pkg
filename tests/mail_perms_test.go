package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-2466 — the company mailbox codes are flat and default-deny, and the
// company admin holds all three.
//
// They shipped on no role at all. Being off by default is right for a driver,
// but it also locked out the one account that must never be locked out: the
// admin role's matrix is guarded (guardRoleMatrixAuthority), so the company
// admin had no checkbox to hand themselves mail and the mail pages answered
// "access denied" on a live feature. assertFlatPerm pins both halves at once —
// admin holds each code, and every other built-in role still does not.
func TestMailPerms_FlatAdminOnlyByDefault(t *testing.T) {
	assertFlatPerm(t, enums.PermMailView, "mail_view", enums.UserRoleAdmin)
	assertFlatPerm(t, enums.PermMailSend, "mail_send", enums.UserRoleAdmin)
	assertFlatPerm(t, enums.PermMailEdit, "mail_edit", enums.UserRoleAdmin)

	// The three are independent checkboxes: none implies another, so a tenant
	// can hand a custom role read-only mail without handing it the outbox.
	assert.False(t, middleware.HasPermission([]string{string(enums.PermMailView)}, string(enums.PermMailSend)),
		"view must not imply send")
	assert.False(t, middleware.HasPermission([]string{string(enums.PermMailEdit)}, string(enums.PermMailView)),
		"edit must not imply view")
}
