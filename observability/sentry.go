// Package observability wires error reporting to Sentry. Init is a no-op when
// SENTRY_DSN is empty, so services compile and run identically with or without
// a configured DSN — useful for local dev and for staged rollout.
package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"

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

// LoadConfigFromEnv reads SENTRY_DSN / SENTRY_ENV / SENTRY_RELEASE directly
// from the process environment. Convenience for services that don't want to
// thread Sentry through their typed config struct — the values are operational
// rather than application config.
func LoadConfigFromEnv(service string) Config {
	return Config{
		DSN:     os.Getenv("SENTRY_DSN"),
		Env:     os.Getenv("SENTRY_ENV"),
		Release: os.Getenv("SENTRY_RELEASE"),
		Service: service,
	}
}

// Init configures the global Sentry hub. Safe to call once at service boot.
// Returns false (and logs at INFO) when DSN is empty so callers can branch on
// whether to install Sentry middleware.
func Init(cfg Config) bool {
	if cfg.DSN == "" {
		slog.Info("Sentry disabled: empty DSN", "service", cfg.Service)
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

// CaptureWithCtx tags an error with the request_id (and any future
// per-request fields) and sends it to Sentry. Safe to call when Sentry is
// disabled — it becomes a no-op.
func CaptureWithCtx(ctx context.Context, err error) {
	if !enabled || err == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		if rid := middleware.GetRequestID(ctx); rid != "" {
			scope.SetTag("request_id", rid)
		}
		hub.CaptureException(err)
	})
}

// GinMiddleware installs Sentry's request-scoped hub + panic recovery on the
// router. No-op (returns a passthrough) when Sentry is disabled.
func GinMiddleware() gin.HandlerFunc {
	if !enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return sentrygin.New(sentrygin.Options{Repanic: true})
}
