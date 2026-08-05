package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

// PermsCacheTTL is the lifetime of a cached user perm list.
//
// tms-auth invalidates the key directly on every permission write, so the
// normal path is immediate. This TTL is the blast-radius cap for the cases
// direct invalidation cannot reach:
//
//   - a subgraph running against its OWN Redis instance (auth deletes only the
//     key in the Redis it is connected to);
//   - a Redis DEL that failed or was lost.
//
// DEV-1430: it used to be 24h, which turned a bad permission write into a
// day-long outage for the affected users even after ops had already fixed the
// grants — the fix was invisible to teams/loads/etc. because their cached copy
// never expired. 5 minutes is the agreed worst case (AC: ≤ 5 min); it costs at
// most one extra ResolveUserPerms round-trip per user per 5 minutes.
const PermsCacheTTL = 5 * time.Minute

// AuthServiceClient is the contract every service uses to fetch a user's
// effective perms from tms-auth. The concrete implementation in tms-auth's
// gRPC client wraps the ResolveUserPerms RPC.
type AuthServiceClient interface {
	ResolveUserPerms(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// PermResolver fronts the AuthServiceClient with a Redis cache. Read paths
// (HTTP middleware, internal callers) should use this — never call the
// AuthServiceClient directly.
type PermResolver struct {
	authClient AuthServiceClient
	sf         singleflight.Group
}

func NewPermResolver(authClient AuthServiceClient) *PermResolver {
	return &PermResolver{authClient: authClient}
}

// GetUserPerms returns the user's effective permission keys, hitting Redis
// first and falling back to the AuthServiceClient on cache miss. On fetch
// failure it returns an empty slice and the error so callers can fail-closed
// as "unresolved" (not as a real empty grant list).
//
// Concurrent misses for the same user are single-flighted so a page full of
// parallel GraphQL ops at TTL expiry does not stampede tms-auth (DEV-1555).
func (pr *PermResolver) GetUserPerms(ctx context.Context, userID uuid.UUID) ([]string, error) {
	key := cacheKey(userID)

	var cached []string
	if err := cache.GetGlobal(ctx, key, &cached); err == nil {
		return cached, nil
	} else if !errors.Is(err, redis.Nil) {
		slog.Debug("perm cache read failed", "userID", userID, "err", err)
	}

	v, err, _ := pr.sf.Do(userID.String(), func() (interface{}, error) {
		// Re-check cache inside the singleflight so waiters after the first
		// successful resolve do not each hit auth again.
		var again []string
		if err := cache.GetGlobal(ctx, key, &again); err == nil {
			return again, nil
		}

		perms, err := pr.authClient.ResolveUserPerms(ctx, userID)
		if err != nil {
			slog.Warn("failed to resolve user perms from auth-service", "userID", userID, "err", err)
			return []string{}, err
		}

		if perms == nil {
			perms = []string{}
		}

		if len(perms) > 0 {
			if cacheErr := cache.SetGlobal(ctx, key, perms, PermsCacheTTL); cacheErr != nil {
				slog.Warn("failed to cache user perms", "userID", userID, "err", cacheErr)
			}
		}

		return perms, nil
	})
	if err != nil {
		return []string{}, err
	}
	perms, _ := v.([]string)
	if perms == nil {
		perms = []string{}
	}
	return perms, nil
}

// InvalidateUserPerms removes one user's cached perms. Call this after any
// mutation that changes the user's role or direct perm grants.
func InvalidateUserPerms(ctx context.Context, userID uuid.UUID) error {
	return InvalidateUsersPerms(ctx, []uuid.UUID{userID})
}

// InvalidateUsersPerms drops cached perms for a batch of users. Use this
// after role-level mutations, where every user holding the role needs a
// fresh read on the next request.
//
// It deletes BOTH key shapes for every user:
//
//   - `user_perms:{userID}` — what this version reads and writes;
//   - `{companyID}:user_perms:{userID}` — the tenant-prefixed key that services
//     still running an older backend-pkg read and write.
//
// The legacy delete is what makes a permission fix land during a rollout.
// backend-pkg is pinned per service, so tms-auth picks up a cache-key change
// long before the subgraphs do. Deleting only the new shape meant every
// un-bumped subgraph kept serving its stale copy, with nothing to fall back on
// but that version's 24h TTL — the AC3 failure QA caught on 2026-07-29. One
// extra DEL removes the lock-step-deploy requirement entirely; drop it once
// every PermResolver consumer runs this version or later.
func InvalidateUsersPerms(ctx context.Context, userIDs []uuid.UUID) error {
	return cache.DeleteKeysGlobal(ctx, PermCacheKeys(ctx, userIDs))
}

// PermCacheKeys returns every Redis key that may hold these users' cached perms
// — the canonical one plus, while the fleet is mid-rollout, the legacy
// tenant-prefixed one. Exported so invalidation stays testable without Redis and
// so ops tooling can clear the same keys by hand.
func PermCacheKeys(ctx context.Context, userIDs []uuid.UUID) []string {
	if len(userIDs) == 0 {
		return nil
	}
	// The tenant prefix an older reader would have used. It is the acting user's
	// company, which for a permission write is the tenant being edited — the one
	// case that matters, since a super_admin's own company is empty and yields
	// the bare key we already delete.
	legacyPrefix := ""
	if actor, _ := ctx.Value(consts.ActorCtx).(*consts.Actor); actor != nil {
		if cid := actor.GetCompanyID(); cid != nil {
			legacyPrefix = cid.String()
		}
	}

	keys := make([]string, 0, len(userIDs)*2)
	for _, uid := range userIDs {
		key := cacheKey(uid)
		keys = append(keys, key)
		if legacyPrefix != "" {
			keys = append(keys, cache.ScopedKey(legacyPrefix, key))
		}
	}
	return keys
}

// cacheKey is deliberately company-free: userID is globally unique, and the key
// must be identical for the subgraph that caches it and the auth service that
// invalidates it.
func cacheKey(userID uuid.UUID) string {
	return fmt.Sprintf("user_perms:%s", userID.String())
}
