package ratelimit

import (
	"context"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/go-redis/redis/v8"
)

// incrExpireScript atomically increments a counter and sets a TTL on first hit.
// Returning the post-increment count lets the caller compare against limit.
var incrExpireScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// Allow reports whether a request keyed by `key` is within `limit` per `window`.
// Uses Redis INCR+PEXPIRE via Lua for atomic fixed-window counting shared across
// replicas and microservices.
//
// Fail-open: if Redis is not initialized or returns an error, Allow returns true
// so transient infra problems don't lock out legitimate traffic. Callers that
// need fail-closed semantics should check the returned error.
func Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	rdb := cache.Client()
	if rdb == nil {
		return true, nil
	}

	n, err := incrExpireScript.Run(ctx, rdb, []string{"ratelimit:" + key}, window.Milliseconds()).Int64()
	if err != nil {
		return true, err
	}
	return n <= int64(limit), nil
}
