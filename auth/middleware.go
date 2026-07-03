package auth

import (
	"context"
	"log/slog"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/gin-gonic/gin"
)

// IdentifyUserPerms fetches the caller's effective perms once per request
// and stashes them on the request context. Every @hasPerms check downstream
// reads from this context — never from JWT, never from Redis directly — so
// a request with N protected fields still triggers exactly one resolve call.
//
// Must run after IdentifyUser. Guest actors and unauthenticated requests
// are skipped (their resolver call would be pointless).
func IdentifyUserPerms(pr *PermResolver) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		actor, err := middleware.GetActor(ctx.Request.Context())
		if err != nil || actor == nil || actor.IsGuest || actor.IsSystem {
			ctx.Next()
			return
		}

		perms, err := pr.GetUserPerms(ctx.Request.Context(), actor.ID)
		if err != nil {
			slog.Warn("perms resolution failed, continuing with empty perms", "userID", actor.ID, "err", err)
			perms = []string{}
		}

		newCtx := context.WithValue(ctx.Request.Context(), middleware.PermsCtxKey{}, perms)
		ctx.Request = ctx.Request.WithContext(newCtx)
		ctx.Next()
	}
}

// WithUserPerms returns a copy of ctx with the given perm list attached.
// Use in tests and non-HTTP code paths (kafka consumers, cron jobs) that
// need to set up an actor's perms manually.
func WithUserPerms(ctx context.Context, perms []string) context.Context {
	if perms == nil {
		perms = []string{}
	}
	return context.WithValue(ctx, middleware.PermsCtxKey{}, perms)
}
