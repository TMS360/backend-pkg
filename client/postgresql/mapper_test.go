package postgresql

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// AsPublicError must translate raw constraint violations into clean 4xx public
// errors, and never leak the SQLSTATE code or the constraint name to the user.
func TestAsPublicError(t *testing.T) {
	const constraintName = "orders_customer_id_fkey"

	cases := []struct {
		name       string
		code       string
		wantStatus int
	}{
		{"foreign key", PgForeignKeyViolationCode, http.StatusBadRequest},
		{"unique", PgUniqueViolationCode, http.StatusConflict},
		{"exclusion", PgExclusionViolationCode, http.StatusConflict},
		{"check", PgCheckViolationCode, http.StatusBadRequest},
		{"not null", PgNotNullViolationCode, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, ok := AsPublicError(&pgconn.PgError{Code: tc.code, ConstraintName: constraintName})
			if !ok {
				t.Fatalf("AsPublicError(code=%s) ok = false, want true", tc.code)
			}
			if got := pub.ErrorStatus(); got != tc.wantStatus {
				t.Errorf("ErrorStatus() = %d, want %d", got, tc.wantStatus)
			}

			msg := pub.UserMessage()
			if msg == "" {
				t.Error("UserMessage() is empty")
			}
			if strings.Contains(msg, tc.code) {
				t.Errorf("UserMessage() = %q leaks SQLSTATE %q", msg, tc.code)
			}
			if strings.Contains(msg, constraintName) {
				t.Errorf("UserMessage() = %q leaks constraint name %q", msg, constraintName)
			}

			// The constraint name must still be available for ops in the technical message.
			if !strings.Contains(pub.Error(), constraintName) {
				t.Errorf("Error() = %q, want it to contain constraint name %q", pub.Error(), constraintName)
			}
		})
	}
}

func TestAsPublicError_NonPgError(t *testing.T) {
	pub, ok := AsPublicError(errors.New("boom"))
	if ok {
		t.Errorf("AsPublicError(non-pg) ok = true, want false")
	}
	if pub != nil {
		t.Errorf("AsPublicError(non-pg) pub = %v, want nil", pub)
	}
}

// An unrecognized SQLSTATE (e.g. serialization failure) is not a user-input
// error and must fall through to default handling.
func TestAsPublicError_UnrecognizedCode(t *testing.T) {
	pub, ok := AsPublicError(&pgconn.PgError{Code: "40001"})
	if ok || pub != nil {
		t.Errorf("AsPublicError(code=40001) = (%v, %v), want (nil, false)", pub, ok)
	}
}
