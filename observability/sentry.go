// Package observability wires error reporting to Sentry. Init is a no-op when
// SENTRY_DSN is empty, so services compile and run identically with or without
// a configured DSN — useful for local dev and for staged rollout.
package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/response"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/getsentry/sentry-go"
)

// Config carries Sentry init parameters. Pull from env via your service's
// config layer; an empty DSN disables reporting cleanly.
type Config struct {
	DSN     string
	Env     string
	Release string
	Service string
	// Sample rate for non-error events (traces). 0 disables.
	TracesSampleRate float64
}

var enabled bool

// LoadConfigFromEnv reads SENTRY_DSN / SENTRY_ENVIRONMENT / SENTRY_RELEASE
// from the process environment. SENTRY_ENV is accepted as a legacy fallback
// when SENTRY_ENVIRONMENT is unset.
func LoadConfigFromEnv(service string) Config {
	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = os.Getenv("SENTRY_ENV")
	}
	return Config{
		DSN:     os.Getenv("SENTRY_DSN"),
		Env:     env,
		Release: os.Getenv("SENTRY_RELEASE"),
		Service: service,
	}
}

// Init configures the global Sentry hub. Safe to call once at service boot.
// Returns false when DSN is empty so callers can branch on whether to install
// Sentry middleware. Missing DSN is logged at WARN (ERROR when Env looks
// production-like) so the silent-disable failure mode is loud.
func Init(cfg Config) bool {
	if cfg.DSN == "" {
		msg := "Sentry disabled: SENTRY_DSN is empty — errors will NOT be reported"
		if isProdEnv(cfg.Env) {
			slog.Error(msg, "service", cfg.Service, "env", cfg.Env)
		} else {
			slog.Warn(msg, "service", cfg.Service, "env", cfg.Env)
		}
		return false
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Env,
		Release:          cfg.Release,
		ServerName:       cfg.Service,
		TracesSampleRate: cfg.TracesSampleRate,
		// AttachStacktrace ensures plain `errors.New` captures get a stack.
		AttachStacktrace: true,
	}); err != nil {
		slog.Error("Sentry init failed", "err", err, "service", cfg.Service)
		return false
	}
	enabled = true
	slog.Info("Sentry enabled", "service", cfg.Service, "env", cfg.Env, "release", cfg.Release)
	return true
}

// Flush waits up to timeout for buffered events to be sent. Call from main on
// shutdown so in-flight events aren't dropped.
func Flush(timeout time.Duration) {
	if !enabled {
		return
	}
	sentry.Flush(timeout)
}

// CaptureWithCtx sends err to Sentry at Error level, enriched with the
// request_id and actor tags (see captureWithLevel). Use for server faults
// (5xx) and genuinely unexpected errors — these fire alerts. Safe to call when
// Sentry is disabled — it becomes a no-op.
func CaptureWithCtx(ctx context.Context, err error) {
	captureWithLevel(ctx, err, sentry.LevelError)
}

// CaptureWarningWithCtx sends err to Sentry at Warning level, enriched the same
// way as CaptureWithCtx. Use for expected user-facing rejections (4xx) — wrong
// input, permission walls, etc. Warnings appear in the Sentry issues list so
// the team can query user friction on demand, but they don't fire alerts or
// page anyone. Safe to call when Sentry is disabled — it becomes a no-op.
func CaptureWarningWithCtx(ctx context.Context, err error) {
	captureWithLevel(ctx, err, sentry.LevelWarning)
}

// captureWithLevel is the shared capture path: it tags the event with the
// request_id and actor identity, sets the given severity level, and sends it.
// Keeping both public helpers on one path guarantees warnings and errors carry
// identical enrichment.
func captureWithLevel(ctx context.Context, err error, level sentry.Level) {
	if !enabled || err == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(level)
		if rid := middleware.GetRequestID(ctx); rid != "" {
			scope.SetTag("request_id", rid)
		}
		enrichWithActor(ctx, scope)
		hub.CaptureException(err)
	})
}

// enrichWithActor tags the scope with who triggered the event so user-friction
// warnings (and errors) can be filtered by user/company in Sentry. No-op when
// there is no actor on the context (e.g. background jobs).
func enrichWithActor(ctx context.Context, scope *sentry.Scope) {
	actor, err := middleware.GetActor(ctx)
	if err != nil || actor == nil {
		return
	}
	switch {
	case actor.IsSystem:
		scope.SetTag("actor_type", "system")
	case actor.IsGuest:
		scope.SetTag("actor_type", "guest")
	default:
		scope.SetTag("actor_type", "user")
	}
	if actor.ID != uuid.Nil {
		scope.SetUser(sentry.User{ID: actor.ID.String()})
	}
	if cid := actor.GetCompanyID(); cid != nil {
		scope.SetTag("company_id", cid.String())
	}
}

// GinMiddleware installs Sentry's request-scoped hub + panic recovery on the
// router. No-op (returns a passthrough) when Sentry is disabled.
func GinMiddleware() gin.HandlerFunc {
	if !enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return sentrygin.New(sentrygin.Options{Repanic: true})
}

// CaptureRestError reports err to Sentry when the effective HTTP status is
// 5xx, mirroring the GraphQL ErrorPresenter behavior. The effective status is
// PublicError.ErrorStatus() when err implements it; otherwise the passed code.
// 4xx are user errors — noisy and rarely actionable for ops, so they're
// dropped. Safe to call when Sentry is disabled.
func CaptureRestError(c *gin.Context, code int, err error) {
	if err == nil {
		return
	}
	status := code
	var pe response.PublicError
	if errors.As(err, &pe) {
		status = pe.ErrorStatus()
	}
	if status < http.StatusInternalServerError {
		return
	}
	CaptureWithCtx(c.Request.Context(), err)
}

// RecoverGoroutine is the non-gin equivalent of sentrygin's panic recovery —
// use as `defer observability.RecoverGoroutine(ctx)` at the top of a spawned
// goroutine. Captures the panic to Sentry (when enabled), logs it with a
// stack, and DOES NOT repanic so the parent process keeps running. Workers
// want to survive a bad message, not die on it.
func RecoverGoroutine(ctx context.Context) {
	r := recover()
	if r == nil {
		return
	}
	stack := debug.Stack()
	slog.Error("goroutine recovered from panic", "panic", r, "stack", string(stack))
	if !enabled {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("source", "background")
		if rid := middleware.GetRequestID(ctx); rid != "" {
			scope.SetTag("request_id", rid)
		}
		// Prefer an error value so Sentry indexes a useful message rather
		// than the interface{} stringification.
		if err, ok := r.(error); ok {
			hub.CaptureException(err)
		} else {
			hub.CaptureException(fmt.Errorf("goroutine panic: %v", r))
		}
	})
	sentry.Flush(2 * time.Second)
}

func isProdEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	}
	return false
}
