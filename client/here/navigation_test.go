package here

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestNavService spins up an httptest server that responds with `body`,
// captures the inbound request, and returns a NavigationService pointed at it.
func newTestNavService(t *testing.T, body string, captured **http.Request) NavigationService {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			cp := *r
			*captured = &cp
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		httpClient: srv.Client(),
		routerHost: srv.URL,
		apiKey:     "test-key",
	}
	return NewNavigationService(c)
}

func TestBuildTruckRoute_URLContainsTruckParams(t *testing.T) {
	body := `{"routes":[{"id":"r1","sections":[{"id":"s1","summary":{"length":1000,"duration":120,"baseDuration":100},"polyline":""}]}]}`
	var got *http.Request
	svc := newTestNavService(t, body, &got)

	_, err := svc.BuildTruckRoute(context.Background(), TruckNavRequest{
		Origin:      Coordinates{Latitude: 41.0, Longitude: 69.0},
		Destination: Coordinates{Latitude: 41.5, Longitude: 69.5},
		Truck: TruckAttributes{
			GrossWeightKg:         36000,
			HeightCm:              400,
			ShippedHazardousGoods: []string{"flammable"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil {
		t.Fatal("no request captured")
	}

	q := got.URL.Query()
	if q.Get("transportMode") != "truck" {
		t.Errorf("transportMode=%q, want truck", q.Get("transportMode"))
	}
	if q.Get("truck[grossWeight]") != "36000" {
		t.Errorf("truck[grossWeight]=%q, want 36000", q.Get("truck[grossWeight]"))
	}
	if q.Get("truck[height]") != "400" {
		t.Errorf("truck[height]=%q, want 400", q.Get("truck[height]"))
	}
	if q.Get("vehicle[shippedHazardousGoods]") != "flammable" {
		t.Errorf("hazmat=%q, want flammable", q.Get("vehicle[shippedHazardousGoods]"))
	}
	ret := q.Get("return")
	for _, must := range []string{"actions", "instructions", "polyline", "spans", "notices"} {
		if !strings.Contains(ret, must) {
			t.Errorf("return=%q missing %q", ret, must)
		}
	}
}

func TestBuildTruckRouteMultiStop_SingleHTTPCallWithVia(t *testing.T) {
	body := `{"routes":[{"id":"r1","sections":[{"id":"s1","summary":{"length":500,"duration":60,"baseDuration":50},"polyline":""}]}]}`
	var got *http.Request
	svc := newTestNavService(t, body, &got)

	_, err := svc.BuildTruckRouteMultiStop(context.Background(), TruckNavMultiStopRequest{
		Stops: []Coordinates{
			{Latitude: 41.0, Longitude: 69.0},
			{Latitude: 41.1, Longitude: 69.1},
			{Latitude: 41.2, Longitude: 69.2},
			{Latitude: 41.3, Longitude: 69.3},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil {
		t.Fatal("no request captured")
	}

	via := got.URL.Query()["via"]
	if len(via) != 2 {
		t.Fatalf("via count=%d, want 2 (got %v)", len(via), via)
	}
}

func TestBuildTruckRoute_AlternativesParsed(t *testing.T) {
	body := `{"routes":[
		{"id":"r1","sections":[{"id":"s1","summary":{"length":100,"duration":10,"baseDuration":10},"polyline":""}]},
		{"id":"r2","sections":[{"id":"s2","summary":{"length":200,"duration":20,"baseDuration":20},"polyline":""}]}
	]}`
	svc := newTestNavService(t, body, nil)

	res, err := svc.BuildTruckRoute(context.Background(), TruckNavRequest{
		Origin:       Coordinates{Latitude: 41, Longitude: 69},
		Destination:  Coordinates{Latitude: 42, Longitude: 70},
		Alternatives: 1,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Alternatives) != 1 {
		t.Fatalf("alternatives=%d, want 1", len(res.Alternatives))
	}
	if res.Alternatives[0].DistanceMeters != 200 {
		t.Errorf("alt distance=%d, want 200", res.Alternatives[0].DistanceMeters)
	}
}

func TestBuildTruckRoute_ParsesActionsAndNotices(t *testing.T) {
	body := `{"routes":[{"id":"r1","sections":[{
		"id":"s1",
		"summary":{"length":1000,"duration":120,"baseDuration":100},
		"polyline":"BFoz5xJ67i1B1B7PzIhaxL7Y",
		"actions":[
			{"action":"depart","duration":10,"length":50,"instruction":"Head north","offset":0},
			{"action":"turn","direction":"right","duration":20,"length":100,"instruction":"Turn right onto Main St","offset":1,
			 "nextRoad":{"name":[{"value":"Main St","language":"en"}]}}
		],
		"notices":[
			{"title":"Truck restriction","code":"violatedTruckRestriction","severity":"critical"}
		]
	}]}]}`
	svc := newTestNavService(t, body, nil)

	res, err := svc.BuildTruckRoute(context.Background(), TruckNavRequest{
		Origin:      Coordinates{Latitude: 41, Longitude: 69},
		Destination: Coordinates{Latitude: 42, Longitude: 70},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Steps) != 2 {
		t.Fatalf("steps=%d, want 2", len(res.Steps))
	}
	if res.Steps[1].RoadName != "Main St" {
		t.Errorf("road name=%q, want Main St", res.Steps[1].RoadName)
	}
	if len(res.Notices) != 1 || res.Notices[0].Severity != "critical" {
		t.Errorf("notices=%+v", res.Notices)
	}
}

func TestSanitizeOffset(t *testing.T) {
	cases := []struct{ off, length, want int }{
		{0, 0, 0},
		{-5, 10, 0},
		{15, 10, 9},
		{3, 10, 3},
	}
	for _, c := range cases {
		if got := sanitizeOffset(c.off, c.length); got != c.want {
			t.Errorf("sanitizeOffset(%d,%d)=%d, want %d", c.off, c.length, got, c.want)
		}
	}
}
