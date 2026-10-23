package postgresql

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGormLoggerSkipsRecordNotFound(t *testing.T) {
	var buf bytes.Buffer
	l := newGormLogger(&buf)
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		return `SELECT * FROM "local_settings" LIMIT 1`, 0
	}, gorm.ErrRecordNotFound)
	if buf.Len() != 0 {
		t.Fatalf("record not found should print nothing, got %q", buf.String())
	}
}

func TestGormLoggerPrintsRealErrorsWithoutColour(t *testing.T) {
	var buf bytes.Buffer
	l := newGormLogger(&buf)
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		return `SELECT broken`, 0
	}, errors.New("syntax error"))
	out := buf.String()
	if !strings.Contains(out, "syntax error") || !strings.Contains(out, "SELECT broken") {
		t.Fatalf("real error must print with its SQL, got %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("no colour codes expected, got %q", out)
	}
}

func TestGormLoggerSkipsFastQueries(t *testing.T) {
	var buf bytes.Buffer
	l := newGormLogger(&buf)
	l.Trace(context.Background(), time.Now(), func() (string, int64) {
		return `SELECT 1`, 1
	}, nil)
	if buf.Len() != 0 {
		t.Fatalf("fast query should print nothing, got %q", buf.String())
	}
}

func TestGormLoggerPrintsSlowQueries(t *testing.T) {
	var buf bytes.Buffer
	l := newGormLogger(&buf)
	l.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) {
		return `SELECT slow`, 1
	}, nil)
	if !strings.Contains(buf.String(), "SLOW SQL") {
		t.Fatalf("slow query should print, got %q", buf.String())
	}
}
