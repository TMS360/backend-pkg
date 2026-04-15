package guest

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type ctxKey int

const (
	pendingClaimsKey ctxKey = iota
	resolvedGuestKey
)

// PendingGuestClaims is set by the middleware after JWT verification (no Redis yet).
type PendingGuestClaims struct {
	ShareLinkID uuid.UUID
	Request     *http.Request
}

func WithPendingClaims(ctx context.Context, c *PendingGuestClaims) context.Context {
	return context.WithValue(ctx, pendingClaimsKey, c)
}

func GetPendingClaims(ctx context.Context) (*PendingGuestClaims, bool) {
	c, ok := ctx.Value(pendingClaimsKey).(*PendingGuestClaims)
	return c, ok
}

func WithResolvedGuest(ctx context.Context, g *ResolvedGuest) context.Context {
	return context.WithValue(ctx, resolvedGuestKey, g)
}

func GetResolvedGuest(ctx context.Context) (*ResolvedGuest, bool) {
	g, ok := ctx.Value(resolvedGuestKey).(*ResolvedGuest)
	return g, ok
}

// ResolveClientIP extracts the client IP, preferring X-Forwarded-For for proxied traffic.
func ResolveClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For can be comma-separated; take the first (original client).
		if idx := strings.Index(fwd, ","); idx != -1 {
			return strings.TrimSpace(fwd[:idx])
		}
		return strings.TrimSpace(fwd)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
