package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/go-redis/redis/v8"
)

var client *redis.Client

func Init(rdb *redis.Client) {
	client = rdb
}

func Client() *redis.Client {
	return client
}

func buildKey(ctx context.Context, key string) string {
	// Read the actor straight from the context key (which lives in consts) rather
	// than via middleware.GetActor. Importing middleware here would form an import
	// cycle: middleware -> auth -> cache -> middleware. Behavior is identical.
	actor, _ := ctx.Value(consts.ActorCtx).(*consts.Actor)
	if actor == nil {
		return key
	}
	companyID := actor.GetCompanyID()
	if companyID == nil {
		return key
	}
	return ScopedKey(companyID.String(), key)
}

// ScopedKey builds the tenant-prefixed key for an EXPLICIT company, instead of
// deriving it from whoever happens to be acting. Same format buildKey produces,
// defined once so the two can never drift.
//
// Use it when a key is written by one actor and addressed by another — the
// classic case being an admin invalidating a cache entry that the target user
// wrote under their own company. An empty companyID yields the bare key.
func ScopedKey(companyID, key string) string {
	if companyID == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", companyID, key)
}

func Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	key = buildKey(ctx, key)

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal error: %w", err)
	}
	return client.Set(ctx, key, data, ttl).Err()
}

// SetNX sets the key only if it does not already exist.
// Returns true if the key was newly set, false if it already existed.
func SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	key = buildKey(ctx, key)

	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("cache: marshal error: %w", err)
	}
	return client.SetNX(ctx, key, data, ttl).Result()
}

func Get(ctx context.Context, key string, dest any) error {
	key = buildKey(ctx, key)

	data, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func Delete(ctx context.Context, key string) error {
	key = buildKey(ctx, key)
	return client.Del(ctx, key).Err()
}

func DeleteKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	for i, key := range keys {
		keys[i] = buildKey(ctx, key)
	}
	return client.Del(ctx, keys...).Err()
}

func Exists(ctx context.Context, key string) (bool, error) {
	key = buildKey(ctx, key)
	n, err := client.Exists(ctx, key).Result()
	return n > 0, err
}

// --- Global (actor-independent) keys ---------------------------------------
//
// The default Set/Get/Delete prefix every key with the *calling actor's*
// company id. That is right for tenant-scoped payloads, but wrong for a key
// that is already globally unique (one keyed by a UUID) and is written and
// invalidated by DIFFERENT actors.
//
// user_perms:{userID} is exactly that case: the subgraph middleware writes it
// as the user themselves (prefix = user's company), while tms-auth invalidates
// it as whoever edited the permissions — a tenant admin (same prefix, works)
// or a super_admin / system actor with no company_id at all (NO prefix, silent
// miss). The invalidation then hit a key nobody reads and the stale set stayed
// live until the TTL expired. The *Global variants skip buildKey so both sides
// address the same key regardless of who is acting.

func SetGlobal(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal error: %w", err)
	}
	return client.Set(ctx, key, data, ttl).Err()
}

func GetGlobal(ctx context.Context, key string, dest any) error {
	data, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func DeleteGlobal(ctx context.Context, key string) error {
	return client.Del(ctx, key).Err()
}

func DeleteKeysGlobal(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return client.Del(ctx, keys...).Err()
}

func Pipeline() redis.Pipeliner {
	return client.Pipeline()
}
