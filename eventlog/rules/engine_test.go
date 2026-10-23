package rules

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUndefinedTable(t *testing.T) {
	missing := fmt.Errorf("find rules: %w", &pgconn.PgError{Code: "42P01"})
	if !isUndefinedTable(missing) {
		t.Fatal("wrapped 42P01 must count as a missing table")
	}
	if isUndefinedTable(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("other database errors are not a missing table")
	}
	if isUndefinedTable(errors.New("boom")) || isUndefinedTable(nil) {
		t.Fatal("non-postgres errors are not a missing table")
	}
}

func TestNoTableSkipsLookup(t *testing.T) {
	e := &Engine{} // nil db: a lookup would panic
	e.noTable.Store(true)
	got, err := e.GetMatchingRules(t.Context(), "trips", "created", nil)
	if err != nil || got != nil {
		t.Fatalf("want no rules and no error once the table is known missing, got %v %v", got, err)
	}
}
