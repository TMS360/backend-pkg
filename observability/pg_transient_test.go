package observability

import (
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


