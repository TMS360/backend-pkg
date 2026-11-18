package postgresql

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm/logger"
)

// slowStatement is when a statement that did not fail is still logged.
const slowStatement = 500 * time.Millisecond

// NewRedactedGormLogger is the gorm logger every service gets: it prints real
// errors and slow statements (DEV-3338) but never the values in them
// (DEV-4280). A slow or failing write used to be printed with its values
// filled in, which put cell contents, comment text and other user data in the
// service logs. Now the statement keeps its $1, $2 placeholders, and a
// Postgres error is cut down to its SQLSTATE code, table and constraint,
// because the server's message can quote a value
// (`invalid input syntax for type uuid: "…"`). The lines read like gorm's own
// and name the caller outside gorm and this file.
func NewRedactedGormLogger(w io.Writer) logger.Interface {
	return &redactedLogger{out: log.New(w, "", log.LstdFlags), level: logger.Warn}
}

type redactedLogger struct {
	out   *log.Logger
	level logger.LogLevel
}

func (l *redactedLogger) LogMode(level logger.LogLevel) logger.Interface {
	next := *l
	next.level = level
	return &next
}

func (l *redactedLogger) Info(_ context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Info {
		l.out.Printf("%s\n[info] "+msg, append([]interface{}{caller()}, data...)...)
	}
}

func (l *redactedLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Warn {
		l.out.Printf("%s\n[warn] "+msg, append([]interface{}{caller()}, data...)...)
	}
}

func (l *redactedLogger) Error(_ context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Error {
		l.out.Printf("%s\n[error] "+msg, append([]interface{}{caller()}, data...)...)
	}
}

// ParamsFilter drops the values before gorm builds the logged statement. gorm
// asks the logger for it through an interface check
// (callbacks.go: db.Logger.(gorm.ParamsFilter)).
func (*redactedLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}

func (l *redactedLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= logger.Silent {
		return
	}
	elapsed := time.Since(begin)
	ms := float64(elapsed.Nanoseconds()) / 1e6
	switch {
	case err != nil && l.level >= logger.Error && !errors.Is(err, logger.ErrRecordNotFound):
		sql, rows := fc()
		l.out.Printf("%s %s\n[%.3fms] [rows:%s] %s", caller(), RedactDBError(err), ms, rowCount(rows), placeholders(sql))
	case elapsed > slowStatement && l.level >= logger.Warn:
		sql, rows := fc()
		l.out.Printf("%s SLOW SQL >= %v\n[%.3fms] [rows:%s] %s", caller(), slowStatement, ms, rowCount(rows), placeholders(sql))
	case l.level == logger.Info:
		sql, rows := fc()
		l.out.Printf("%s\n[%.3fms] [rows:%s] %s", caller(), ms, rowCount(rows), placeholders(sql))
	}
}

func rowCount(rows int64) string {
	if rows == -1 {
		return "-"
	}
	return strconv.FormatInt(rows, 10)
}

// explainedPlaceholder is how gorm's postgres Explain leaves a placeholder it
// had no value for: $1 becomes $1$.
var explainedPlaceholder = regexp.MustCompile(`\$(\d+)\$`)

func placeholders(sql string) string {
	return explainedPlaceholder.ReplaceAllString(sql, "$$$1")
}

// caller is the first frame outside gorm and this file: the service code that
// ran the statement, as gorm's own logger reports it.
func caller() string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "/gorm.io/") && !strings.HasSuffix(frame.File, "/log_redact.go") {
			return frame.File + ":" + strconv.Itoa(frame.Line)
		}
		if !more {
			return ""
		}
	}
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
