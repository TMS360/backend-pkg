package here

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"time"
)

// HERE bills per transaction, so every outbound call is money. logCall emits one
// line per call — "here_call" — carrying the operation and the call chain that
// asked for it, so a sample can be ranked with `grep | sort | uniq -c` to find
// which code path is burning the quota (DEV-653).
//
// The call rate is ~0.02 rps, so stack walking and an extra log line cost
// nothing next to the 200-500ms HERE round trip they describe.

// callerKey carries an explicit caller tag for call sites whose stack does not
// name them — a goroutine spawned from a detached context roots the stack at the
// closure, not at the code that scheduled the work.
type callerKey struct{}

// WithCaller tags ctx so here_call attributes its calls to tag instead of
// walking the stack. Use it when the stack would lie: work handed to a
// background goroutine, a debouncer, or anything else that outlives its caller.
func WithCaller(ctx context.Context, tag string) context.Context {
	return context.WithValue(ctx, callerKey{}, tag)
}

func callerFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tag, _ := ctx.Value(callerKey{}).(string)
	return tag
}

// callerChainDepth is how many frames outside this package are joined into the
// caller attribute. One frame is not enough: several distinct paths funnel
// through a single shared wrapper (backend-load's mileage.CalcRouteMilesWithRetry
// fronts both the create-time estimate and the manual recalc), and reporting only
// that wrapper would collapse them into one indistinguishable bucket.
const callerChainDepth = 5

// pkgPrefix is this package's import path; frames beneath it are the client's own
// plumbing (doRequest → GetRoute → CalculateTruckRoute …) and say nothing about
// who wanted the call.
const pkgPrefix = "github.com/TMS360/backend-pkg/client/here."

// callerChain returns up to callerChainDepth frames above this package, nearest
// first, joined by "<-":
//
//	mileage.CalcRouteMilesWithRetry<-trip.RecalcTripMiles<-resolvers.UpdateTrip
//
// The result holds no spaces, so slog's TextHandler leaves it unquoted and
// `grep -o 'caller=[^ ]*'` picks it up whole.
func callerChain() string {
	pcs := make([]uintptr, callerChainDepth+8)
	// Skip runtime.Callers, callerChain, and resolveCaller.
	n := runtime.Callers(3, pcs)
	if n == 0 {
		return "unknown"
	}

	frames := runtime.CallersFrames(pcs[:n])
	names := make([]string, 0, cap(pcs))
	for {
		frame, more := frames.Next()
		if frame.Function == "" {
			break
		}
		names = append(names, frame.Function)
		if !more {
			break
		}
	}

	return chainFrom(names)
}

// chainFrom is the frame-selection rule, split out from the stack walk so it can
// be tested against synthetic frames: a test living in this package would have
// all of its own frames filtered out as internal.
func chainFrom(fnNames []string) string {
	names := make([]string, 0, callerChainDepth)
	for _, fn := range fnNames {
		if len(names) == callerChainDepth {
			break
		}
		if strings.HasPrefix(fn, pkgPrefix) {
			continue
		}
		names = append(names, shortFuncName(fn))
	}

	if len(names) == 0 {
		return "unknown"
	}
	return strings.Join(names, "<-")
}

// shortFuncName trims a fully-qualified frame down to package.Function:
//
//	tms-load/internal/service/trip.(*Updater).RecalcTripMiles → trip.RecalcTripMiles
func shortFuncName(fn string) string {
	if i := strings.LastIndex(fn, "/"); i >= 0 {
		fn = fn[i+1:]
	}
	parts := strings.Split(fn, ".")
	if len(parts) < 2 {
		return fn
	}
	pkg := parts[0]
	name := parts[len(parts)-1]
	// Drop the receiver in the middle: trip.(*Updater).RecalcTripMiles.
	return pkg + "." + strings.TrimSuffix(name, "-fm")
}

// resolveCaller prefers an explicit tag and falls back to the stack, so an
// untagged call site is still attributed rather than silently anonymous.
func resolveCaller(ctx context.Context) string {
	if tag := callerFromCtx(ctx); tag != "" {
		return tag
	}
	return callerChain()
}

// logCall records one outbound HERE transaction. It never receives the request
// URL: HERE takes its credential as an `apiKey` query parameter, so the URL is a
// secret and must not reach the logs. op is passed in by the caller instead.
func logCall(ctx context.Context, op string, status int, started time.Time, err error) {
	slog.LogAttrs(ctx, slog.LevelInfo, "here_call",
		slog.String("op", op),
		slog.String("caller", resolveCaller(ctx)),
		slog.String("outcome", outcomeOf(ctx, status, err)),
		slog.Int("status", status),
		slog.Int64("dur_ms", time.Since(started).Milliseconds()),
	)
}

// outcomeOf splits calls into paid-and-useful vs paid-and-wasted, and separates
// retryable failures from permanent ones. Ranking callers without it is
// misleading: a retried path (backend-load retries route calls three times)
// inflates on a HERE outage, so the loudest caller in a sample may be the
// unluckiest rather than the most frequent.
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
