package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1485 — authenticated traffic must be throttled per (user, IP) by wiring
// the existing backend-pkg/ratelimit counter into the shared HTTP edge. These
// cover the decision the middleware owns — who is counted, the key shape, the
// 429 shape, and fail-open — with the limiter injected so no Redis is needed.

// --- test doubles ------------------------------------------------------------

type limiterCall struct {
	key    string
	limit  int
	window time.Duration
}

// fakeLimiter stands in for ratelimit.Allow. When counts is set it reproduces
// the real fixed-window semantics (n-th hit on a key allowed iff n <= limit);
// otherwise it returns the static `allow`/`err` pair.
type fakeLimiter struct {
	mu     sync.Mutex
	calls  []limiterCall
	counts map[string]int
	allow  bool
	err    error
}

func (f *fakeLimiter) fn(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, limiterCall{key, limit, window})
	if f.counts != nil {
		f.counts[key]++
		return f.counts[key] <= limit, f.err
	}
	return f.allow, f.err
}

// --- helpers -----------------------------------------------------------------

func newRateLimitEngine(mw gin.HandlerFunc, downstreamRan *bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw)
	r.POST("/query", func(c *gin.Context) {
		if downstreamRan != nil {
			*downstreamRan = true
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func userActor() *consts.Actor {
	id := uuid.New()
	return &consts.Actor{ID: id, Claims: &consts.UserClaims{UserID: id}}
}

// authedRequest builds a GraphQL POST carrying actor + client origin on its
// context, exactly as IdentifyUser would have left them. A nil actor models an
// anonymous request; an empty ip models an origin the edge could not resolve.
func authedRequest(actor *consts.Actor, ip, operation string) *http.Request {
	body := `{"operationName":"` + operation + `","query":"query ` + operation + ` { x }"}`
	r := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(body))
	ctx := context.Background()
	if actor != nil {
		ctx = middleware.WithActor(ctx, actor)
	}
	ctx = middleware.WithClientOrigin(ctx, &middleware.ClientOrigin{IP: ip})
	return r.WithContext(ctx)
}

func serve(eng *gin.Engine, actor *consts.Actor, ip string) int {
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, authedRequest(actor, ip, "Loads"))
	return w.Code
}

// --- tests -------------------------------------------------------------------

// AC1: a token past the ceiling gets 429 + Retry-After, the resolver never runs,
// and it recovers once the window resets.
func TestAuthRateLimit_ThrottlesOverCeilingAndRecovers(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	var downstreamRan bool
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(3, time.Minute),
		middleware.WithRateLimiter(lim.fn),
	), &downstreamRan)
	actor := userActor()

	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"), "hit %d within ceiling", i+1)
	}

	downstreamRan = false
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, authedRequest(actor, "203.0.113.9", "Loads"))

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.False(t, downstreamRan, "resolver must not run when the request is throttled")
	assert.Equal(t, "60", w.Header().Get("Retry-After"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, consts.CodeRateLimited, resp["error"], "429 must be distinguishable from the guest throttle")

	// Window resets → the same token is served again.
	lim.counts = map[string]int{}
	assert.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"))
}

// AC2: ordinary heavy use of the app stays well under the ceiling and is never
// throttled — the number is tuned above real peak op/min, not against it.
func TestAuthRateLimit_OrdinaryUseNeverThrottled(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(600, time.Minute),
		middleware.WithRateLimiter(lim.fn),
	), nil)
	actor := userActor()

	for i := 0; i < 200; i++ {
		require.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"), "request %d", i+1)
	}
}

// AC3: the bucket is per (user, IP). One abusive client hitting its own ceiling
// leaves other users — and the same user from another address — unaffected.
func TestAuthRateLimit_IsolatesByUserAndIP(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(1, time.Minute),
		middleware.WithRateLimiter(lim.fn),
	), nil)
	abusive := userActor()
	other := userActor()

	require.Equal(t, http.StatusOK, serve(eng, abusive, "1.1.1.1"))
	require.Equal(t, http.StatusTooManyRequests, serve(eng, abusive, "1.1.1.1"), "abuser exceeds own ceiling")
	require.Equal(t, http.StatusOK, serve(eng, other, "1.1.1.1"), "a different user on the same office IP is untouched")
	require.Equal(t, http.StatusOK, serve(eng, abusive, "2.2.2.2"), "the same user from another IP is a separate bucket")
}

// AC5 + AC6: guests keep their own resolver-level limiter, and system / internal
// service-to-service and anonymous callers are never counted here.
func TestAuthRateLimit_SkipsGuestSystemAnonymous(t *testing.T) {
	denyAll := &fakeLimiter{allow: false} // would 429 anyone actually counted
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithRateLimiter(denyAll.fn),
	), nil)

	guest := &consts.Actor{ID: uuid.New(), IsGuest: true}
	system := &consts.Actor{ID: uuid.New(), IsSystem: true}

	assert.Equal(t, http.StatusOK, serve(eng, guest, "1.1.1.1"), "guest keeps its separate limiter")
	assert.Equal(t, http.StatusOK, serve(eng, system, "1.1.1.1"), "internal service-to-service is exempt")
	assert.Equal(t, http.StatusOK, serve(eng, nil, "1.1.1.1"), "anonymous request is not counted")

	assert.Empty(t, denyAll.calls, "skipped actors must never touch the limiter")
}

// Edge case: if the limiter errors (Redis down) the request is served anyway —
// an infra outage must not lock the product out.
func TestAuthRateLimit_FailsOpenOnLimiterError(t *testing.T) {
	// Mirrors ratelimit.Allow, which returns true alongside the error.
	failing := &fakeLimiter{allow: true, err: errors.New("redis unavailable")}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithRateLimiter(failing.fn),
	), nil)

	assert.Equal(t, http.StatusOK, serve(eng, userActor(), "1.1.1.1"))
}

// AC4: every throttle is logged with the actor id, IP and operation name, so
// "who got throttled and when" is answerable from logs alone.
func TestAuthRateLimit_LogsActorOnThrottle(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(1, time.Minute),
		middleware.WithRateLimiter(lim.fn),
	), nil)
	actor := userActor()

	require.Equal(t, http.StatusOK, serve(eng, actor, "9.9.9.9"))
	require.Equal(t, http.StatusTooManyRequests, serve(eng, actor, "9.9.9.9"))

	out := buf.String()
	assert.Contains(t, out, "auth rate limit exceeded")
	assert.Contains(t, out, actor.ID.String(), "actor id must be in the throttle log")
	assert.Contains(t, out, "9.9.9.9", "IP must be in the throttle log")
	assert.Contains(t, out, "Loads", "operation name must be in the throttle log")
}

// The ceiling and window are environment-configurable, with the documented
// defaults applied when the env is unset or invalid.
func TestAuthRateLimit_ReadsEnvConfig(t *testing.T) {
	t.Setenv(middleware.EnvAuthRateLimitMax, "5")
	t.Setenv(middleware.EnvAuthRateLimitWindow, "30s")

	capLim := &fakeLimiter{allow: true}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithRateLimiter(capLim.fn),
	), nil)

	require.Equal(t, http.StatusOK, serve(eng, userActor(), "1.1.1.1"))
	require.Len(t, capLim.calls, 1)
	assert.Equal(t, 5, capLim.calls[0].limit)
	assert.Equal(t, 30*time.Second, capLim.calls[0].window)
}

// The Redis key is auth:<user>:<ip>, falling back to auth:<user> when the edge
// could not resolve an address — never a shared blank bucket.
func TestAuthRateLimit_KeyShape(t *testing.T) {
	capLim := &fakeLimiter{allow: true}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithRateLimiter(capLim.fn),
	), nil)
	actor := userActor()

	require.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"))
	require.Equal(t, http.StatusOK, serve(eng, actor, "")) // origin present but IP unresolved

	require.Len(t, capLim.calls, 2)
	assert.Equal(t, "auth:"+actor.ID.String()+":203.0.113.9", capLim.calls[0].key)
	assert.Equal(t, "auth:"+actor.ID.String(), capLim.calls[1].key)
}
