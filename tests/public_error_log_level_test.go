package tests

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1970 — building a 4xx used to write an ERROR log line, so every routine
// "you are not allowed" sat in the logs next to real faults. 4xx now logs at
// WARN; 5xx keeps ERROR.

// captureSlog swaps the default logger for a buffer and returns it.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestPublicError_4xxLogsAsWarning(t *testing.T) {
	cases := map[string]func() response.PublicError{
		"forbidden":  func() response.PublicError { return response.NewForbidden("tech", "user") },
		"badRequest": func() response.PublicError { return response.NewBadRequest("tech", "user") },
		"notFound":   func() response.PublicError { return response.NewNotFound("Trip", "id") },
		"codedDenial": func() response.PublicError {
			return response.NewCodedError("FORBIDDEN", "tech", "user", http.StatusForbidden, nil)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			buf := captureSlog(t)
			require.NotNil(t, build())

			out := buf.String()
			assert.Contains(t, out, "level=WARN")
			assert.NotContains(t, out, "level=ERROR")
		})
	}
}

func TestPublicError_5xxStillLogsAsError(t *testing.T) {
	buf := captureSlog(t)
	require.NotNil(t, response.NewInternalError("the database is gone"))

	assert.Contains(t, buf.String(), "level=ERROR")
}

// The user-facing half of a denial never carries the technical half.
func TestPublicError_UserMessageIsSeparateFromTechnical(t *testing.T) {
	_ = captureSlog(t)
	err := response.NewForbidden("access denied: missing role (requires one of [super_admin])",
		"You are not allowed to do this. Ask an administrator if you need access.")

	assert.False(t, strings.Contains(err.UserMessage(), "super_admin"))
	assert.Equal(t, http.StatusForbidden, err.ErrorStatus())
}
