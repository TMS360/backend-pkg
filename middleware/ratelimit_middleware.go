package middleware

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/ratelimit"
	"github.com/gin-gonic/gin"
)

// Authenticated-traffic rate limiting (DEV-1485).
//
// Until now only the guest path was throttled; every request behind a user
// token was unlimited, so one script / stolen token / runaway loop could hammer
// prod unnoticed. This wires the existing, production-proven backend-pkg/ratelimit
// counter into the shared HTTP edge, keyed per (user, IP).
//
// It reuses ratelimit.Allow, which counts against a single Redis key shared by
// every replica and every service — so the ceiling is global for a given
// (user, IP), not per-subgraph. This middleware only decides who is counted and
// what happens on a throttle; it does not implement a limiter.

const (
	// EnvAuthRateLimitMax overrides the per-(user, IP) request ceiling per window.
	EnvAuthRateLimitMax = "RATE_LIMIT_AUTH_MAX"
	// EnvAuthRateLimitWindow overrides the counting window (a Go duration string,
	// e.g. "1m", "30s").
	EnvAuthRateLimitWindow = "RATE_LIMIT_AUTH_WINDOW"

	// DefaultAuthRateLimitMax is the request ceiling per (user, IP) per window.
	//
	// It is set well above real peak usage on purpose: the heaviest pages
	// (invoice batches, then the audit board) fire dozens of GraphQL operations
	// per view, and a user with several tabs open multiplies that. The ceiling
	// guards against a runaway loop or a stolen token doing thousands of requests
	// a minute — not against a busy human. Tune per environment via
	// RATE_LIMIT_AUTH_MAX once real peak op/min has been measured on prod.
	DefaultAuthRateLimitMax = 600
	// DefaultAuthRateLimitWindow is the counting window for the ceiling above.
	DefaultAuthRateLimitWindow = time.Minute

	// maxBodyPeek bounds how much of the request body we read to recover the
	// GraphQL operation name for the throttle log line. Only read on the abort
	// path, so it never touches a request we let through.
	maxBodyPeek = 1 << 20 // 1 MiB
)

// RateLimiterFunc counts one hit against key and reports whether it stays within
// limit for the window. ratelimit.Allow is the production implementation; the
// seam exists so the middleware can be unit-tested without Redis.
type RateLimiterFunc func(ctx context.Context, key string, limit int, window time.Duration) (bool, error)

type rateLimitConfig struct {
	max    int
	window time.Duration
	allow  RateLimiterFunc
}

// RateLimitOption customizes RateLimitAuthenticated.
type RateLimitOption func(*rateLimitConfig)

// WithAuthRateLimit overrides the ceiling and window in code. Non-positive
// values are ignored so callers can pass one without the other. Env vars still
// win when set; use this for a service-specific hard default.
func WithAuthRateLimit(max int, window time.Duration) RateLimitOption {
	return func(c *rateLimitConfig) {
		if max > 0 {
			c.max = max
		}
		if window > 0 {
			c.window = window
		}
	}
}

// WithRateLimiter injects the limiter implementation (defaults to
// ratelimit.Allow). Used by tests to make the throttle decision deterministic.
func WithRateLimiter(fn RateLimiterFunc) RateLimitOption {
	return func(c *rateLimitConfig) {
		if fn != nil {
			c.allow = fn
		}
	}
}

// RateLimitAuthenticated throttles authenticated user traffic per (user, IP).
// Install it right AFTER IdentifyUser so the actor and the resolved client
// origin are already on the context.
//
// Who is counted: only real user tokens. Anonymous requests (no actor), guests
// (their own limiter runs at the resolver), and system / internal
// service-to-service actors are passed through untouched — so a login page, a
// share link, or a background job is never throttled by this ceiling.
//
// On a throttle it aborts with 429 + Retry-After BEFORE the GraphQL handler
// runs, so a rejected mutation cannot partially apply, and logs the actor id,
// IP and operation name so "who got throttled and when" is answerable from logs
// alone.
//
// Fail-open: ratelimit.Allow returns true when Redis is unavailable, so an infra
// outage lets traffic through rather than locking the product out.
func RateLimitAuthenticated(opts ...RateLimitOption) gin.HandlerFunc {
	cfg := &rateLimitConfig{
		max:    envInt(EnvAuthRateLimitMax, DefaultAuthRateLimitMax),
		window: envDuration(EnvAuthRateLimitWindow, DefaultAuthRateLimitWindow),
		allow:  ratelimit.Allow,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(c *gin.Context) {
		reqCtx := c.Request.Context()

		actor, err := GetActor(reqCtx)
		if err != nil || actor == nil || actor.IsGuest || actor.IsSystem {
			c.Next()
			return
		}

		// (user, IP). Keying on the user means one office sharing a public IP is
		// not throttled as a group; adding the IP means a single leaked token
		// used from many machines is still bounded per source. A missing IP
		// (origin not resolvable) falls back to user-only rather than a shared
		// blank bucket.
		ip := ""
		if origin := GetClientOrigin(reqCtx); origin != nil {
			ip = origin.IP
		}
		key := "auth:" + actor.ID.String()
		if ip != "" {
			key += ":" + ip
		}

		allowed, rlErr := cfg.allow(reqCtx, key, cfg.max, cfg.window)
		if rlErr != nil {
			// Fail open, but make the infra blip visible.
			slog.WarnContext(reqCtx, "auth rate limit check failed — allowing request",
				"userID", actor.ID, "err", rlErr)
		}
		if allowed {
			c.Next()
			return
		}

		retryAfter := int(cfg.window.Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}

		// One structured line per throttle: this is the raw material the security
		// dashboard consumes to detect API abuse (it is otherwise blind to
		// authenticated traffic).
		slog.WarnContext(reqCtx, "auth rate limit exceeded",
			"userID", actor.ID,
			"ip", ip,
			"operation", operationName(c),
			"limit", cfg.max,
			"window", cfg.window.String(),
		)

		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":   consts.CodeRateLimited,
			"message": consts.MsgRateLimited,
		})
	}
}

// operationName best-effort recovers the GraphQL operation name from the request
// body for the throttle log. It is only called on the abort path, where the
// request is rejected and the body is not read by any downstream handler, so
// consuming it here is safe. Non-GraphQL requests and batched operations simply
// yield "".
func operationName(c *gin.Context) string {
	if c.Request == nil || c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyPeek))
	if err != nil {
		return ""
	}
	var payload struct {
		OperationName string `json:"operationName"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return payload.OperationName
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}
