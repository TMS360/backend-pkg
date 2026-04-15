package guest

import (
	"log/slog"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/gin-gonic/gin"
)

// IdentifyGuest parses the X-Guest-Token header and stashes pending claims on
// the context. The actual Redis lookup is deferred until an @authGuest directive
// triggers it — services with no guest-accessible resolvers never hit Redis.
func IdentifyGuest(secret []byte) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Skip if a real user was already identified by IdentifyUser.
		if _, err := middleware.GetActor(ctx.Request.Context()); err == nil {
			ctx.Next()
			return
		}

		token := ctx.GetHeader("X-Guest-Token")
		if token == "" {
			ctx.Next()
			return
		}

		// Fast gate — signature + expiry check, no network call.
		claims, err := ParseGuestToken(token, secret)
		if err != nil {
			slog.Debug("guest token parse failed", "error", err)
			ctx.Next()
			return
		}

		// Store pending claims; Redis lookup happens lazily in the directive.
		pending := &PendingGuestClaims{
			ShareLinkID: claims.ShareLinkID,
			Request:     ctx.Request,
		}
		ctx.Request = ctx.Request.WithContext(
			WithPendingClaims(ctx.Request.Context(), pending),
		)

		ctx.Next()
	}
}
