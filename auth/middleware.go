package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/gin-gonic/gin"
)

// IdentifyUserPerms fetches the caller's effective perms once per request
// and stashes them on the request context. Every @hasPerms check downstream
// reads from this context — never from JWT, never from Redis directly — so
// a request with N protected fields still triggers exactly one resolve call.
//
// It is also the revocation gate: on the same pass it reads the user's
// tokens_valid_after cutoff (one extra Redis GET) and rejects any access token
// issued before it — the token-level counterpart to deleting a refresh session.
// This is why an already-issued token stops working the moment a session is
// revoked or a user is terminated, instead of living out its whole TTL.
//
// revocationExemptPrefixes are matched against the gin route pattern
// (ctx.FullPath); a request whose route starts with one is not subjected to the
// revocation abort. tms-auth passes its "/api/auth" prefix so a user with a
// revoked access token can still reach /api/auth/refresh — the browser attaches
// the (now-rejected) access token to that call too, and blocking it would break
// the very refresh that lets a still-valid session recover. Subgraphs have no
// such endpoints and pass nothing.
//
// Must run after IdentifyUser. Guest, system (x-internal-token) and
// unauthenticated actors are skipped — internal service-to-service tokens are
// therefore never affected by user revocation.
func IdentifyUserPerms(pr *PermResolver, revocationExemptPrefixes ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		actor, err := consts.GetActor(ctx.Request.Context())
		if err != nil || actor == nil || actor.IsGuest || actor.IsSystem {
			ctx.Next()
			return
		}

		// Revocation gate. Skipped for exempt routes (see above) and for tokens
		// with no iat to compare (guest/share tokens never reach here anyway).
		if actor.Claims != nil && actor.Claims.IssuedAt != nil &&
			!hasAnyPrefix(ctx.FullPath(), revocationExemptPrefixes) {
			revoked, rerr := TokenRevoked(ctx.Request.Context(), actor.ID, actor.Claims.IssuedAt.Time)
			if rerr != nil {
				// Fail OPEN: a Redis outage must not lock out every user. The
				// window (marker unreadable) is narrow and bounded; failing
				// closed would be a fleet-wide outage to defend a rare case.
				slog.Warn("token revocation check failed, allowing request", "userID", actor.ID, "err", rerr)
			} else if revoked {
				slog.Info("rejected revoked access token", "userID", actor.ID)
				ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error":   consts.CodeTokenRevoked,
					"message": consts.MsgTokenRevoked,
				})
				return
			}
		}

		perms, err := pr.GetUserPerms(ctx.Request.Context(), actor.ID)
		if err != nil {
			// Unresolved ≠ denied. Substituting []string{} made every @hasPerm
			// answer "access denied: missing permission" for infra blips
			// (auth hop 403/timeout), which is indistinguishable from a real
			// RBAC denial for the user, FE, and Sentry (DEV-1555).
			slog.Error("perms resolution failed", "userID", actor.ID, "err", err)
			newCtx := context.WithValue(ctx.Request.Context(), consts.PermsUnresolvedCtx, true)
			// Keep an empty list for readers that only check the slice; gates
			// must consult PermsUnresolved first.
			newCtx = context.WithValue(newCtx, consts.PermsCtx, []string{})
			ctx.Request = ctx.Request.WithContext(newCtx)
			ctx.Next()
			return
		}

		newCtx := context.WithValue(ctx.Request.Context(), consts.PermsCtx, perms)
		ctx.Request = ctx.Request.WithContext(newCtx)
		ctx.Next()
	}
}

// hasAnyPrefix reports whether path starts with any of prefixes. Empty prefixes
// (the subgraph case) never match, so the revocation gate stays fully on.
func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// WithUserPerms returns a copy of ctx with the given perm list attached.
// Use in tests and non-HTTP code paths (kafka consumers, cron jobs) that
// need to set up an actor's perms manually.
func WithUserPerms(ctx context.Context, perms []string) context.Context {
	if perms == nil {
		perms = []string{}
	}
	return context.WithValue(ctx, consts.PermsCtx, perms)
}
