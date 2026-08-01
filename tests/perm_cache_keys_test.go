package tests

import (
	"context"
	"testing"

	pkgauth "github.com/TMS360/backend-pkg/auth"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// DEV-1430 QA (AC3). backend-pkg is pinned per service, so tms-auth runs a newer
// version than the subgraphs for as long as it takes to bump them. A permission
// write invalidates the cache from auth, but the stale copy lives in whatever key
// shape the READER's version uses:
//
//	old readers (teams, loads, …): {companyID}:user_perms:{userID}
//	this version:                             user_perms:{userID}
//
// Deleting only the new shape is why teams kept authorizing a user whose access
// had just been stripped. Invalidation must therefore clear both until every
// PermResolver consumer is on this version.

func actorCtx(companyID *uuid.UUID) context.Context {
	return context.WithValue(context.Background(), consts.ActorCtx, &consts.Actor{
		Claims: &consts.UserClaims{CompanyID: companyID},
	})
}

func TestPermCacheKeys_TenantActor_ClearsBothKeyShapes(t *testing.T) {
	companyID := uuid.New()
	userID := uuid.New()

	keys := pkgauth.PermCacheKeys(actorCtx(&companyID), []uuid.UUID{userID})

	require.Contains(t, keys, "user_perms:"+userID.String(),
		"the canonical key this version reads must be cleared")
	require.Contains(t, keys, companyID.String()+":user_perms:"+userID.String(),
		"the legacy tenant-prefixed key that un-bumped subgraphs read must be cleared too")
	require.Len(t, keys, 2, "exactly two shapes, no wildcard scans")
}

func TestPermCacheKeys_Batch_CoversEveryUser(t *testing.T) {
	companyID := uuid.New()
	users := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

	keys := pkgauth.PermCacheKeys(actorCtx(&companyID), users)

	require.Len(t, keys, len(users)*2)
	for _, u := range users {
		require.Contains(t, keys, "user_perms:"+u.String())
		require.Contains(t, keys, companyID.String()+":user_perms:"+u.String())
	}
}

// A super_admin (or an internal caller) has no company of their own, so there is
// no legacy prefix to derive — the canonical key is still always cleared, which
// is what every bumped consumer reads.
func TestPermCacheKeys_ActorWithoutCompany_ClearsCanonicalKeyOnly(t *testing.T) {
	userID := uuid.New()

	for name, ctx := range map[string]context.Context{
		"no actor":         context.Background(),
		"actor no company": actorCtx(nil),
	} {
		t.Run(name, func(t *testing.T) {
			keys := pkgauth.PermCacheKeys(ctx, []uuid.UUID{userID})
			require.Equal(t, []string{"user_perms:" + userID.String()}, keys)
		})
	}
}

func TestPermCacheKeys_NoUsers_NoKeys(t *testing.T) {
	require.Empty(t, pkgauth.PermCacheKeys(context.Background(), nil))
}
