package middleware

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Request-origin capture (DEV-1411).
//
// Audit rows answer "who" and "when" but not "from where". These helpers resolve
// the caller's IP address and user-agent once, at the HTTP edge of every service,
// and stash them on the request context. tmsdb.writeEvent reads them back when it
// stamps the outbox event, so every producer inherits the fields without passing
// anything explicitly — exactly how actor id already works.
//
// Both values are best-effort and optional: a background/system-triggered event
// has no HTTP request behind it and therefore records nothing.

const (
	// HeaderXForwardedFor is the proxy chain header. Railway's edge appends the
	// real client address to it before the request reaches us.
	HeaderXForwardedFor = "X-Forwarded-For"

	// HeaderClientUserAgent carries the *client's* user-agent across Apollo
	// Router, which replaces User-Agent with its own on subgraph requests. The
	// router copies the incoming User-Agent into this header (see
	// apollo-router/router.yaml). Direct (non-federated) REST calls have no such
	// header and fall back to plain User-Agent.
	HeaderClientUserAgent = "X-Client-User-Agent"

	// EnvTrustedProxyHops is the number of reverse proxies in front of this
	// service. See TrustedProxyHops.
	EnvTrustedProxyHops = "TRUSTED_PROXY_HOPS"

	// DefaultTrustedProxyHops matches the Railway deployment: exactly one
	// TLS-terminating edge proxy in front of the app (and in front of Apollo
	// Router, which forwards X-Forwarded-For verbatim rather than appending to
	// it — so subgraphs see the same single-hop chain the router sees).
	DefaultTrustedProxyHops = 1

	// MaxUserAgentLen bounds what we persist. Real user-agents are well under
	// this; anything longer is truncated, never rejected.
	MaxUserAgentLen = 512
)

// ClientOrigin is where a request came from. Empty strings mean "not recorded"
// — never a placeholder or the server's own address.
type ClientOrigin struct {
	IP        string
	UserAgent string
}

type clientOriginKey struct{}

// WithClientOrigin binds the resolved origin to ctx. Exported so non-gin entry
// points (websocket upgrades, tests) can stamp it too.
func WithClientOrigin(ctx context.Context, origin *ClientOrigin) context.Context {
	return context.WithValue(ctx, clientOriginKey{}, origin)
}

// GetClientOrigin returns the origin bound to ctx, or nil when the ctx does not
// belong to an inbound HTTP request (Kafka consumers, crons, seeders).
func GetClientOrigin(ctx context.Context) *ClientOrigin {
	origin, _ := ctx.Value(clientOriginKey{}).(*ClientOrigin)
	return origin
}

// TrustedProxyHops reports how many reverse proxies sit in front of this
// service, from the TRUSTED_PROXY_HOPS env var (default DefaultTrustedProxyHops).
// Set it to 0 when the service is exposed directly — then the socket peer is the
// client and no forwarded header is trusted at all.
func TrustedProxyHops() int {
	raw := strings.TrimSpace(os.Getenv(EnvTrustedProxyHops))
	if raw == "" {
		return DefaultTrustedProxyHops
	}
	hops, err := strconv.Atoi(raw)
	if err != nil || hops < 0 {
		return DefaultTrustedProxyHops
	}
	return hops
}

// ResolveClientIP returns the caller's address, or "" when it cannot be
// established from a source we trust.
//
// Why not "first entry of X-Forwarded-For": that entry is whatever the client
// typed. XFF is append-only, so with N trusted proxies in front of us the last N
// entries are the ones our own infrastructure wrote; everything to their left is
// client-controlled. We therefore index from the RIGHT — an audit field a client
// can forge is worse than no audit field.
//
// With hops > 0 an absent XFF means the request never crossed the edge proxy
// (internal service-to-service traffic), so we record nothing rather than an
// internal container address.
func ResolveClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	hops := TrustedProxyHops()
	if hops == 0 {
		return normalizeIP(r.RemoteAddr)
	}

	forwarded := r.Header.Get(HeaderXForwardedFor)
	if forwarded == "" {
		return ""
	}

	parts := strings.Split(forwarded, ",")
	idx := len(parts) - hops
	if idx < 0 {
		// Shorter chain than configured — the topology assumption is off, so the
		// leftmost entry is the only candidate left. Still never RemoteAddr.
		idx = 0
	}
	return normalizeIP(parts[idx])
}

// ResolveUserAgent returns the caller's user-agent, truncated to
// MaxUserAgentLen. Missing or unparseable yields "" — never an error.
func ResolveUserAgent(r *http.Request) string {
	if r == nil {
		return ""
	}

	ua := strings.TrimSpace(r.Header.Get(HeaderClientUserAgent))
	if ua == "" {
		ua = strings.TrimSpace(r.Header.Get("User-Agent"))
	}
	return TruncateUserAgent(ua)
}

// TruncateUserAgent bounds a user-agent to MaxUserAgentLen bytes, dropping any
// rune left half-cut by the slice so the result is always valid UTF-8.
func TruncateUserAgent(ua string) string {
	if len(ua) <= MaxUserAgentLen {
		return ua
	}
	return strings.ToValidUTF8(ua[:MaxUserAgentLen], "")
}

// ResolveClientOrigin builds the ClientOrigin for an inbound request.
func ResolveClientOrigin(r *http.Request) *ClientOrigin {
	return &ClientOrigin{
		IP:        ResolveClientIP(r),
		UserAgent: ResolveUserAgent(r),
	}
}

// normalizeIP canonicalizes one XFF entry / RemoteAddr into a bare IP literal.
// Handles "1.2.3.4", "1.2.3.4:5678", "2001:db8::1" and "[2001:db8::1]:443".
// Anything that is not a valid IP returns "" rather than being stored verbatim.
func normalizeIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if ip := net.ParseIP(raw); ip != nil {
		return ip.String()
	}

	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.String()
	}
	return ""
}
