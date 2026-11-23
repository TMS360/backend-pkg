package tmsgraphql

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DEV-3081: a request the client abandoned (navigated away, router cancel) is
// not a server fault. It must reach no Sentry severity, and the client gets a
// REQUEST_CANCELED / 499 answer instead of a masked 500.
func TestErrorPresenter_ClientCancelSkipsSentry(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
	}{
		{"context.Canceled", context.Background(), context.Canceled},
		{"wrapped context.Canceled", context.Background(), fmt.Errorf("load shipments: %w", context.Canceled)},
		{"gRPC Canceled from a subservice", context.Background(), status.Error(codes.Canceled, "context canceled")},
		{"any error on an already-cancelled request", cancelled, errors.New("driver: bad connection")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := withCaptureSpy(t)

			gqlErr := NewErrorPresenter(false)(tc.ctx, tc.err)

			if spy.errors != 0 || spy.warnings != 0 {
				t.Errorf("captures = (err:%d, warn:%d), want (0, 0)", spy.errors, spy.warnings)
			}
			if got := gqlErr.Extensions["code"]; got != "REQUEST_CANCELED" {
				t.Errorf("Extensions[code] = %v, want REQUEST_CANCELED", got)
			}
			if got := gqlErr.Extensions["status"]; got != statusClientClosedRequest {
				t.Errorf("Extensions[status] = %v, want %d", got, statusClientClosedRequest)
			}
		})
	}
}

// DEV-3081: a server-side timeout is a real problem and must stay visible in
// Sentry, as a warning (it should not page), with a clean 504 for the client.
func TestErrorPresenter_DeadlineExceededWarns(t *testing.T) {
	spy := withCaptureSpy(t)

	gqlErr := NewErrorPresenter(false)(context.Background(), fmt.Errorf("query loads: %w", context.DeadlineExceeded))

	if spy.errors != 0 || spy.warnings != 1 {
		t.Errorf("captures = (err:%d, warn:%d), want (0, 1)", spy.errors, spy.warnings)
	}
	if gqlErr.Message != "Request timed out" {
		t.Errorf("Message = %q, want %q", gqlErr.Message, "Request timed out")
	}
	if got := gqlErr.Extensions["code"]; got != "DEADLINE_EXCEEDED" {
		t.Errorf("Extensions[code] = %v, want DEADLINE_EXCEEDED", got)
	}
	if got := gqlErr.Extensions["status"]; got != http.StatusGatewayTimeout {
		t.Errorf("Extensions[status] = %v, want %d", got, http.StatusGatewayTimeout)
	}
}
