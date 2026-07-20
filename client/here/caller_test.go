package here

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient points a Client at a stub that always answers with body/status,
// so doRequest runs its real logging path without touching HERE.
func newTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &Client{
		httpClient:  srv.Client(),
		routerHost:  srv.URL,
		geocodeHost: srv.URL,
		lookupHost:  srv.URL,
		apiKey:      "SECRET-KEY-DO-NOT-LOG",
	}
}

// captureLogs swaps the default slog handler for the duration of the test —
// logCall writes through slog.Default().
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return &buf
}

// The URL carries the credential as a query parameter, so a naive "log the
// request" would publish the API key on every call.
func TestDoRequest_NeverLogsAPIKey(t *testing.T) {
	buf := captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"items":[]}`)

	_, err := c.Geocode(context.Background(), GeocodeRequest{Query: "1 Main St", Limit: 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "here_call") {
		t.Fatalf("no here_call line emitted: %q", out)
	}
	if strings.Contains(out, "SECRET-KEY-DO-NOT-LOG") {
		t.Fatalf("API key leaked into logs: %q", out)
	}
	if strings.Contains(out, "apiKey") {
		t.Fatalf("request URL leaked into logs: %q", out)
	}
}

func TestDoRequest_LogsOpAndOutcome(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		call        func(*Client) error
		wantOp      string
		wantOutcome string
	}{
		{
			name:   "geocode ok",
			status: http.StatusOK,
			body:   `{"items":[]}`,
			call: func(c *Client) error {
				_, err := c.Geocode(context.Background(), GeocodeRequest{Query: "x", Limit: 1})
				return err
			},
			wantOp:      "geocode",
			wantOutcome: "ok",
		},
		{
			name:   "routes ok",
			status: http.StatusOK,
			body:   `{"routes":[{"id":"r1","sections":[{"summary":{"length":100,"duration":10,"baseDuration":10}}]}]}`,
			call: func(c *Client) error {
				_, err := c.GetTruckRoute(context.Background(), Coordinates{Latitude: 41, Longitude: 69},
					Coordinates{Latitude: 42, Longitude: 70}, nil)
				return err
			},
			wantOp:      "routes",
			wantOutcome: "ok",
		},
		{
			name:   "lookup auth failure",
			status: http.StatusUnauthorized,
			body:   `{"error":"unauthorized"}`,
			call: func(c *Client) error {
				_, err := c.LookupByID(context.Background(), "here:cm:namedplace:1")
				return err
			},
			wantOp:      "lookup",
			wantOutcome: "auth",
		},
		{
			name:   "revgeocode server error",
			status: http.StatusInternalServerError,
			body:   `{"error":"boom"}`,
			call: func(c *Client) error {
				_, err := c.ReverseGeocode(context.Background(), 41, 69)
				return err
			},
			wantOp:      "revgeocode",
			wantOutcome: "http_5xx",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLogs(t)
			c := newTestClient(t, tc.status, tc.body)

			_ = tc.call(c)

			out := buf.String()
			if !strings.Contains(out, "op="+tc.wantOp) {
				t.Errorf("op=%s not found in %q", tc.wantOp, out)
			}
			if !strings.Contains(out, "outcome="+tc.wantOutcome) {
				t.Errorf("outcome=%s not found in %q", tc.wantOutcome, out)
			}
		})
	}
}

// TestConnection bypasses doRequest to preserve its ErrInvalidCredentials
// contract, so it needs its own proof that it still counts against the meter.
func TestTestConnection_IsCounted(t *testing.T) {
	buf := captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"items":[]}`)

	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "op=revgeocode_testconn") {
		t.Errorf("test-connection call not counted: %q", out)
	}
	if strings.Contains(out, "SECRET-KEY-DO-NOT-LOG") {
		t.Errorf("API key leaked into logs: %q", out)
	}
}

func TestTestConnection_KeepsErrInvalidCredentials(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)

	// tms360-backend's integration classify matches this sentinel to render
	// "Invalid API key"; an *AuthError here would silently degrade it to
	// "Connection failed".
	if err := c.TestConnection(context.Background()); err != ErrInvalidCredentials {
		t.Fatalf("got %v, want ErrInvalidCredentials", err)
	}
}

// An explicit tag exists for call sites whose stack does not name them.
func TestResolveCaller_CtxTagWins(t *testing.T) {
	ctx := WithCaller(context.Background(), "mileage_debouncer")

	if got := resolveCaller(ctx); got != "mileage_debouncer" {
		t.Fatalf("got %q, want %q", got, "mileage_debouncer")
	}
}

// A tagged context must survive all the way down to the log line, since that is
// the whole point for goroutine-spawned call sites.
func TestDoRequest_CtxTagReachesLog(t *testing.T) {
	buf := captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"items":[]}`)

	ctx := WithCaller(context.Background(), "shipment_create_bg")
	if _, err := c.Geocode(ctx, GeocodeRequest{Query: "x", Limit: 1}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if out := buf.String(); !strings.Contains(out, "caller=shipment_create_bg") {
		t.Fatalf("ctx caller tag missing from log: %q", out)
	}
}

// Every here_call must name someone: an untagged call site still has to be
// attributed from the stack rather than logged as anonymous.
func TestDoRequest_UntaggedCallIsStillAttributed(t *testing.T) {
	buf := captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"items":[]}`)

	if _, err := c.Geocode(context.Background(), GeocodeRequest{Query: "x", Limit: 1}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "caller=unknown") {
		t.Fatalf("stack walk failed to attribute the call: %q", out)
	}
	if !strings.Contains(out, "caller=") {
		t.Fatalf("no caller attribute emitted: %q", out)
	}
}

// The stack walk itself is exercised through doRequest in the tests above; here
// the selection rule is fed synthetic frames, because a test in this package has
// all of its own frames filtered out as internal plumbing.
func TestChainFrom_DropsInternalFrames(t *testing.T) {
	got := chainFrom([]string{
		pkgPrefix + "(*Client).doRequest",
		pkgPrefix + "(*Client).GetRoute",
		pkgPrefix + "(*service).CalculateTruckRoute",
		"tms-load/internal/service/mileage.CalcRouteMilesWithRetry",
		"tms-load/internal/service/trip.(*Updater).RecalcTripMiles",
	})

	want := "mileage.CalcRouteMilesWithRetry<-trip.RecalcTripMiles"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The reason a single frame is not enough: distinct paths funnel through one
// shared wrapper, and reporting only that wrapper buckets them together.
func TestChainFrom_DoesNotCollapseAtSharedWrapper(t *testing.T) {
	const wrapper = "tms-load/internal/service/mileage.CalcRouteMilesWithRetry"

	createPath := chainFrom([]string{
		pkgPrefix + "(*Client).doRequest",
		wrapper,
		"tms-load/internal/service/shipment.(*Creator).computeAndPersistRoute",
	})
	recalcPath := chainFrom([]string{
		pkgPrefix + "(*Client).doRequest",
		wrapper,
		"tms-load/internal/service/trip.(*Updater).RecalcTripMiles",
	})

	if createPath == recalcPath {
		t.Fatalf("distinct call paths collapsed to the same tag: %q", createPath)
	}
	if !strings.HasPrefix(createPath, "mileage.CalcRouteMilesWithRetry<-") {
		t.Errorf("chain %q does not reach past the shared wrapper", createPath)
	}
}

func TestChainFrom_CapsDepth(t *testing.T) {
	frames := []string{
		"a/pkg1.F1", "b/pkg2.F2", "c/pkg3.F3", "d/pkg4.F4",
		"e/pkg5.F5", "f/pkg6.F6", "g/pkg7.F7",
	}

	got := chainFrom(frames)

	if n := strings.Count(got, "<-") + 1; n != callerChainDepth {
		t.Fatalf("chain has %d frames, want %d: %q", n, callerChainDepth, got)
	}
}

func TestChainFrom_AllInternal(t *testing.T) {
	got := chainFrom([]string{pkgPrefix + "(*Client).doRequest", pkgPrefix + "(*Client).Geocode"})

	if got != "unknown" {
		t.Fatalf("got %q, want %q", got, "unknown")
	}
}

func TestChainFrom_HasNoSpaces(t *testing.T) {
	// TextHandler quotes any value containing a space, which would break
	// `grep -o 'caller=[^ ]*'` in the sampling commands.
	got := chainFrom([]string{"tms-load/internal/service/trip.(*Updater).RecalcTripMiles"})

	if strings.ContainsAny(got, " \t") {
		t.Fatalf("caller chain %q contains whitespace", got)
	}
}

func TestShortFuncName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"tms-load/internal/service/trip.(*Updater).RecalcTripMiles", "trip.RecalcTripMiles"},
		{"tms-load/internal/service/mileage.CalcRouteMilesWithRetry", "mileage.CalcRouteMilesWithRetry"},
		{"main.main", "main.main"},
	}

	for _, tc := range tests {
		if got := shortFuncName(tc.in); got != tc.want {
			t.Errorf("shortFuncName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
