package tmsgraphql

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/TMS360/backend-pkg/client/postgresql"
	"github.com/TMS360/backend-pkg/response"
	"github.com/jackc/pgx/v5/pgconn"
)

// captureSpy counts how many errors hit each Sentry severity path.
type captureSpy struct {
	errors   int
	warnings int
}

// withCaptureSpy swaps both Sentry seams for counters and restores them after
// the test.
func withCaptureSpy(t *testing.T) *captureSpy {
	t.Helper()
	prevErr, prevWarn := captureFunc, captureWarningFunc
	spy := &captureSpy{}
	captureFunc = func(context.Context, error) { spy.errors++ }
	captureWarningFunc = func(context.Context, error) { spy.warnings++ }
	t.Cleanup(func() { captureFunc, captureWarningFunc = prevErr, prevWarn })
	return spy
}

// A bare Postgres constraint violation must degrade to a clean 4xx public
// message and be surfaced as a Sentry WARNING, never an error.
func TestErrorPresenter_BackstopsRawPgError(t *testing.T) {
	spy := withCaptureSpy(t)

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
	if spy.errors != 0 {
		t.Errorf("error capture called %d times for a 4xx, want 0", spy.errors)
	}
	if spy.warnings != 1 {
		t.Errorf("warning capture called %d times for a 4xx, want 1", spy.warnings)
	}
}

// A 4xx PublicError (e.g. a permission wall) must be a Sentry warning, not an error.
func TestErrorPresenter_PublicError4xxWarns(t *testing.T) {
	spy := withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), response.NewForbidden("no perm", "You cannot do that."))

	if got := gqlErr.Extensions["status"]; got != http.StatusForbidden {
		t.Errorf("Extensions[status] = %v, want %d", got, http.StatusForbidden)
	}
	if spy.warnings != 1 || spy.errors != 0 {
		t.Errorf("captures = (err:%d, warn:%d), want (0, 1)", spy.errors, spy.warnings)
	}
}

// A 5xx PublicError must be captured as a Sentry error (it alerts), not a warning.
func TestErrorPresenter_PublicError5xxCaptures(t *testing.T) {
	spy := withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), response.NewInternalError("boom"))

	if got := gqlErr.Extensions["status"]; got != http.StatusInternalServerError {
		t.Errorf("Extensions[status] = %v, want %d", got, http.StatusInternalServerError)
	}
	if spy.errors != 1 || spy.warnings != 0 {
		t.Errorf("captures = (err:%d, warn:%d), want (1, 0)", spy.errors, spy.warnings)
	}
}

// A PublicError with a structured payload must have its extensions merged into
// gqlErr.Extensions on top of code/status — that is the contract the FE reads
// to render, e.g. a link to the blocking resource without parsing the message.
func TestErrorPresenter_MergesPublicErrorExtensions(t *testing.T) {
	withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), response.NewConflictWithExtensions(
		"tech",
		"user",
		map[string]any{
			"blockingTripId":     "11111111-1111-1111-1111-111111111111",
			"blockingTripNumber": 42,
		},
	))

	if got := gqlErr.Extensions["status"]; got != http.StatusConflict {
		t.Errorf("Extensions[status] = %v, want %d", got, http.StatusConflict)
	}
	if got := gqlErr.Extensions["blockingTripId"]; got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("Extensions[blockingTripId] = %v, want the uuid string", got)
	}
	if got := gqlErr.Extensions["blockingTripNumber"]; got != 42 {
		t.Errorf("Extensions[blockingTripNumber] = %v, want 42", got)
	}
}

// A payload key of "code" or "status" must not shadow the presenter's own
// values — those two keys are the shared contract every client reads first.
func TestErrorPresenter_ExtensionsCannotOverwriteCodeOrStatus(t *testing.T) {
	withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), response.NewConflictWithExtensions(
		"tech",
		"user",
		map[string]any{
			"code":   "HIJACKED",
			"status": 200,
			"detail": "ok",
		},
	))

	if got := gqlErr.Extensions["status"]; got != http.StatusConflict {
		t.Errorf("Extensions[status] = %v, want %d — payload must not overwrite", got, http.StatusConflict)
	}
	if got := gqlErr.Extensions["code"]; got == "HIJACKED" {
		t.Errorf("Extensions[code] = %v, payload must not overwrite the reserved code key", got)
	}
	if got := gqlErr.Extensions["detail"]; got != "ok" {
		t.Errorf("Extensions[detail] = %v, want ok — non-reserved keys still pass through", got)
	}
}

// A PublicError constructed without a payload must not add any extra keys to
// extensions — existing callers (NewConflict / NewBadRequest / ...) stay
// wire-compatible.
func TestErrorPresenter_PublicErrorWithoutExtensionsIsUnchanged(t *testing.T) {
	withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), response.NewConflict("tech", "user"))

	// Expect only the three keys the presenter always writes: code, status, requestId.
	if got, want := len(gqlErr.Extensions), 3; got != want {
		t.Errorf("Extensions has %d keys (%v), want %d", got, gqlErr.Extensions, want)
	}
}

// A genuinely unexpected (non-Public) error must stay a 500 and be captured as
// an error, never a warning.
func TestErrorPresenter_UnknownErrorCaptures(t *testing.T) {
	spy := withCaptureSpy(t)

	present := NewErrorPresenter(false)
	gqlErr := present(context.Background(), errors.New("boom"))

	if gqlErr.Message != "Internal Server Error" {
		t.Errorf("Message = %q, want %q", gqlErr.Message, "Internal Server Error")
	}
	if got := gqlErr.Extensions["code"]; got != "INTERNAL_SERVER_ERROR" {
		t.Errorf("Extensions[code] = %v, want INTERNAL_SERVER_ERROR", got)
	}
	if spy.errors != 1 || spy.warnings != 0 {
		t.Errorf("captures = (err:%d, warn:%d), want (1, 0)", spy.errors, spy.warnings)
	}
}
