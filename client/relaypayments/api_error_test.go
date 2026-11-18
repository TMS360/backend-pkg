package relaypayments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TMS360/backend-pkg/config"
)

// DEV-3071: a non-auth Relay refusal keeps the whole answer for the caller,
// while Error() reads exactly as before.
func TestAPIError_KeepsWholeAnswer(t *testing.T) {
	cases := []struct {
		name, body, wantErr, wantMsg string
	}{
		{"envelope", `{"message":"Bad request","errors":{"id":["gone"]}}`, "relaypayments: status 400: Bad request", "Bad request"},
		{"no envelope", `oops`, "relaypayments: status 400: oops", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Request-Id", "req_1")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			cli, err := NewClient(config.RelayConfig{Host: srv.URL}, "k")
			if err != nil {
				t.Fatal(err)
			}
			err = cli.DeletePolicyAssignment(context.Background(), "pa_1")
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			ae, ok := AsAPIError(err)
			if !ok {
				t.Fatalf("want *APIError, got %T", err)
			}
			if ae.StatusCode != 400 || ae.Body != tc.body || ae.RequestID != "req_1" || ae.Message != tc.wantMsg {
				t.Errorf("got %+v", *ae)
			}
		})
	}
}
