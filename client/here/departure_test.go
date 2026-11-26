package here

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// departureStub answers every route request with `sections` one-section legs of
// 1000 m and records the departureTime each request carried.
func departureStub(t *testing.T, sections int) (Service, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var seen []string
	body := `{"routes":[{"sections":[` + strings.TrimSuffix(strings.Repeat(`{"summary":{"length":1000}},`, sections), ",") + `]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Query().Get("departureTime"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	svc := NewService(&Client{httpClient: srv.Client(), routerHost: srv.URL, apiKey: "k"})
	return svc, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

var threeStops = []Coordinates{{Latitude: 41.8, Longitude: -87.6}, {Latitude: 39.7, Longitude: -86.1}, {Latitude: 38.6, Longitude: -90.2}}

func TestMultiStopRoute_TimeIndependent(t *testing.T) {
	cases := []struct {
		name     string
		sections int // 2 = one call; 1 = section count mismatch, per-leg calls
		calls    int
	}{
		{"single call", 2, 1},
		{"per-leg fallback", 1, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, seen := departureStub(t, tc.sections)
			ctx := WithTimeIndependentRoute(context.Background())
			if _, err := svc.CalculateMultiStopRoute(ctx, threeStops, nil); err != nil {
				t.Fatalf("route: %v", err)
			}
			got := seen()
			if len(got) != tc.calls {
				t.Fatalf("calls = %d, want %d", len(got), tc.calls)
			}
			for i, d := range got {
				if d != "any" {
					t.Fatalf("call %d departureTime = %q, want any", i, d)
				}
			}
		})
	}
}

func TestMultiStopRoute_DefaultKeepsDepartureTime(t *testing.T) {
	svc, seen := departureStub(t, 2)
	if _, err := svc.CalculateMultiStopRoute(context.Background(), threeStops, nil); err != nil {
		t.Fatalf("route: %v", err)
	}
	got := seen()
	if len(got) != 1 {
		t.Fatalf("calls = %d, want 1", len(got))
	}
	if _, err := time.Parse(time.RFC3339, got[0]); err != nil {
		t.Fatalf("departureTime = %q, want an RFC3339 time", got[0])
	}
}
