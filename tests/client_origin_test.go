package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1411 — capture the caller's IP + user-agent once at the HTTP edge so every
// audit producer inherits them. No DB needed: these cover the resolution rules
// and the context plumbing that tmsdb.writeEvent reads back.

const chromeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

func requestWithHeaders(t *testing.T, headers map[string]string, remoteAddr string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/query", nil)
	r.RemoteAddr = remoteAddr
	// httptest sets a default User-Agent-less request; clear anything implicit.
	r.Header.Del("User-Agent")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// AC1 (capture side): an admin acting from a browser, behind Railway's single
// TLS-terminating proxy, yields that session's IP and browser string.
func TestResolveClientOrigin_BrowserBehindEdgeProxy(t *testing.T) {
	t.Setenv(middleware.EnvTrustedProxyHops, "1")

	r := requestWithHeaders(t, map[string]string{
		middleware.HeaderXForwardedFor: "203.0.113.9",
		"User-Agent":                   chromeUA,
	}, "10.0.0.7:41234")

	origin := middleware.ResolveClientOrigin(r)
	assert.Equal(t, "203.0.113.9", origin.IP)
	assert.Equal(t, chromeUA, origin.UserAgent)
}

// Edge case: a client that writes its own X-Forwarded-For must not be able to
// choose what lands in the audit trail. With one trusted hop the only entry we
// believe is the rightmost one — the one our own edge appended.
func TestResolveClientIP_IgnoresClientSuppliedForwardedFor(t *testing.T) {
	t.Setenv(middleware.EnvTrustedProxyHops, "1")

	r := requestWithHeaders(t, map[string]string{
		// "1.1.1.1" is attacker-controlled; "203.0.113.9" was appended by the edge.
		middleware.HeaderXForwardedFor: "1.1.1.1, 203.0.113.9",
	}, "10.0.0.7:41234")

	assert.Equal(t, "203.0.113.9", middleware.ResolveClientIP(r))
}

// Two trusted hops (e.g. an extra CDN in front) shifts the trusted index left by
// one — still counted from the right, never from the client-controlled head.
func TestResolveClientIP_HonoursTrustedProxyHops(t *testing.T) {
	t.Setenv(middleware.EnvTrustedProxyHops, "2")

	r := requestWithHeaders(t, map[string]string{
		middleware.HeaderXForwardedFor: "1.1.1.1, 203.0.113.9, 172.16.0.4",
	}, "10.0.0.7:41234")

	assert.Equal(t, "203.0.113.9", middleware.ResolveClientIP(r))
}

// Edge case: both address families must round-trip, with or without a port and
// with or without brackets.
func TestResolveClientIP_AddressFamiliesAndPorts(t *testing.T) {
	cases := []struct {
		name     string
		hops     string
		xff      string
		remote   string
		expected string
	}{
		{"ipv4 bare", "1", "203.0.113.9", "10.0.0.7:1", "203.0.113.9"},
		{"ipv4 with port", "1", "203.0.113.9:5555", "10.0.0.7:1", "203.0.113.9"},
		{"ipv6 bare", "1", "2001:db8::1", "10.0.0.7:1", "2001:db8::1"},
		{"ipv6 bracketed with port", "1", "[2001:db8::1]:443", "10.0.0.7:1", "2001:db8::1"},
		{"ipv6 canonicalized", "1", "2001:0DB8:0000:0000:0000:0000:0000:0001", "10.0.0.7:1", "2001:db8::1"},
		{"ipv6 remote addr, no proxy", "0", "", "[2001:db8::2]:443", "2001:db8::2"},
		{"garbage entry is dropped", "1", "not-an-ip", "10.0.0.7:1", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(middleware.EnvTrustedProxyHops, tc.hops)
			headers := map[string]string{}
			if tc.xff != "" {
				headers[middleware.HeaderXForwardedFor] = tc.xff
			}
			r := requestWithHeaders(t, headers, tc.remote)
			assert.Equal(t, tc.expected, middleware.ResolveClientIP(r))
		})
	}
}

// AC3 (capture side): internal traffic that never crossed the edge proxy has no
// X-Forwarded-For — we record nothing rather than a container's own address.
func TestResolveClientIP_NoForwardedHeaderRecordsNothing(t *testing.T) {
	t.Setenv(middleware.EnvTrustedProxyHops, "1")

	r := requestWithHeaders(t, nil, "10.0.0.7:41234")
	assert.Empty(t, middleware.ResolveClientIP(r))
}

// Apollo Router replaces User-Agent with its own on subgraph requests, so the
// client's value arrives renamed. Prefer it; fall back for direct REST calls.
func TestResolveUserAgent_PrefersClientHeaderFromRouter(t *testing.T) {
	r := requestWithHeaders(t, map[string]string{
		middleware.HeaderClientUserAgent: chromeUA,
		"User-Agent":                     "apollo-router/2.9.0",
	}, "10.0.0.7:1")
	assert.Equal(t, chromeUA, middleware.ResolveUserAgent(r))

	direct := requestWithHeaders(t, map[string]string{"User-Agent": chromeUA}, "10.0.0.7:1")
	assert.Equal(t, chromeUA, middleware.ResolveUserAgent(direct))
}

// Edge case: a very long or missing user-agent is truncated or left empty,
// never rejected.
func TestResolveUserAgent_TruncatesAndTolerates(t *testing.T) {
	long := strings.Repeat("a", middleware.MaxUserAgentLen+250)
	r := requestWithHeaders(t, map[string]string{"User-Agent": long}, "10.0.0.7:1")
	got := middleware.ResolveUserAgent(r)
	assert.Len(t, got, middleware.MaxUserAgentLen)

	missing := requestWithHeaders(t, nil, "10.0.0.7:1")
	assert.Empty(t, middleware.ResolveUserAgent(missing))

	// Multi-byte input must not be left half-cut.
	multibyte := strings.Repeat("Ω", middleware.MaxUserAgentLen)
	truncated := middleware.TruncateUserAgent(multibyte)
	assert.True(t, len(truncated) <= middleware.MaxUserAgentLen)
	assert.True(t, utf8ValidString(truncated))
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// AC1 (plumbing): IdentifyUser is the one middleware every service installs, so
// the origin lands on the request context with no per-subgraph change — even for
// an unauthenticated request.
func TestIdentifyUser_StampsOriginOnContext(t *testing.T) {
	t.Setenv(middleware.EnvTrustedProxyHops, "1")
	gin.SetMode(gin.TestMode)

	var captured *middleware.ClientOrigin
	router := gin.New()
	router.Use(middleware.IdentifyUser(nil))
	router.POST("/query", func(c *gin.Context) {
		captured = middleware.GetClientOrigin(c.Request.Context())
		c.Status(http.StatusOK)
	})

	req := requestWithHeaders(t, map[string]string{
		middleware.HeaderXForwardedFor: "1.1.1.1, 203.0.113.9",
		"User-Agent":                   chromeUA,
	}, "10.0.0.7:41234")
	router.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, captured)
	assert.Equal(t, "203.0.113.9", captured.IP)
	assert.Equal(t, chromeUA, captured.UserAgent)
}

// AC3 (plumbing): a background process has no request context, so there is no
// origin to read and the event carries neither field.
func TestGetClientOrigin_AbsentForBackgroundContext(t *testing.T) {
	assert.Nil(t, middleware.GetClientOrigin(context.Background()))
}

// AC2 (wire contract): the two fields are omitted from the event payload when
// unset, and a payload produced before this change unmarshals to nil pointers —
// "not recorded", distinct from "recorded as blank".
func TestEventPayload_OriginFieldsAreOptional(t *testing.T) {
	raw, err := json.Marshal(events.EventPayload{EntityType: "settings", Action: "updated"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "actor_ip")
	assert.NotContains(t, string(raw), "user_agent")

	var legacy events.EventPayload
	require.NoError(t, json.Unmarshal([]byte(`{"entity_type":"settings","action":"updated"}`), &legacy))
	assert.Nil(t, legacy.ActorIP)
	assert.Nil(t, legacy.UserAgent)

	ip, ua := "2001:db8::1", chromeUA
	roundTrip, err := json.Marshal(events.EventPayload{ActorIP: &ip, UserAgent: &ua})
	require.NoError(t, err)
	var decoded events.EventPayload
	require.NoError(t, json.Unmarshal(roundTrip, &decoded))
	require.NotNil(t, decoded.ActorIP)
	require.NotNil(t, decoded.UserAgent)
	assert.Equal(t, ip, *decoded.ActorIP)
	assert.Equal(t, ua, *decoded.UserAgent)
}
