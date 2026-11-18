package googlemaps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient points a Client at a stub that always answers with body/status,
// so the real request/decode path runs without touching Google.
func newTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &Client{
		httpClient:  srv.Client(),
		geocodeHost: srv.URL,
		routesHost:  srv.URL,
		apiKey:      "SECRET-KEY-DO-NOT-LOG",
	}
}

// captureLogs redirects slog to a buffer for the duration of a test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

const geocodeOK = `{
  "results": [{
    "formatted_address": "6440 W Howard St, Niles, IL 60714, USA",
    "place_id": "test-place",
    "types": ["street_address"],
    "geometry": {"location": {"lat": 42.0208, "lng": -87.8067}, "location_type": "ROOFTOP"},
    "address_components": [
      {"long_name": "6440", "short_name": "6440", "types": ["street_number"]},
      {"long_name": "West Howard Street", "short_name": "W Howard St", "types": ["route"]},
      {"long_name": "Niles", "short_name": "Niles", "types": ["locality"]},
      {"long_name": "Illinois", "short_name": "IL", "types": ["administrative_area_level_1"]},
      {"long_name": "60714", "short_name": "60714", "types": ["postal_code"]}
    ]
  }],
  "status": "OK"
}`

func TestGeocode_HappyPath(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusOK, geocodeOK)

	got, err := c.Geocode(context.Background(), "6440 W Howard St, Niles, IL")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Geometry.Location.Lat != 42.0208 {
		t.Errorf("lat = %v, want 42.0208", got[0].Geometry.Location.Lat)
	}
	if city := ComponentByType(got[0], "locality"); city != "Niles" {
		t.Errorf("locality = %q, want Niles", city)
	}
	// State must come from the abbreviated form, or a US address would be
	// stamped "Illinois" where the rest of the system expects "IL".
	if state := ShortComponentByType(got[0], "administrative_area_level_1"); state != "IL" {
		t.Errorf("state short name = %q, want IL", state)
	}
}

// ZERO_RESULTS is an answer, not an outage: it must not surface as an error, or
// callers would retry a question Google already answered.
func TestGeocode_ZeroResultsIsNotAnError(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"results":[],"status":"ZERO_RESULTS"}`)

	got, err := c.Geocode(context.Background(), "nowhere at all")
	if err != nil {
		t.Fatalf("ZERO_RESULTS must not be an error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

// The trap this client exists to handle: Google answers a bad key on the classic
// REST endpoints with HTTP 200 + REQUEST_DENIED, so status-line-only detection
// would read a rejected credential as success.
func TestGeocode_RequestDeniedIsAuthError(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"results":[],"status":"REQUEST_DENIED","error_message":"The provided API key is invalid."}`)

	_, err := c.Geocode(context.Background(), "anywhere")
	if err == nil {
		t.Fatal("REQUEST_DENIED must be an error")
	}
	if !IsAuthError(err) {
		t.Fatalf("REQUEST_DENIED must be an auth error, got %v", err)
	}
	var ae *AuthError
	if errors.As(err, &ae); ae.Status != statusRequestDenied {
		t.Errorf("Status = %q, want %q", ae.Status, statusRequestDenied)
	}
}

func TestComputeRouteDistance_403IsAuthError(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusForbidden, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"denied"}}`)

	_, err := c.ComputeRouteDistance(context.Background(), []Coordinates{
		{Latitude: 1, Longitude: 2},
		{Latitude: 3, Longitude: 4},
	})
	if !IsAuthError(err) {
		t.Fatalf("403 must be an auth error, got %v", err)
	}
}

func TestComputeRouteDistance_HappyPath(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"routes":[{"distanceMeters":160934,"duration":"7200s"}]}`)

	got, err := c.ComputeRouteDistance(context.Background(), []Coordinates{
		{Latitude: 1, Longitude: 2},
		{Latitude: 3, Longitude: 4},
	})
	if err != nil {
		t.Fatalf("ComputeRouteDistance: %v", err)
	}
	if got.DistanceMeters != 160934 {
		t.Errorf("DistanceMeters = %d, want 160934", got.DistanceMeters)
	}
	if got.DurationSeconds != 7200 {
		t.Errorf("DurationSeconds = %d, want 7200", got.DurationSeconds)
	}
}

// The waypoint order defines the route, so it has to survive the request build:
// origin first, destination last, everything else intermediate and in sequence.
func TestComputeRouteDistance_SendsWaypointsInOrder(t *testing.T) {
	captureLogs(t)

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"routes":[{"distanceMeters":1,"duration":"1s"}]}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{httpClient: srv.Client(), geocodeHost: srv.URL, routesHost: srv.URL, apiKey: "k"}

	_, err := c.ComputeRouteDistance(context.Background(), []Coordinates{
		{Latitude: 1, Longitude: 1},
		{Latitude: 2, Longitude: 2},
		{Latitude: 3, Longitude: 3},
	})
	if err != nil {
		t.Fatalf("ComputeRouteDistance: %v", err)
	}

	var req computeRoutesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if req.Origin.Location.LatLng.Latitude != 1 {
		t.Errorf("origin lat = %v, want 1", req.Origin.Location.LatLng.Latitude)
	}
	if req.Destination.Location.LatLng.Latitude != 3 {
		t.Errorf("destination lat = %v, want 3", req.Destination.Location.LatLng.Latitude)
	}
	if len(req.Intermediates) != 1 || req.Intermediates[0].Location.LatLng.Latitude != 2 {
		t.Errorf("intermediates = %+v, want exactly the middle waypoint", req.Intermediates)
	}
	if req.TravelMode != "DRIVE" {
		t.Errorf("travelMode = %q, want DRIVE", req.TravelMode)
	}
}

// Too many stops must fail loudly. A silently truncated route returns a plausible
// short distance, which would quietly understate driver pay.
func TestComputeRouteDistance_RejectsTooManyWaypoints(t *testing.T) {
	captureLogs(t)
	c := newTestClient(t, http.StatusOK, `{"routes":[{"distanceMeters":1,"duration":"1s"}]}`)

	wps := make([]Coordinates, maxIntermediates+3) // +2 endpoints, +1 over the cap
	_, err := c.ComputeRouteDistance(context.Background(), wps)
	if err == nil {
		t.Fatal("expected an error for too many intermediate waypoints")
	}
	if !strings.Contains(err.Error(), "exceeds the Routes API limit") {
		t.Errorf("error should name the limit, got %v", err)
	}
}

// tms-auth's classify() matches this sentinel to render "Invalid API key"; an
// *AuthError here would silently degrade it to "Connection failed".
func TestTestConnection_KeepsErrInvalidCredentials(t *testing.T) {
	captureLogs(t)

	t.Run("http 403", func(t *testing.T) {
		c := newTestClient(t, http.StatusForbidden, `{"error":{"code":403}}`)
		if err := c.TestConnection(context.Background()); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("http 200 REQUEST_DENIED", func(t *testing.T) {
		c := newTestClient(t, http.StatusOK, `{"results":[],"status":"REQUEST_DENIED","error_message":"bad key"}`)
		if err := c.TestConnection(context.Background()); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("valid key", func(t *testing.T) {
		c := newTestClient(t, http.StatusOK, geocodeOK)
		if err := c.TestConnection(context.Background()); err != nil {
			t.Fatalf("valid key must pass, got %v", err)
		}
	})
}

// The Geocoding and Places endpoints carry the credential in the query string,
// so the request URL is a secret. Nothing may put it in a log line.
func TestDoGet_NeverLogsAPIKey(t *testing.T) {
	buf := captureLogs(t)
	c := newTestClient(t, http.StatusOK, geocodeOK)

	if _, err := c.Geocode(context.Background(), "anywhere"); err != nil {
		t.Fatalf("Geocode: %v", err)
	}

	logged := buf.String()
	if strings.Contains(logged, "SECRET-KEY-DO-NOT-LOG") {
		t.Fatalf("API key leaked into logs: %s", logged)
	}
	if strings.Contains(logged, "key=") {
		t.Fatalf("query string leaked into logs: %s", logged)
	}
	if !strings.Contains(logged, "google_call") {
		t.Fatalf("expected a google_call billing line, got: %s", logged)
	}
}

func TestNewClientWithToken_RejectsEmptyKey(t *testing.T) {
	if _, err := NewClientWithToken(""); err == nil {
		t.Fatal("an empty key must be rejected, so the provider reports the tenant as unconfigured")
	}
}
