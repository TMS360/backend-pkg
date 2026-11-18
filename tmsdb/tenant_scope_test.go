package tmsdb

import (
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rule this file defends: a service that named a tenant sees that tenant
// and no other.
//
// Every system actor used to run unscoped, and the internal-token interceptor
// makes one out of any service-to-service call. backend-files therefore
// answered GetFile and Download for ANY company's file — the caller only had to
// know the uuid, and the proto comments claiming otherwise were wrong. What
// tells a genuine cross-tenant sweep from a call made for one tenant is whether
// a company came with it.

func TestDecideScope(t *testing.T) {
	company := uuid.New()

	t.Run("a service acting for one tenant is scoped to that tenant", func(t *testing.T) {
		actor := &consts.Actor{
			IsSystem: true,
			Claims:   &consts.UserClaims{CompanyID: &company},
		}

		decision, got := decideScope(actor)

		require.Equal(t, scopeCompany, decision, "x-company-id names the only tenant this call may read")
		assert.Equal(t, company, got)
	})

	t.Run("a cross-tenant sweep stays global", func(t *testing.T) {
		// The shape WithSystemActor produces: no JWT, so no claims at all.
		actor := &consts.Actor{ID: uuid.Nil, IsSystem: true}

		decision, _ := decideScope(actor)

		assert.Equal(t, scopeGlobal, decision,
			"the pollers and the outbox relay read every company on purpose")
	})

	t.Run("a system actor with claims but no company stays global", func(t *testing.T) {
		actor := &consts.Actor{IsSystem: true, Claims: &consts.UserClaims{}}

		decision, _ := decideScope(actor)

		assert.Equal(t, scopeGlobal, decision)
	})

	t.Run("an ordinary user is scoped to their own company", func(t *testing.T) {
		actor := &consts.Actor{
			ID:     uuid.New(),
			Claims: &consts.UserClaims{CompanyID: &company, Roles: []string{"dispatcher"}},
		}

		decision, got := decideScope(actor)

		require.Equal(t, scopeCompany, decision)
		assert.Equal(t, company, got)
	})

	t.Run("a super admin stays global", func(t *testing.T) {
		actor := &consts.Actor{
			ID:     uuid.New(),
			Claims: &consts.UserClaims{CompanyID: &company, Roles: []string{"super_admin"}},
		}

		decision, _ := decideScope(actor)

		assert.Equal(t, scopeGlobal, decision,
			"a wiped role table must still leave ops a way in")
	})

	t.Run("a user with no company is refused rather than run unscoped", func(t *testing.T) {
		actor := &consts.Actor{ID: uuid.New(), Claims: &consts.UserClaims{Roles: []string{"dispatcher"}}}

		decision, _ := decideScope(actor)

		assert.Equal(t, scopeReject, decision,
			"failing open here would hand one tenant the whole table")
	})

	t.Run("a user with no claims at all is refused, not a panic", func(t *testing.T) {
		actor := &consts.Actor{ID: uuid.New()}

		decision, _ := decideScope(actor)

		assert.Equal(t, scopeReject, decision)
	})
}
