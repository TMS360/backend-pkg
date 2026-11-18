package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

// mockHubCtx wires a MockTransport-backed hub onto a context and flips the
// package `enabled` flag on for the duration of the test.
func mockHubCtx(t *testing.T, ctx context.Context) (context.Context, *sentry.MockTransport) {
	t.Helper()
	mt := &sentry.MockTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Transport:  mt,
		SampleRate: 1.0, // never sample out
	})
	if err != nil {
		t.Fatalf("sentry.NewClient: %v", err)
	}
	hub := sentry.NewHub(client, sentry.NewScope())

	prev := enabled
	enabled = true
	t.Cleanup(func() { enabled = prev })

	return sentry.SetHubOnContext(ctx, hub), mt
}

// CaptureWarningWithCtx must send at Warning level and enrich the event with the
// request_id and actor identity so user friction is filterable in Sentry.
func TestCaptureWarningWithCtx_LevelAndActorTags(t *testing.T) {
	companyID := uuid.New()
	actor := &consts.Actor{
		ID:     uuid.New(),
		Claims: &consts.UserClaims{CompanyID: &companyID},
	}
	ctx, mt := mockHubCtx(t, context.Background())
	ctx = middleware.WithRequestID(ctx, "req-42")
	ctx = middleware.WithActor(ctx, actor)

	CaptureWarningWithCtx(ctx, errors.New("paydown too large"))

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.Level != sentry.LevelWarning {
		t.Errorf("Level = %q, want %q", ev.Level, sentry.LevelWarning)
	}
	if ev.Tags["actor_type"] != "user" {
		t.Errorf("actor_type = %q, want %q", ev.Tags["actor_type"], "user")
	}
	if ev.Tags["company_id"] != companyID.String() {
		t.Errorf("company_id = %q, want %q", ev.Tags["company_id"], companyID.String())
	}
	if ev.Tags["request_id"] != "req-42" {
		t.Errorf("request_id = %q, want %q", ev.Tags["request_id"], "req-42")
	}
	if ev.User.ID != actor.ID.String() {
		t.Errorf("User.ID = %q, want %q", ev.User.ID, actor.ID.String())
	}
}

// CaptureWithCtx must keep sending at Error level.
func TestCaptureWithCtx_LevelError(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())

	CaptureWithCtx(ctx, errors.New("boom"))

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Level != sentry.LevelError {
		t.Errorf("Level = %q, want %q", events[0].Level, sentry.LevelError)
	}
}

// A system actor must be tagged as such and carry no user id.
func TestEnrichWithActor_SystemActor(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())
	ctx = middleware.WithSystemActor(ctx)

	CaptureWarningWithCtx(ctx, errors.New("x"))

	events := mt.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Tags["actor_type"] != "system" {
		t.Errorf("actor_type = %q, want %q", events[0].Tags["actor_type"], "system")
	}
}

// When Sentry is disabled both helpers must be safe no-ops.
func TestCapture_DisabledIsNoop(t *testing.T) {
	prev := enabled
	enabled = false
	t.Cleanup(func() { enabled = prev })

	// Must not panic despite a nil-actor, no-hub context.
	CaptureWarningWithCtx(context.Background(), errors.New("x"))
	CaptureWithCtx(context.Background(), errors.New("y"))
}
