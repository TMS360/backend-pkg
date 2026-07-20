package here

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// newCountingService serves bodies in order and counts requests, so tests can
// assert how many billed HERE transactions a call actually costs.
func newCountingService(t *testing.T, bodies ...string) (*service, func() int, func() []*http.Request) {
	t.Helper()

	var (
		mu     sync.Mutex
		calls  int
		gotReq []*http.Request
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		body := bodies[0]
		if calls < len(bodies) {
			body = bodies[calls]
		} else {
			body = bodies[len(bodies)-1]
		}
		calls++
		gotReq = append(gotReq, r)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		httpClient:  srv.Client(),
		routerHost:  srv.URL,
		geocodeHost: srv.URL,
		lookupHost:  srv.URL,
		apiKey:      "test-key",
	}

	countFn := func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
	reqFn := func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return gotReq
	}

	return &service{client: c}, countFn, reqFn
}

// sectionsBody builds a routes response with one section per leg, which is what
// HERE returns for a plain (stopover) via route.
func sectionsBody(lengths, baseDurations, durations []int) string {
	body := `{"routes":[{"id":"r1","sections":[`
	for i := range lengths {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"id":"s%d","summary":{"length":%d,"duration":%d,"baseDuration":%d},"polyline":"poly%d"}`,
			i, lengths[i], durations[i], baseDurations[i], i)
	}
	return body + `]}]}`
}

func fourStops() []Coordinates {
	return []Coordinates{
		{Latitude: 41.0, Longitude: 69.0},
		{Latitude: 41.1, Longitude: 69.1},
		{Latitude: 41.2, Longitude: 69.2},
		{Latitude: 41.3, Longitude: 69.3},
	}
}

// The point of the change: one route, one paid transaction.
func TestCalculateMultiStopRoute_SingleHTTPCall(t *testing.T) {
	body := sectionsBody([]int{100, 200, 300}, []int{10, 20, 30}, []int{11, 22, 33})
	svc, calls, reqs := newCountingService(t, body)

	_, err := svc.CalculateMultiStopRoute(context.Background(), fourStops(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if got := calls(); got != 1 {
		t.Fatalf("made %d HERE calls for a 4-stop route, want 1", got)
	}

	via := reqs()[0].URL.Query()["via"]
	if len(via) != 2 {
		t.Fatalf("via count=%d, want 2 (got %v)", len(via), via)
	}
	// Order matters: a resequenced route would silently contradict the stop
	// sequence the dispatcher set.
	if via[0] != "41.100000,69.100000" || via[1] != "41.200000,69.200000" {
		t.Errorf("via points out of order: %v", via)
	}

	q := reqs()[0].URL.Query()
	if q.Get("origin") != "41.000000,69.000000" {
		t.Errorf("origin=%q", q.Get("origin"))
	}
	if q.Get("destination") != "41.300000,69.300000" {
		t.Errorf("destination=%q", q.Get("destination"))
	}
	if q.Get("transportMode") != "truck" {
		t.Errorf("transportMode=%q, want truck", q.Get("transportMode"))
	}
	// Without polyline the per-leg geometry stored by asset-tracking would be
	// empty.
	if q.Get("return") != "summary,polyline" {
		t.Errorf("return=%q, want summary,polyline", q.Get("return"))
	}
}

func TestCalculateMultiStopRoute_MapsSectionsToLegs(t *testing.T) {
	body := sectionsBody([]int{100, 200, 300}, []int{10, 20, 30}, []int{11, 22, 33})
	svc, _, _ := newCountingService(t, body)

	stops := fourStops()
	got, err := svc.CalculateMultiStopRoute(context.Background(), stops, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(got.Legs) != 3 {
		t.Fatalf("got %d legs, want 3", len(got.Legs))
	}
	if got.TotalDistanceMeters != 600 {
		t.Errorf("TotalDistanceMeters=%d, want 600", got.TotalDistanceMeters)
	}
	// Totals stay traffic-free (baseDuration), as before the change.
	if got.TotalDurationSeconds != 60 {
		t.Errorf("TotalDurationSeconds=%d, want 60", got.TotalDurationSeconds)
	}

	wantDist := []int{100, 200, 300}
	wantDur := []int{10, 20, 30}
	for i, leg := range got.Legs {
		if leg.Index != i {
			t.Errorf("leg %d: Index=%d", i, leg.Index)
		}
		if leg.DistanceMeters != wantDist[i] {
			t.Errorf("leg %d: DistanceMeters=%d, want %d", i, leg.DistanceMeters, wantDist[i])
		}
		if leg.DurationSeconds != wantDur[i] {
			t.Errorf("leg %d: DurationSeconds=%d, want %d", i, leg.DurationSeconds, wantDur[i])
		}
		if leg.Polyline != fmt.Sprintf("poly%d", i) {
			t.Errorf("leg %d: Polyline=%q", i, leg.Polyline)
		}
		if leg.Origin != stops[i] || leg.Destination != stops[i+1] {
			t.Errorf("leg %d: endpoints %v→%v", i, leg.Origin, leg.Destination)
		}
	}
}

// Arrival must chain on traffic-aware duration, matching what the per-leg walk
// produced by departing each leg at the previous leg's arrival.
func TestCalculateMultiStopRoute_ArrivalChainsOnTrafficDuration(t *testing.T) {
	body := sectionsBody([]int{100, 200, 300}, []int{10, 20, 30}, []int{11, 22, 33})
	svc, _, _ := newCountingService(t, body)

	depart := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	got, err := svc.CalculateMultiStopRoute(context.Background(), fourStops(), &depart)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	want := []time.Time{
		depart.Add(11 * time.Second),
		depart.Add(33 * time.Second),
		depart.Add(66 * time.Second),
	}
	for i, leg := range got.Legs {
		if !leg.EstimatedArrival.Equal(want[i]) {
			t.Errorf("leg %d: arrival=%v, want %v", i, leg.EstimatedArrival, want[i])
		}
	}
}

// A two-stop route has no via points and must still work.
func TestCalculateMultiStopRoute_TwoStops(t *testing.T) {
	body := sectionsBody([]int{500}, []int{50}, []int{55})
	svc, calls, reqs := newCountingService(t, body)

	got, err := svc.CalculateMultiStopRoute(context.Background(), []Coordinates{
		{Latitude: 41.0, Longitude: 69.0},
		{Latitude: 42.0, Longitude: 70.0},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if got := calls(); got != 1 {
		t.Fatalf("made %d calls, want 1", got)
	}
	if via := reqs()[0].URL.Query()["via"]; len(via) != 0 {
		t.Errorf("via=%v, want none", via)
	}
	if len(got.Legs) != 1 || got.TotalDistanceMeters != 500 {
		t.Errorf("got %d legs, total=%d", len(got.Legs), got.TotalDistanceMeters)
	}
}

// HERE opens a new section on a transport-mode change (e.g. a ferry), so the
// count can exceed the legs. Mapping by index would then misattribute geometry;
// the fallback re-requests each pair instead.
func TestCalculateMultiStopRoute_SectionMismatchFallsBack(t *testing.T) {
	multiSection := `{"routes":[{"id":"r1","sections":[` +
		`{"id":"s0","summary":{"length":10,"duration":1,"baseDuration":1},"polyline":"a"},` +
		`{"id":"s1","summary":{"length":20,"duration":2,"baseDuration":2},"polyline":"b"},` +
		`{"id":"s2","summary":{"length":30,"duration":3,"baseDuration":3},"polyline":"c"},` +
		`{"id":"s3","summary":{"length":40,"duration":4,"baseDuration":4},"polyline":"d"},` +
		`{"id":"s4","summary":{"length":50,"duration":5,"baseDuration":5},"polyline":"e"}` +
		`]}]}`
	perLeg := sectionsBody([]int{7}, []int{1}, []int{1})

	// First response is the mismatching multi-section route; every later
	// response serves the per-leg fallback.
	svc, calls, _ := newCountingService(t, multiSection, perLeg)

	got, err := svc.CalculateMultiStopRoute(context.Background(), fourStops(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// 1 combined attempt + 3 per-leg calls.
	if n := calls(); n != 4 {
		t.Fatalf("made %d calls, want 4 (1 combined + 3 fallback legs)", n)
	}
	if len(got.Legs) != 3 {
		t.Fatalf("got %d legs, want 3", len(got.Legs))
	}
	if got.TotalDistanceMeters != 21 {
		t.Errorf("TotalDistanceMeters=%d, want 21 (3 fallback legs × 7)", got.TotalDistanceMeters)
	}
}

func TestCalculateMultiStopRoute_Rejects(t *testing.T) {
	svc, calls, _ := newCountingService(t, sectionsBody([]int{1}, []int{1}, []int{1}))

	if _, err := svc.CalculateMultiStopRoute(context.Background(), []Coordinates{{Latitude: 1, Longitude: 1}}, nil); err == nil {
		t.Error("expected error for a single waypoint")
	}
	if n := calls(); n != 0 {
		t.Errorf("made %d HERE calls for an invalid request, want 0", n)
	}

	nilSvc := &service{}
	if _, err := nilSvc.CalculateMultiStopRoute(context.Background(), fourStops(), nil); err == nil {
		t.Error("expected error when client is not configured")
	}
}

func TestCalculateMultiStopRoute_NoRoutes(t *testing.T) {
	svc, _, _ := newCountingService(t, `{"routes":[]}`)

	if _, err := svc.CalculateMultiStopRoute(context.Background(), fourStops(), nil); err == nil {
		t.Fatal("expected error when HERE returns no routes")
	}
}
