package tmsgraphql

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/TMS360/backend-pkg/client/postgresql"
	"github.com/jackc/pgx/v5/pgconn"
)

// withCaptureSpy swaps the Sentry capture seam for a counter and restores it
// after the test.
func withCaptureSpy(t *testing.T) *int {
	t.Helper()
	prev := captureFunc
	var calls int
	captureFunc = func(context.Context, error) { calls++ }
	t.Cleanup(func() { captureFunc = prev })
	return &calls
}

// A bare Postgres constraint violation must degrade to a clean 4xx public
// message and must NOT be captured to Sentry.
func TestErrorPresenter_BackstopsRawPgError(t *testing.T) {
	calls := withCaptureSpy(t)

	present := NewErrorPresenter(false)
	err := &pgconn.PgError{
		Code:           postgresql.PgForeignKeyViolationCode,
		ConstraintName: "orders_customer_id_fkey",
	}
	gqlErr := present(context.Background(), err)

	if gqlErr.Message == "Internal Server Error" || gqlErr.Message == "" {
		t.Fatalf("Message = %q, want a clean user-facing message", gqlErr.Message)
	}
	if got := gqlErr.Extensions["status"]; got != http.StatusBadRequest {
		t.Errorf("Extensions[status] = %v, want %d", got, http.StatusBadRequest)
	}
	if *calls != 0 {
		t.Errorf("capture called %d times for a mapped user error, want 0", *calls)
	}
}

// A genuinely unexpected error must stay a 500 and MUST be captured.
func TestErrorPresenter_UnknownErrorCaptures(t *testing.T) {
	calls := withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), errors.New("boom"))

	if gqlErr.Message != "Internal Server Error" {
		t.Errorf("Message = %q, want %q", gqlErr.Message, "Internal Server Error")
	}
	if got := gqlErr.Extensions["code"]; got != "INTERNAL_SERVER_ERROR" {
		t.Errorf("Extensions[code] = %v, want INTERNAL_SERVER_ERROR", got)
	}
	if *calls != 1 {
		t.Errorf("capture called %d times for an unknown error, want 1", *calls)
	}
}
