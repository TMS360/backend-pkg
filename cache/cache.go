package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
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
	actor, _ := middleware.GetActor(ctx)
	if actor == nil {
		return key
	}
	companyID := actor.GetCompanyID()
	if companyID == nil {
		return key
	}
	return fmt.Sprintf("%s:%s", companyID.String(), key)
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

func Pipeline() redis.Pipeliner {
	return client.Pipeline()
}
