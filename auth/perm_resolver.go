package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// PermsCacheTTL is the lifetime of a cached user perm list. Permission
// mutations invalidate the key directly; this TTL is the safety net for
// stale writers that forget to invalidate.
const PermsCacheTTL = 24 * time.Hour

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
}

func NewPermResolver(authClient AuthServiceClient) *PermResolver {
	return &PermResolver{authClient: authClient}
}

// GetUserPerms returns the user's effective permission keys, hitting Redis
// first and falling back to the AuthServiceClient on cache miss. On fetch
// failure it returns an empty slice and the error so callers can fail-closed.
func (pr *PermResolver) GetUserPerms(ctx context.Context, userID uuid.UUID) ([]string, error) {
	key := cacheKey(userID)

	var cached []string
	if err := cache.Get(ctx, key, &cached); err == nil {
		return cached, nil
	} else if !errors.Is(err, redis.Nil) {
		slog.Debug("perm cache read failed", "userID", userID, "err", err)
	}

	perms, err := pr.authClient.ResolveUserPerms(ctx, userID)
	if err != nil {
		slog.Warn("failed to resolve user perms from auth-service", "userID", userID, "err", err)
		return []string{}, err
	}

	if perms == nil {
		perms = []string{}
	}
	if cacheErr := cache.Set(ctx, key, perms, PermsCacheTTL); cacheErr != nil {
		slog.Warn("failed to cache user perms", "userID", userID, "err", cacheErr)
	}

	return perms, nil
}

// InvalidateUserPerms removes one user's cached perms. Call this after any
// mutation that changes the user's role or direct perm grants.
func InvalidateUserPerms(ctx context.Context, userID uuid.UUID) error {
	return cache.Delete(ctx, cacheKey(userID))
}

// InvalidateUsersPerms drops cached perms for a batch of users. Use this
// after role-level mutations, where every user holding the role needs a
// fresh read on the next request.
func InvalidateUsersPerms(ctx context.Context, userIDs []uuid.UUID) error {
	if len(userIDs) == 0 {
		return nil
	}
	keys := make([]string, len(userIDs))
	for i, uid := range userIDs {
		keys[i] = cacheKey(uid)
	}
	return cache.DeleteKeys(ctx, keys)
}

func cacheKey(userID uuid.UUID) string {
	return fmt.Sprintf("user_perms:%s", userID.String())
}
