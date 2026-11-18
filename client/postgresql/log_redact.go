package postgresql

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm/logger"
)

// NewRedactedGormLogger is the gorm logger every service gets: it prints real
// errors and slow statements (DEV-3338) but never the values in them
// (DEV-4280). A slow or failing write used to be printed with its values
// filled in, which put cell contents, comment text and other user data in the
// service logs. Now the statement keeps its $1, $2 placeholders, and a
// Postgres error is cut down to its SQLSTATE code, table and constraint,
// because the server's message can quote a value
// (`invalid input syntax for type uuid: "…"`).
func NewRedactedGormLogger(w io.Writer) logger.Interface {
	return redactedLogger{logger.New(log.New(w, "", log.LstdFlags), logger.Config{
		SlowThreshold:             500 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	})}
}

type redactedLogger struct {
	logger.Interface
}

func (l redactedLogger) LogMode(level logger.LogLevel) logger.Interface {
	return redactedLogger{l.Interface.LogMode(level)}
}

// ParamsFilter drops the values before gorm builds the logged statement.
// gorm asks the logger for this through an interface check
// (callbacks.go: db.Logger.(gorm.ParamsFilter)), so the wrapper must answer it
// itself: the embedded logger's method is not visible through the interface.
func (redactedLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}

func (l redactedLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.Interface.Trace(ctx, begin, fc, RedactDBError(err))
}

// RedactDBError replaces a Postgres error with one that says only what failed
// and where: SQLSTATE code, table, column and constraint. It still unwraps to
// the original, so errors.Is / errors.As keep working. Other errors pass
// through unchanged.
func RedactDBError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	return redactedPgError{pg: pgErr, err: err}
}

type redactedPgError struct {
	pg  *pgconn.PgError
	err error
}

func (e redactedPgError) Error() string {
	parts := []string{"postgres error " + e.pg.Code}
	for _, f := range []struct{ name, value string }{
		{"table", e.pg.TableName},
		{"column", e.pg.ColumnName},
		{"constraint", e.pg.ConstraintName},
	} {
		if f.value != "" {
			parts = append(parts, fmt.Sprintf("%s %s", f.name, f.value))
		}
	}
	return strings.Join(parts, ", ")
}

func (e redactedPgError) Unwrap() error { return e.err }
