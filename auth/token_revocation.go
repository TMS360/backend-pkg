package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// Access-token revocation without a jti or a per-token record.
//
// An access token is verified by signature + expiry only (parseAuthToken), so
// ending a refresh session never touched a token already in a browser: it kept
// working until it expired. This adds a per-USER cutoff in Redis — the one thing
// that actually matters in an incident: "cut this user off now".
//
// Writers (session revoke, employee/driver termination) stamp
// tokens_valid_after:<userID> = now; the read path (IdentifyUserPerms) rejects
// any token whose iat predates it. It reuses the exact pattern perm_resolver
// already runs — a global Redis key by user id, written and read through the
// *Global cache variants so writer and reader address the same key regardless of
// who is acting.

// TokensValidAfterTTL is how long the cutoff marker lives. It only needs to
// outlive the longest-lived access token that could still be presented: once
// every token issued before the cutoff has itself expired, the marker is inert.
// Access tokens are meant to be 15m, but 7-day tokens were minted in production,
// so the marker is kept comfortably longer than that worst case.
const TokensValidAfterTTL = 8 * 24 * time.Hour

// RevocationClockSkew is slack applied in the token's favour when comparing iat
// against the cutoff. iat is stamped by whichever tms-auth replica minted the
// token and the cutoff by whichever replica handled the revoke; their clocks can
// differ by a few seconds. Without slack, a token legitimately refreshed a
// moment AFTER a revoke could be stamped a moment BEFORE the cutoff and be
// rejected by mistake. We only reject tokens clearly older than the cutoff.
const RevocationClockSkew = 5 * time.Second

// tokensValidAfterKey is deliberately company-free and identical on both sides,
// exactly like the perm cache key — the user id is globally unique, and the
// writer (an admin or a system worker) and the reader (the user's own request)
// must address the same key.
func tokensValidAfterKey(userID uuid.UUID) string {
	return fmt.Sprintf("tokens_valid_after:%s", userID.String())
}

// RevokeUserTokens sets the cutoff so every access token this user already holds
// is refused on its next request. Call it from every path that ends a session —
// session revoke, revoke-others, terminate — right after the refresh session(s)
// are deleted. Deleting the session stops NEW tokens; this stops the OLD one.
//
// It is a best-effort security marker, not a transactional write: callers log a
// failure and carry on rather than failing the revoke. The sessions are already
// gone, so the user cannot refresh, and the orphaned access token dies on its
// own within the (now short) access TTL.
func RevokeUserTokens(ctx context.Context, userID uuid.UUID) error {
	return cache.SetGlobal(ctx, tokensValidAfterKey(userID), time.Now().Unix(), TokensValidAfterTTL)
}

// TokenRevoked reports whether a token issued at issuedAt has been revoked for
// userID. It is the read side, one Redis GET on the path that already reads
// perms.
//
// Fail-open on Redis trouble is deliberate: a key-miss (never revoked) and any
// Redis error both return "not revoked". Failing closed would turn a Redis
// outage into a fleet-wide lockout of every user — a far worse outcome than the
// narrow, already-bounded window in which a just-revoked token keeps working
// until Redis recovers. The bool answers "revoked?"; the error is returned only
// so the caller can log the degradation, never to deny the request.
func TokenRevoked(ctx context.Context, userID uuid.UUID, issuedAt time.Time) (bool, error) {
	var cutoffUnix int64
	if err := cache.GetGlobal(ctx, tokensValidAfterKey(userID), &cutoffUnix); err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil // never revoked — the normal case
		}
		return false, err // fail open: caller logs and allows
	}
	return IssuedBeforeCutoff(issuedAt, time.Unix(cutoffUnix, 0)), nil
}

// IssuedBeforeCutoff is the pure comparison behind TokenRevoked, split out so the
// skew handling is testable without Redis. A token is treated as revoked only
// when its iat is clearly (by more than RevocationClockSkew) before the cutoff.
func IssuedBeforeCutoff(issuedAt, cutoff time.Time) bool {
	return issuedAt.Before(cutoff.Add(-RevocationClockSkew))
}
