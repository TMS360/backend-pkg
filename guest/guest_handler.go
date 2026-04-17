package guest

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/ratelimit"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Per-guest rate limit for @authGuest-protected resolvers. Keyed by share link
// (companyID:resource:resourceID), so one link's bad behavior can't starve
// others. Tuned low for initial rollout; bump via NewHandler options if needed.
const (
	defaultGuestRateLimit  = 10
	defaultGuestRateWindow = time.Minute
)

type Handler struct {
	secret     []byte
	tm         tmsdb.TransactionManager
	rateLimit  int
	rateWindow time.Duration
}

type Option func(*Handler)

// WithRateLimit overrides the per-share-link guest rate limit.
func WithRateLimit(limit int, window time.Duration) Option {
	return func(h *Handler) {
		h.rateLimit = limit
		h.rateWindow = window
	}
}

func NewHandler(secret []byte, tm tmsdb.TransactionManager, opts ...Option) *Handler {
	h := &Handler{
		secret:     secret,
		tm:         tm,
		rateLimit:  defaultGuestRateLimit,
		rateWindow: defaultGuestRateWindow,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Middleware — Gin middleware that resolves guest token and sets actor
func (gh *Handler) Middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if _, err := middleware.GetActor(ctx.Request.Context()); err == nil {
			ctx.Next()
			return
		}

		token := ctx.GetHeader("X-Guest-Token")
		if token == "" {
			ctx.Next()
			return
		}

		claims, err := parseGuestToken(token, gh.secret)
		if err != nil {
			slog.Debug("guest token parse failed", "error", err)
			ctx.Next()
			return
		}

		companyID := claims.CompanyID

		key := fmt.Sprintf("%s:share_link:%s", companyID, claims.ShareLinkID)
		var data ShareLinkRedisData
		if err := cache.Get(ctx.Request.Context(), key, &data); err != nil {
			slog.Debug("share link not found or revoked", "slid", claims.ShareLinkID)
			ctx.Next()
			return
		}

		resourceID, err := uuid.Parse(data.ResourceID)
		if err != nil {
			slog.Debug("invalid resource ID in redis", "err", err)
			ctx.Next()
			return
		}

		actor := &consts.Actor{
			ID:      uuid.Nil,
			IsGuest: true,
			Claims: &consts.UserClaims{
				CompanyID:  &companyID,
				Resource:   data.Resource,
				ResourceID: resourceID,
			},
		}

		guestCtx := middleware.WithActor(ctx.Request.Context(), actor)
		gh.maybeLogAccess(guestCtx, claims.ShareLinkID, ctx.Request)

		ctx.Request = ctx.Request.WithContext(guestCtx)
		ctx.Next()
	}
}

// Directive implements @authGuest(resource: "shipment")
func (gh *Handler) Directive(ctx context.Context, obj interface{}, next graphql.Resolver, resource string) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if !actor.IsGuest {
		return next(ctx)
	}

	if actor.Claims.Resource != resource {
		return nil, fmt.Errorf("unauthorized: guest access not allowed for this resource")
	}

	companyID := ""
	if actor.Claims.CompanyID != nil {
		companyID = actor.Claims.CompanyID.String()
	}
	key := fmt.Sprintf("guest:%s:%s:%s", companyID, actor.Claims.Resource, actor.Claims.ResourceID)
	allowed, rlErr := ratelimit.Allow(ctx, key, gh.rateLimit, gh.rateWindow)
	if rlErr != nil {
		slog.Warn("guest rate limit check failed — failing open", "err", rlErr)
	}
	if !allowed {
		return nil, fmt.Errorf("429 Too Many Requests: guest rate limit exceeded (%d per %s)", gh.rateLimit, gh.rateWindow)
	}

	return next(ctx)
}

// maybeLogAccess debounce access logs via Redis SETNX and publishes via outbox
func (gh *Handler) maybeLogAccess(ctx context.Context, shareLinkID uuid.UUID, r *http.Request) {
	ip := resolveClientIP(r)
	dedupeKey := fmt.Sprintf("access_seen:%s:%s", shareLinkID, ip)

	set, err := cache.SetNX(ctx, dedupeKey, "1", 3*time.Minute)
	if err != nil || !set {
		return
	}

	event := AccessLogEvent{
		ShareLinkID: shareLinkID.String(),
		IPAddress:   ip,
		UserAgent:   r.UserAgent(),
		AccessedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if err := gh.tm.Publish(ctx, "share_links", "access_log", shareLinkID, event); err != nil {
		slog.Error("failed to publish access log event", "err", err)
	}
}

func parseGuestToken(tokenString string, secret []byte) (*ShareLinkClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &ShareLinkClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid guest token: %w", err)
	}

	claims, ok := token.Claims.(*ShareLinkClaims)
	if !ok {
		return nil, fmt.Errorf("failed to cast share link claims")
	}
	return claims, nil
}

func resolveClientIP(r *http.Request) string {
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
