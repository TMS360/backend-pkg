// Package googlemaps is a thin server-side client for the Google Maps Platform
// APIs that TMS360 uses as a fallback when HERE is unavailable: Geocoding for
// stop resolution, Places Autocomplete for address search, and Routes v2 for
// multi-stop distance.
//
// It deliberately mirrors client/here — same constructor shape, same AuthError /
// IsAuthError / ErrInvalidCredentials contract — so the per-company
// provider.ClientProvider wiring and the tms-auth "Test connection" classifier
// treat both vendors identically.
//
// One thing does NOT mirror HERE and is the main trap here: the classic
// Geocoding and Places endpoints answer an invalid key with HTTP 200 and a
// "status": "REQUEST_DENIED" body, not a 401/403. Auth detection therefore has
// to read the payload, not just the status line. Routes v2 is the modern API and
// does return a real 403.
package googlemaps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

// ErrInvalidCredentials is returned by TestConnection when Google rejects the
// configured API key. tms-auth's integration classifier matches this sentinel
// with errors.Is to render "Invalid API key"; an *AuthError there would degrade
// to the generic "Connection failed" (same contract as here.ErrInvalidCredentials).
var ErrInvalidCredentials = errors.New("googlemaps: invalid credentials")

// Billed Google endpoints, as reported in the "op" attribute of google_call.
const (
	opGeocode      = "geocode"
	opAutocomplete = "places_autocomplete"
	opComputeRoute = "compute_routes"
	// opTestConn is the credential probe behind the Settings "Test connection"
	// button. It bills like any other geocode, so it is counted separately rather
	// than folded into opGeocode, which would misattribute it to stop resolution.
	opTestConn = "geocode_testconn"
)

// Google's REST status strings that mean "your key is not usable", as opposed to
// "your query found nothing". OVER_QUERY_LIMIT is included because a key over its
// billing cap is unusable until someone changes the account, which is an
// operator problem of exactly the same shape as a rejected key.
const (
	statusOK            = "OK"
	statusZeroResults   = "ZERO_RESULTS"
	statusRequestDenied = "REQUEST_DENIED"
	statusOverLimit     = "OVER_QUERY_LIMIT"
)

// AuthError is returned when Google rejects the credential — either with a real
// 401/403 (Routes v2) or with a 200 carrying REQUEST_DENIED / OVER_QUERY_LIMIT
// (Geocoding, Places). Callers use IsAuthError to stop retrying and to flag the
// tenant's integration.
type AuthError struct {
	// StatusCode is the HTTP status. It is 200 for the REST APIs that report
	// credential failures in the body.
	StatusCode int
	// Status is Google's own status string when present (REQUEST_DENIED, …).
	Status string
	Body   string
}

func (e *AuthError) Error() string {
	if e.Status != "" {
		return fmt.Sprintf("googlemaps auth failed (status %d, %s): %s", e.StatusCode, e.Status, e.Body)
	}
	return fmt.Sprintf("googlemaps auth failed (status %d): %s", e.StatusCode, e.Body)
}

// IsAuthError reports whether err (or any error it wraps) is a *AuthError.
func IsAuthError(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// Client talks to the Google Maps Platform APIs for one company's key.
type Client struct {
	httpClient  *http.Client
	geocodeHost string
	routesHost  string
	apiKey      string
}

// NewClient builds a client with overridable hosts, so tests can point it at a
// local stub. Empty hosts fall back to the public endpoints.
func NewClient(cfg config.GoogleMapsConfig, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	geocodeHost := cfg.GeocodeHost
	if geocodeHost == "" {
		geocodeHost = "https://maps.googleapis.com"
	}

	routesHost := cfg.RoutesHost
	if routesHost == "" {
		routesHost = "https://routes.googleapis.com"
	}

	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		geocodeHost: geocodeHost,
		routesHost:  routesHost,
		apiKey:      apiKey,
	}, nil
}

// NewClientWithToken builds a client against the public endpoints. This is the
// form every DI site uses (mirrors here.NewClientWithToken).
func NewClientWithToken(apiKey string) (*Client, error) {
	return NewClient(config.GoogleMapsConfig{}, apiKey)
}

// Geocoding / Places models.

type GeocodeResult struct {
	FormattedAddress string             `json:"formatted_address"`
	PlaceID          string             `json:"place_id"`
	Types            []string           `json:"types"`
	Geometry         Geometry           `json:"geometry"`
	AddressComponent []AddressComponent `json:"address_components"`
}

type Geometry struct {
	Location     LatLng `json:"location"`
	LocationType string `json:"location_type"`
}

type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// AddressComponent is one structured piece of a Google address. Types is the
// discriminator ("street_number", "route", "locality", …); see ComponentByType.
type AddressComponent struct {
	LongName  string   `json:"long_name"`
	ShortName string   `json:"short_name"`
	Types     []string `json:"types"`
}

type geocodeResponse struct {
	Results      []GeocodeResult `json:"results"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message"`
}

// Prediction is one Places Autocomplete suggestion.
type Prediction struct {
	Description string `json:"description"`
	PlaceID     string `json:"place_id"`
}

type autocompleteResponse struct {
	Predictions  []Prediction `json:"predictions"`
	Status       string       `json:"status"`
	ErrorMessage string       `json:"error_message"`
}

// Coordinates is a waypoint for ComputeRouteDistance. It mirrors
// here.Coordinates field-for-field so call sites can convert without a lookup
// table; the two packages stay decoupled (no import between vendors).
type Coordinates struct {
	Latitude  float64
	Longitude float64
}

// RouteDistance is the outcome of a multi-stop route calculation.
type RouteDistance struct {
	DistanceMeters int
	// DurationSeconds is 0 when Google omits the duration from the field mask.
	DurationSeconds int
}

// Geocode resolves a free-form address. A query that matches nothing returns an
// empty slice and no error — "not found" is not a failure, and callers must be
// able to tell it apart from an outage so they do not retry it.
func (c *Client) Geocode(ctx context.Context, query string) ([]GeocodeResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("googlemaps: empty geocode query")
	}

	params := url.Values{}
	params.Set("address", query)
	params.Set("key", c.apiKey)

	body, err := c.doGet(ctx, opGeocode, c.geocodeHost+"/maps/api/geocode/json?"+params.Encode())
	if err != nil {
		return nil, err
	}

	var out geocodeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("googlemaps: failed to decode geocode response: %w", err)
	}
	if err := statusError(out.Status, out.ErrorMessage, string(body)); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// PlacesAutocomplete returns address suggestions for a partial input. It exists
// so the browser never needs a Google key — address search stays proxied through
// the backend (a locked product rule of the fallback program).
func (c *Client) PlacesAutocomplete(ctx context.Context, input string) ([]Prediction, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("googlemaps: empty autocomplete input")
	}

	params := url.Values{}
	params.Set("input", input)
	params.Set("key", c.apiKey)

	body, err := c.doGet(ctx, opAutocomplete, c.geocodeHost+"/maps/api/place/autocomplete/json?"+params.Encode())
	if err != nil {
		return nil, err
	}

	var out autocompleteResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("googlemaps: failed to decode autocomplete response: %w", err)
	}
	if err := statusError(out.Status, out.ErrorMessage, string(body)); err != nil {
		return nil, err
	}
	return out.Predictions, nil
}

// routes v2 request/response shapes. Only the fields we ask for in the field
// mask are modelled.

type routesLatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type routesLocation struct {
	LatLng routesLatLng `json:"latLng"`
}

type routesWaypoint struct {
	Location routesLocation `json:"location"`
}

type computeRoutesRequest struct {
	Origin        routesWaypoint   `json:"origin"`
	Destination   routesWaypoint   `json:"destination"`
	Intermediates []routesWaypoint `json:"intermediates,omitempty"`
	TravelMode    string           `json:"travelMode"`
}

type computeRoutesResponse struct {
	Routes []struct {
		DistanceMeters int    `json:"distanceMeters"`
		Duration       string `json:"duration"` // e.g. "1234s"
	} `json:"routes"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// maxIntermediates is the Routes v2 cap on intermediate waypoints. A trip with
// more stops than this is rejected up front rather than silently truncated —
// a short route would read as a real number and quietly understate driver pay.
const maxIntermediates = 25

// ComputeRouteDistance returns the driving distance over waypoints in order.
// Callers convert metres to miles at their own call site (backend-load owns the
// metersPerMile constant).
func (c *Client) ComputeRouteDistance(ctx context.Context, waypoints []Coordinates) (*RouteDistance, error) {
	if len(waypoints) < 2 {
		return nil, fmt.Errorf("googlemaps: need at least two waypoints, got %d", len(waypoints))
	}
	if n := len(waypoints) - 2; n > maxIntermediates {
		return nil, fmt.Errorf("googlemaps: %d intermediate waypoints exceeds the Routes API limit of %d", n, maxIntermediates)
	}

	req := computeRoutesRequest{
		Origin:      toWaypoint(waypoints[0]),
		Destination: toWaypoint(waypoints[len(waypoints)-1]),
		TravelMode:  "DRIVE",
	}
	for _, wp := range waypoints[1 : len(waypoints)-1] {
		req.Intermediates = append(req.Intermediates, toWaypoint(wp))
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("googlemaps: failed to encode routes request: %w", err)
	}

	body, err := c.doPost(ctx, opComputeRoute, c.routesHost+"/directions/v2:computeRoutes", payload)
	if err != nil {
		return nil, err
	}

	var out computeRoutesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("googlemaps: failed to decode routes response: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("googlemaps: routes error %s: %s", out.Error.Status, out.Error.Message)
	}
	if len(out.Routes) == 0 {
		return nil, fmt.Errorf("googlemaps: no route between the given waypoints")
	}

	return &RouteDistance{
		DistanceMeters:  out.Routes[0].DistanceMeters,
		DurationSeconds: parseDurationSeconds(out.Routes[0].Duration),
	}, nil
}

// TestConnection probes the credential for the Settings "Test connection"
// button. It deliberately does not surface *AuthError: callers match on
// ErrInvalidCredentials (tms-auth's integration classify), mirroring
// here.TestConnection.
func (c *Client) TestConnection(ctx context.Context) error {
	params := url.Values{}
	params.Set("address", "1600 Amphitheatre Parkway, Mountain View, CA")
	params.Set("key", c.apiKey)

	body, err := c.doGet(ctx, opTestConn, c.geocodeHost+"/maps/api/geocode/json?"+params.Encode())
	if err != nil {
		if IsAuthError(err) {
			return ErrInvalidCredentials
		}
		return err
	}

	var out geocodeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("googlemaps: failed to decode test response: %w", err)
	}
	if err := statusError(out.Status, out.ErrorMessage, string(body)); err != nil {
		if IsAuthError(err) {
			return ErrInvalidCredentials
		}
		return err
	}
	return nil
}

// doGet performs a GET whose URL carries the API key, and returns the raw body.
//
// op names the endpoint being billed and is passed in rather than parsed back
// out of fullURL, which carries the key and must never be logged.
func (c *Client) doGet(ctx context.Context, op, fullURL string) ([]byte, error) {
	started := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return c.finish(ctx, op, started, req)
}

// doPost performs the Routes v2 call. Routes takes the credential in a header
// rather than the query string, and answers a bad key with a real 403.
func (c *Client) doPost(ctx context.Context, op, fullURL string, payload []byte) ([]byte, error) {
	started := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	// Ask only for what we use; Routes bills by the fields requested.
	req.Header.Set("X-Goog-FieldMask", "routes.distanceMeters,routes.duration")

	return c.finish(ctx, op, started, req)
}

func (c *Client) finish(ctx context.Context, op string, started time.Time, req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Never reached Google, so nothing was billed — logged anyway so a
		// network-level outage is visible in the same sample.
		logCall(ctx, op, 0, started, err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		authErr := &AuthError{StatusCode: resp.StatusCode, Body: string(body)}
		logCall(ctx, op, resp.StatusCode, started, authErr)
		return nil, authErr
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusErr := fmt.Errorf("received non-2xx status code: %d, body: %s", resp.StatusCode, string(body))
		logCall(ctx, op, resp.StatusCode, started, statusErr)
		return nil, statusErr
	}

	if readErr != nil {
		wrapped := fmt.Errorf("failed to read response body: %w", readErr)
		logCall(ctx, op, resp.StatusCode, started, wrapped)
		return nil, wrapped
	}

	logCall(ctx, op, resp.StatusCode, started, nil)
	return body, nil
}

// statusError turns Google's REST status string into an error. ZERO_RESULTS is
// deliberately NOT an error: "no match" is an answer, and a caller that retried
// it would burn quota re-asking a question already answered.
func statusError(status, errorMessage, body string) error {
	switch status {
	case statusOK, statusZeroResults, "":
		return nil
	case statusRequestDenied, statusOverLimit:
		return &AuthError{StatusCode: http.StatusOK, Status: status, Body: pickMessage(errorMessage, body)}
	default:
		return fmt.Errorf("googlemaps: %s: %s", status, pickMessage(errorMessage, body))
	}
}

func pickMessage(errorMessage, body string) string {
	if errorMessage != "" {
		return errorMessage
	}
	return body
}

func toWaypoint(c Coordinates) routesWaypoint {
	return routesWaypoint{Location: routesLocation{LatLng: routesLatLng{
		Latitude:  c.Latitude,
		Longitude: c.Longitude,
	}}}
}

// parseDurationSeconds reads Routes v2's protobuf-style duration ("1234s").
// An unparseable value yields 0 rather than an error — duration is incidental
// to the distance the caller actually asked for.
func parseDurationSeconds(d string) int {
	d = strings.TrimSuffix(d, "s")
	if d == "" {
		return 0
	}
	var secs int
	if _, err := fmt.Sscanf(d, "%d", &secs); err != nil {
		return 0
	}
	return secs
}

// ComponentByType returns the first address component carrying typ (e.g.
// "locality", "postal_code"), or "" when the result has none. Google returns a
// flat, unordered component list rather than named fields, so every consumer
// needs this lookup.
func ComponentByType(r GeocodeResult, typ string) string {
	for _, comp := range r.AddressComponent {
		for _, t := range comp.Types {
			if t == typ {
				return comp.LongName
			}
		}
	}
	return ""
}

// ShortComponentByType is ComponentByType but prefers the abbreviated form,
// which is what US state and country codes need ("IL", not "Illinois").
func ShortComponentByType(r GeocodeResult, typ string) string {
	for _, comp := range r.AddressComponent {
		for _, t := range comp.Types {
			if t == typ {
				return comp.ShortName
			}
		}
	}
	return ""
}
