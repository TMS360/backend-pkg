package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-3291 — a board view fans out into many router calls made by
// backend-workspaces with the user's JWT. Those behind-the-scenes calls carry
// the shared internal secret and must not count against the user's ceiling;
// every call without the exact secret is counted as before (DEV-1485).

const testInternalToken = "internal-secret-for-tests"

func serveWithInternalHeader(eng *gin.Engine, actor *consts.Actor, ip, headerValue string) int {
	r := authedRequest(actor, ip, "BoardLoads")
	if headerValue != "" {
		r.Header.Set(middleware.HeaderInternalToken, headerValue)
	}
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, r)
	return w.Code
}

// AC1 + AC2: twenty board refreshes in a minute, each fanning out into many
// internal calls, never reach the ceiling — the limiter is not even consulted.
func TestAuthRateLimit_InternalCallsAreNotCounted(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(5, 0),
		middleware.WithRateLimiter(lim.fn),
		middleware.WithInternalToken(testInternalToken),
	), nil)
	actor := userActor()

	for i := 0; i < 20*40; i++ {
		require.Equal(t, http.StatusOK, serveWithInternalHeader(eng, actor, "", testInternalToken), "internal call %d", i+1)
	}
	assert.Empty(t, lim.calls, "internal calls must never touch the limiter")
}

// AC3: a direct call that adds the marker header with a guessed value is still
// counted and refused over the ceiling.
func TestAuthRateLimit_ForgedInternalHeaderIsCounted(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(3, 0),
		middleware.WithRateLimiter(lim.fn),
		middleware.WithInternalToken(testInternalToken),
	), nil)
	actor := userActor()

	for _, forged := range []string{"guess", "true", testInternalToken + "x"} {
		require.Equal(t, http.StatusOK, serveWithInternalHeader(eng, actor, "203.0.113.9", forged))
	}
	assert.Equal(t, http.StatusTooManyRequests, serveWithInternalHeader(eng, actor, "203.0.113.9", "guess"),
		"a wrong secret must not skip the ceiling")
}

// AC3: when the service has no internal secret configured, the header exempts
// nothing — not even an empty value matching an empty secret.
func TestAuthRateLimit_NoInternalTokenConfiguredCountsEverything(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(1, 0),
		middleware.WithRateLimiter(lim.fn),
		middleware.WithInternalToken(""),
	), nil)
	actor := userActor()

	require.Equal(t, http.StatusOK, serveWithInternalHeader(eng, actor, "", "anything"))
	assert.Equal(t, http.StatusTooManyRequests, serveWithInternalHeader(eng, actor, "", "anything"))
}

// AC4: the user's own clicks (no marker) still count and recover in the next
// window, even while internal calls for the same user flow freely.
func TestAuthRateLimit_UserClicksStillCountedBesideInternalCalls(t *testing.T) {
	lim := &fakeLimiter{counts: map[string]int{}}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithAuthRateLimit(2, 0),
		middleware.WithRateLimiter(lim.fn),
		middleware.WithInternalToken(testInternalToken),
	), nil)
	actor := userActor()

	require.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"))
	require.Equal(t, http.StatusOK, serveWithInternalHeader(eng, actor, "", testInternalToken))
	require.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"))
	assert.Equal(t, http.StatusTooManyRequests, serve(eng, actor, "203.0.113.9"), "the 3rd own click is over the ceiling")

	lim.counts = map[string]int{} // next window
	assert.Equal(t, http.StatusOK, serve(eng, actor, "203.0.113.9"), "next window works again")
}

// The default secret comes from GRPC_INTERNAL_TOKEN, the one every service
// already has for gRPC.
func TestAuthRateLimit_InternalTokenReadFromEnv(t *testing.T) {
	t.Setenv(middleware.EnvInternalToken, testInternalToken)
	denyAll := &fakeLimiter{allow: false}
	eng := newRateLimitEngine(middleware.RateLimitAuthenticated(
		middleware.WithRateLimiter(denyAll.fn),
	), nil)

	assert.Equal(t, http.StatusOK, serveWithInternalHeader(eng, userActor(), "", testInternalToken))
	assert.Equal(t, http.StatusTooManyRequests, serveWithInternalHeader(eng, userActor(), "", ""))
}
