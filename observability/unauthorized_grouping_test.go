package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/response"
	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// isUnauthorized is the pure predicate behind the grouping: only a 401 (direct
// PublicError or a downstream gRPC Unauthenticated) qualifies; every other 4xx
// keeps its own grouping (DEV-1668).
func TestIsUnauthorized_Matrix(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"public 401", response.NewUnauthorized("Unauthorized!", "Please sign in"), true},
		{"public 401 wrapped", fmt.Errorf("resolver me: %w", response.NewUnauthorized("Unauthorized!", "x")), true},
		{"public 403 forbidden", response.NewForbidden("access denied: missing permission", "No access"), false},
		{"public 404", response.NewNotFound("shipment", "42"), false},
		{"public 400", response.NewBadRequest("bad input", "Bad input"), false},
		{"grpc unauthenticated", status.Error(codes.Unauthenticated, "Unauthorized!"), true},
		{"grpc unauthenticated wrapped", fmt.Errorf("call: %w", status.Error(codes.Unauthenticated, "x")), true},
		{"grpc permission denied", status.Error(codes.PermissionDenied, "nope"), false},
		{"grpc not found", status.Error(codes.NotFound, "nope"), false},
		{"plain error", errors.New("boom"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isUnauthorized(c.err); got != c.want {
				t.Errorf("isUnauthorized(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// AC1: several different 401s (as if from different GraphQL fields) all get the
// SAME fixed fingerprint, so Sentry groups them into ONE issue — no new
// per-field issue. AC3: they stay at Warning level and keep request_id +
// actor_type.
func TestUnauthorized_GroupsUnderOneFingerprint(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())
	ctx = middleware.WithRequestID(ctx, "req-1")
	ctx = middleware.WithSystemActor(ctx) // the flood is actor_type=system

	// Different fields → different messages, same 401 class.
	CaptureWarningWithCtx(ctx, response.NewUnauthorized("Unauthorized! (getShipments)", "x"))
	CaptureWarningWithCtx(ctx, response.NewUnauthorized("Unauthorized! (me)", "x"))

	events := mt.Events()
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for i, ev := range events {
		if len(ev.Fingerprint) != 1 || ev.Fingerprint[0] != "unauthorized" {
			t.Errorf("event %d Fingerprint = %v, want [unauthorized]", i, ev.Fingerprint)
		}
		if ev.Level != sentry.LevelWarning {
			t.Errorf("event %d Level = %q, want warning", i, ev.Level)
		}
		if ev.Tags["request_id"] != "req-1" {
			t.Errorf("event %d request_id = %q, want req-1", i, ev.Tags["request_id"])
		}
		if ev.Tags["actor_type"] != "system" {
			t.Errorf("event %d actor_type = %q, want system", i, ev.Tags["actor_type"])
		}
	}
	// Both events carry the identical grouping key → Sentry folds them into one
	// issue.
	if events[0].Fingerprint[0] != events[1].Fingerprint[0] {
		t.Errorf("fingerprints differ: %v vs %v", events[0].Fingerprint, events[1].Fingerprint)
	}
}

// AC2: a permission denial ("access denied: missing permission", 403) keeps its
// default grouping — no shared unauthorized fingerprint.
func TestForbidden_KeepsDefaultGrouping(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())

	CaptureWarningWithCtx(ctx, response.NewForbidden("access denied: missing permission", "No access"))

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if len(events[0].Fingerprint) != 0 {
		t.Errorf("Fingerprint = %v, want empty (default grouping)", events[0].Fingerprint)
	}
}

// Edge: a downstream gRPC UNAUTHENTICATED (a second capture path) joins the same
// single group.
func TestGrpcUnauthenticated_JoinsUnauthorizedGroup(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())

	// The presenter captures the raw gRPC status error on this path.
	CaptureWarningWithCtx(ctx, fmt.Errorf("subservice: %w", status.Error(codes.Unauthenticated, "Unauthorized!")))

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if len(events[0].Fingerprint) != 1 || events[0].Fingerprint[0] != "unauthorized" {
		t.Errorf("Fingerprint = %v, want [unauthorized]", events[0].Fingerprint)
	}
}

// Edge: only pure 401 is grouped — a 403, a 404, and a 400 must never fall into
// the shared group.
func TestNon401_NotGrouped(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())

	CaptureWarningWithCtx(ctx, response.NewForbidden("f", "f"))
	CaptureWarningWithCtx(ctx, response.NewNotFound("x", "1"))
	CaptureWarningWithCtx(ctx, response.NewBadRequest("b", "b"))
	CaptureWarningWithCtx(ctx, status.Error(codes.PermissionDenied, "p"))

	for i, ev := range mt.Events() {
		if len(ev.Fingerprint) != 0 {
			t.Errorf("event %d Fingerprint = %v, want empty", i, ev.Fingerprint)
		}
	}
}

// A 5xx server fault also is not grouped as unauthorized (sanity: internal
// errors keep alerting per site).
func TestInternalError_NotGrouped(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())

	CaptureWithCtx(ctx, response.NewInternalError("db down"))

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if len(events[0].Fingerprint) != 0 {
		t.Errorf("Fingerprint = %v, want empty", events[0].Fingerprint)
	}
	if events[0].Level != sentry.LevelError {
		t.Errorf("Level = %q, want error", events[0].Level)
	}
}
