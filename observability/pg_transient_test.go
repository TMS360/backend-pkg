package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// DEV-851: a short-lived Postgres connection drop on a long-running loop (outbox
// relay, consumer, scheduler) self-heals on the next tick, so CaptureWithCtx must
// downgrade the known SQLSTATEs to a WARN and NOT page Sentry — while keeping
// 53300 (pool exhaustion, DEV-848) and every other error loud.

func pgErr(code string) *pgconn.PgError { return &pgconn.PgError{Code: code, Message: "boom"} }

func TestTransientPGError_Classification(t *testing.T) {
	transient := []string{"57P01", "57P02", "57P03", "08006", "08001", "08003", "08004"}
	for _, code := range transient {
		got, ok := transientPGError(pgErr(code))
		if !ok || got != code {
			t.Fatalf("SQLSTATE %s: expected transient, got (%q, %v)", code, got, ok)
		}
	}
	// A transient error wrapped in the caller's context is still recognised.
	if _, ok := transientPGError(fmt.Errorf("outbox poll: %w", pgErr("57P01"))); !ok {
		t.Fatal("wrapped 57P01 must be recognised as transient")
	}
	// 53300 (too_many_connections) is the real capacity signal — NOT transient.
	if _, ok := transientPGError(pgErr("53300")); ok {
		t.Fatal("53300 must stay loud, not be treated as transient")
	}
	// A unique-violation is a genuine fault — not transient.
	if _, ok := transientPGError(pgErr("23505")); ok {
		t.Fatal("23505 must not be treated as transient")
	}
	// A plain non-PG error is not transient.
	if _, ok := transientPGError(errors.New("some other error")); ok {
		t.Fatal("non-PG error must not be treated as transient")
	}
}

// A transient Postgres drop produces no Sentry event (WARN log only).
func TestCaptureWithCtx_TransientPG_SkipsSentry(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())
	CaptureWithCtx(ctx, fmt.Errorf("outbox relay poll: %w", pgErr("57P01")))
	if n := len(mt.Events()); n != 0 {
		t.Fatalf("transient 57P01 must not be captured; got %d events", n)
	}
}

// 53300 pool exhaustion still fires to Sentry (DEV-848 stays loud).
func TestCaptureWithCtx_TooManyConnections_StillCaptures(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())
	CaptureWithCtx(ctx, pgErr("53300"))
	if n := len(mt.Events()); n != 1 {
		t.Fatalf("53300 must still be captured; got %d events", n)
	}
}

// A non-transient Postgres fault is still captured.
func TestCaptureWithCtx_NonTransientPG_StillCaptures(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())
	CaptureWithCtx(ctx, pgErr("23505"))
	if n := len(mt.Events()); n != 1 {
		t.Fatalf("23505 must still be captured; got %d events", n)
	}
}
