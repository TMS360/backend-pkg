package postgresql

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DEV-4280: a slow or failing write must reach the log without its values.

const secretCell = "Driver's private note 4417"

// failingDriver answers every statement with a unique violation whose message
// and detail quote the value, as Postgres does.
type failingDriver struct{}

func (failingDriver) Open(string) (driver.Conn, error) { return failingConn{}, nil }

type failingConn struct{}

func (failingConn) Prepare(string) (driver.Stmt, error) { return failingStmt{}, nil }
func (failingConn) Close() error                        { return nil }
func (failingConn) Begin() (driver.Tx, error)           { return nil, errors.New("no tx") }

type failingStmt struct{}

func (failingStmt) Close() error  { return nil }
func (failingStmt) NumInput() int { return -1 }
func (failingStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, &pgconn.PgError{
		Code:           "22P02",
		Message:        `invalid input syntax for type uuid: "` + secretCell + `"`,
		Detail:         "Key (note)=(" + secretCell + ") already exists.",
		TableName:      "board_cells",
		ColumnName:     "note",
		ConstraintName: "board_cells_note_key",
	}
}
func (failingStmt) Query([]driver.Value) (driver.Rows, error) { return nil, errors.New("no rows") }

func init() { sql.Register("dev4280-failing", failingDriver{}) }

type boardCell struct {
	ID   int
	Note string
}

func TestRedactedLogger_FailingWriteLogsNoValue(t *testing.T) {
	sqlDB, err := sql.Open("dev4280-failing", "")
	require.NoError(t, err)
	var out bytes.Buffer
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, WithoutReturning: true}), &gorm.Config{
		Logger:                 NewRedactedGormLogger(&out),
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	err = db.Exec("UPDATE board_cells SET note = ? WHERE id = ?", secretCell, 7).Error
	require.Error(t, err, "the write fails")

	logged := out.String()
	require.NotEmpty(t, logged, "a failing write is still logged")
	require.NotContains(t, logged, secretCell, "the value is not in the log")
	require.Contains(t, logged, "UPDATE board_cells SET note = $1 WHERE id = $2", "the statement keeps its placeholders")
	require.Contains(t, logged, "postgres error 22P02, table board_cells, column note, constraint board_cells_note_key")
	require.Contains(t, logged, "log_redact_test.go:", "the line names the code that ran the statement")
	require.NotContains(t, logged, "/log_redact.go:", "not the logger itself")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "the caller still gets the real error")
	require.Contains(t, pgErr.Message, secretCell)
}

func TestRedactedLogger_SlowStatementLogsNoValue(t *testing.T) {
	var out bytes.Buffer
	l := NewRedactedGormLogger(&out)
	sqlText, vars := l.(gorm.ParamsFilter).ParamsFilter(context.Background(), "UPDATE board_cells SET note = $1", secretCell)
	require.Nil(t, vars)

	l.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) { return sqlText, 1 }, nil)

	logged := out.String()
	require.Contains(t, logged, "SLOW SQL")
	require.Contains(t, logged, "UPDATE board_cells SET note = $1")
	require.NotContains(t, logged, secretCell)
}

func TestRedactDBError(t *testing.T) {
	require.NoError(t, RedactDBError(nil))
	plain := errors.New("context deadline exceeded")
	require.Same(t, plain, RedactDBError(plain), "a non-Postgres error passes through")

	pg := &pgconn.PgError{Code: "23505", Message: "duplicate key " + secretCell, ConstraintName: "users_email_key"}
	got := RedactDBError(pg)
	require.Equal(t, "postgres error 23505, constraint users_email_key", got.Error())
	require.ErrorIs(t, got, pg, "errors.Is still finds the original")
	require.False(t, strings.Contains(got.Error(), secretCell))

	require.ErrorIs(t, RedactDBError(gorm.ErrRecordNotFound), gorm.ErrRecordNotFound)
}

func TestRedactedLogger_RecordNotFoundStaysQuiet(t *testing.T) {
	var out bytes.Buffer
	l := NewRedactedGormLogger(&out)
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, gorm.ErrRecordNotFound)
	require.Empty(t, out.String())
}
