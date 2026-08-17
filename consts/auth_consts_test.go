package consts

import (
	"testing"

	"github.com/google/uuid"
)

// DEV-1732 (same family as the audit-writer crash): a system actor carries no
// JWT, so Claims is nil. The role checks reach into Claims.Roles, and callers
// evaluate them BEFORE the IsSystem branch — model/tenant_scoped.go does
// `if actor.IsSuperAdmin() || actor.IsSystem`, so a system actor writing a
// tenant-scoped row without an explicit company_id used to panic here instead of
// getting the readable error that branch exists to produce.
func TestRoleChecksAreNilSafeForSystemActor(t *testing.T) {
	actor := &Actor{ID: uuid.Nil, IsSystem: true}

	if actor.IsSuperAdmin() {
		t.Fatal("a system actor holds no super-admin role")
	}
	if actor.IsAdmin() {
		t.Fatal("a system actor holds no admin role")
	}
}

// The guard must not swallow a real role.
func TestRoleChecksStillMatchRealRoles(t *testing.T) {
	super := &Actor{ID: uuid.New(), Claims: &UserClaims{Roles: []string{"super_admin"}}}
	if !super.IsSuperAdmin() {
		t.Fatal("super_admin must be recognised")
	}
	if super.IsAdmin() {
		t.Fatal("super_admin is not the admin role")
	}

	admin := &Actor{ID: uuid.New(), Claims: &UserClaims{Roles: []string{"admin"}}}
	if !admin.IsAdmin() {
		t.Fatal("admin must be recognised")
	}
	if admin.IsSuperAdmin() {
		t.Fatal("admin is not super_admin")
	}
}
