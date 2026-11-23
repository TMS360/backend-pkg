package tmsgraphql

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGrpcPublicError(t *testing.T) {
	// wrap mirrors how the mediator surfaces a broker gRPC error:
	//   fmt.Errorf("failed to communicate with broker service: %w", err)
	wrap := func(err error) error {
		return fmt.Errorf("failed to communicate with broker service: %w", err)
	}

	cases := []struct {
		name       string
		err        error
		wantOK     bool
		wantCode   string
		wantStatus int
		wantMsg    string
	}{
		{"unavailable (the incident), wrapped", wrap(status.Error(codes.Unavailable, "FMCSA verification is temporarily unavailable, please try again shortly")), true, "SERVICE_UNAVAILABLE", http.StatusServiceUnavailable, "FMCSA verification is temporarily unavailable, please try again shortly"},
		{"failed precondition, wrapped", wrap(status.Error(codes.FailedPrecondition, "not authorized")), true, "FAILED_PRECONDITION", http.StatusUnprocessableEntity, "not authorized"},
		{"invalid argument", status.Error(codes.InvalidArgument, "query string cannot be empty"), true, "BAD_USER_INPUT", http.StatusBadRequest, "query string cannot be empty"},
		{"not found", status.Error(codes.NotFound, "no such broker"), true, "NOT_FOUND", http.StatusNotFound, "no such broker"},
		{"unknown stays 500", wrap(status.Error(codes.Unknown, "boom")), false, "", 0, ""},
		{"internal stays 500", status.Error(codes.Internal, "db down"), false, "", 0, ""},
		{"plain non-status error stays 500", errors.New("just an error"), false, "", 0, ""},
		{"nil", nil, false, "", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg, code, st, ok := grpcPublicError(c.err)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if code != c.wantCode || st != c.wantStatus || msg != c.wantMsg {
				t.Fatalf("got (%q, %q, %d), want (%q, %q, %d)", msg, code, st, c.wantMsg, c.wantCode, c.wantStatus)
			}
		})
	}
}
