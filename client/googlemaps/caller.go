package googlemaps

import (
	"context"
	"log/slog"
	"time"
)

// Google Maps Platform bills per request, so every outbound call is money.
// logCall emits one line per call — "google_call" — carrying the operation and
// the outcome, so a sample can be ranked with `grep | sort | uniq -c` to see what
// the fallback actually costs once HERE starts failing.
//
// This is the leaner sibling of here.logCall: HERE is the primary provider and
// carries a caller-chain stack walk (DEV-653) to attribute its quota burn across
// many call paths. Google is fallback-only and reached from a handful of sites,
// so the op alone identifies the path and the stack walk would be noise.

// logCall records one outbound Google transaction. It never receives the request
// URL: the Geocoding and Places endpoints take the credential as a `key` query
// parameter, so the URL is a secret and must not reach the logs. op is passed in
// by the caller instead.
func logCall(ctx context.Context, op string, status int, started time.Time, err error) {
	slog.LogAttrs(ctx, slog.LevelInfo, "google_call",
		slog.String("op", op),
		slog.String("outcome", outcomeOf(ctx, status, err)),
		slog.Int("status", status),
		slog.Int64("dur_ms", time.Since(started).Milliseconds()),
	)
}

// outcomeOf splits calls into paid-and-useful vs paid-and-wasted, and separates
// retryable failures from permanent ones, so a fallback that is quietly failing
// every time is distinguishable from one that is quietly succeeding.
//
// The auth check comes before the status check on purpose: Google reports a
// rejected key on the Geocoding and Places endpoints as HTTP 200 with
// REQUEST_DENIED in the body, so status alone would file it under "ok".
func outcomeOf(ctx context.Context, status int, err error) string {
	switch {
	case err == nil:
		return "ok"
	case ctx != nil && ctx.Err() != nil:
		return "ctx_canceled"
	case IsAuthError(err):
		return "auth"
	case status >= 500:
		return "http_5xx"
	case status >= 400:
		return "http_4xx"
	case status > 0:
		return "http_other"
	default:
		return "transport"
	}
}
