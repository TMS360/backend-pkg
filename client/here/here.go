package here

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

// Route API models
type RouteRequest struct {
	Origin        Coordinates
	Destination   Coordinates
	DepartureTime *time.Time
	TransportMode string // car, truck, pedestrian, bicycle, scooter
	Currency      string
	ReturnOptions []string // tolls, summary, polyline, instructions, etc.
}

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type RouteResponse struct {
	Routes []Route `json:"routes"`
}

type Route struct {
	ID       string         `json:"id"`
	Sections []RouteSection `json:"sections"`
}

type RouteSection struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Departure *LocationInfo  `json:"departure"`
	Arrival   *LocationInfo  `json:"arrival"`
	Summary   *RouteSummary  `json:"summary"`
	Tolls     *TollInfo      `json:"tolls,omitempty"`
	Transport *TransportInfo `json:"transport"`
	Polyline  string         `json:"polyline,omitempty"`
}

type LocationInfo struct {
	Time     string    `json:"time"`
	Place    PlaceInfo `json:"place"`
	Location *Location `json:"location,omitempty"`
}

type PlaceInfo struct {
	Type     string    `json:"type"`
	Location *Location `json:"location"`
}

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type RouteSummary struct {
	Duration     int     `json:"duration"`     // in seconds
	Length       int     `json:"length"`       // in meters
	BaseDuration int     `json:"baseDuration"` // in seconds without traffic
	TollCost     float64 `json:"tollCost,omitempty"`
}

type TollInfo struct {
	EstimatedCost float64      `json:"estimatedCost"`
	Currency      string       `json:"currency"`
	Details       []TollDetail `json:"details,omitempty"`
	Systems       []TollSystem `json:"tollSystems,omitempty"`
}

type TollDetail struct {
	Name     string  `json:"name"`
	Cost     float64 `json:"cost"`
	Currency string  `json:"currency"`
}

type TollSystem struct {
	Name string `json:"name"`
}

type TransportInfo struct {
	Mode string `json:"mode"`
}

// Geocode API models
type GeocodeRequest struct {
	Query    string
	At       *Coordinates // center point for proximity bias
	In       *string      // bounding box or circle
	Limit    int
	Language string
}

type GeocodeResponse struct {
	Items []GeocodeItem `json:"items"`
}

type GeocodeItem struct {
	Title      string       `json:"title"`
	ID         string       `json:"id"`
	ResultType string       `json:"resultType"`
	Address    *AddressInfo `json:"address"`
	Position   *Position    `json:"position"`
	MapView    *MapView     `json:"mapView,omitempty"`
	Categories []Category   `json:"categories,omitempty"`
	Scoring    *Scoring     `json:"scoring,omitempty"`
}

type AddressInfo struct {
	Label       string `json:"label"`
	CountryCode string `json:"countryCode"`
	CountryName string `json:"countryName"`
	State       string `json:"state,omitempty"`
	StateCode   string `json:"stateCode,omitempty"`
	County      string `json:"county,omitempty"`
	City        string `json:"city,omitempty"`
	Street      string `json:"street,omitempty"`
	HouseNumber string `json:"houseNumber,omitempty"`
	PostalCode  string `json:"postalCode,omitempty"`
}

type Position struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type MapView struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

type Category struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Primary bool   `json:"primary,omitempty"`
}

type Scoring struct {
	QueryScore float64 `json:"queryScore"`
	FieldScore struct {
		City        float64 `json:"city,omitempty"`
		Street      float64 `json:"street,omitempty"`
		HouseNumber float64 `json:"houseNumber,omitempty"`
		PostalCode  float64 `json:"postalCode,omitempty"`
	} `json:"fieldScore,omitempty"`
}

// Client struct
type Client struct {
	httpClient  *http.Client
	routerHost  string
	geocodeHost string
	lookupHost  string
	apiKey      string
}

// NewClient creates a new HERE API client
func NewClient(cfg config.HereConfig, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	routerHost := cfg.RouterHost
	if routerHost == "" {
		routerHost = "https://router.hereapi.com"
	}

	geocodeHost := cfg.GeocodeHost
	if geocodeHost == "" {
		geocodeHost = "https://geocode.search.hereapi.com"
	}

	lookupHost := cfg.LookupHost
	if lookupHost == "" {
		lookupHost = "https://lookup.search.hereapi.com"
	}

	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		routerHost:  routerHost,
		geocodeHost: geocodeHost,
		lookupHost:  lookupHost,
		apiKey:      apiKey,
	}, nil
}

// NewClientWithToken creates a new HERE API client with just an API key
func NewClientWithToken(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		routerHost:  "https://router.hereapi.com",
		geocodeHost: "https://geocode.search.hereapi.com",
		lookupHost:  "https://lookup.search.hereapi.com",
		apiKey:      apiKey,
	}, nil
}

// doRequest performs HTTP request
func (c *Client) doRequest(ctx context.Context, method, fullURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("received non-2xx status code: %d (failed to read response body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("received non-2xx status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// GetRoute calculates a route between origin and destination
func (c *Client) GetRoute(ctx context.Context, req RouteRequest) (*RouteResponse, error) {
	params := url.Values{}

	// Required parameters
	params.Set("origin", fmt.Sprintf("%f,%f", req.Origin.Latitude, req.Origin.Longitude))
	params.Set("destination", fmt.Sprintf("%f,%f", req.Destination.Latitude, req.Destination.Longitude))
	params.Set("apiKey", c.apiKey)

	// Transport mode (default to car)
	transportMode := req.TransportMode
	if transportMode == "" {
		transportMode = "car"
	}
	params.Set("transportMode", transportMode)

	// Return options
	if len(req.ReturnOptions) > 0 {
		returnStr := ""
		for i, opt := range req.ReturnOptions {
			if i > 0 {
				returnStr += ","
			}
			returnStr += opt
		}
		params.Set("return", returnStr)
	} else {
		params.Set("return", "tolls,summary")
	}

	// Currency for toll costs
	if req.Currency != "" {
		params.Set("currency", req.Currency)
	}

	// Departure time
	if req.DepartureTime != nil {
		params.Set("departureTime", req.DepartureTime.Format(time.RFC3339))
	}

	fullURL := fmt.Sprintf("%s/v8/routes?%s", c.routerHost, params.Encode())

	resp, err := c.doRequest(ctx, http.MethodGet, fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}
	defer resp.Body.Close()

	var routeResponse RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&routeResponse); err != nil {
		return nil, fmt.Errorf("failed to decode route response: %w", err)
	}

	return &routeResponse, nil
}

// GetTruckRoute calculates a route specifically for trucks
func (c *Client) GetTruckRoute(ctx context.Context, origin, destination Coordinates, departureTime *time.Time) (*RouteResponse, error) {
	req := RouteRequest{
		Origin:        origin,
		Destination:   destination,
		DepartureTime: departureTime,
		TransportMode: "truck",
		Currency:      "USD",
		ReturnOptions: []string{"tolls", "summary", "polyline"},
	}

	return c.GetRoute(ctx, req)
}

// Geocode converts an address to coordinates
func (c *Client) Geocode(ctx context.Context, req GeocodeRequest) (*GeocodeResponse, error) {
	params := url.Values{}

	// Required parameters
	params.Set("q", req.Query)
	params.Set("apiKey", c.apiKey)

	// Optional parameters
	if req.At != nil {
		params.Set("at", fmt.Sprintf("%f,%f", req.At.Latitude, req.At.Longitude))
	}

	if req.In != nil {
		params.Set("in", *req.In)
	}

	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}

	if req.Language != "" {
		params.Set("lang", req.Language)
	}

	fullURL := fmt.Sprintf("%s/v1/geocode?%s", c.geocodeHost, params.Encode())

	resp, err := c.doRequest(ctx, http.MethodGet, fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to geocode: %w", err)
	}
	defer resp.Body.Close()

	var geocodeResponse GeocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&geocodeResponse); err != nil {
		return nil, fmt.Errorf("failed to decode geocode response: %w", err)
	}

	return &geocodeResponse, nil
}

// GeocodeAddress is a simple helper to geocode an address string
func (c *Client) GeocodeAddress(ctx context.Context, address string) (*GeocodeItem, error) {
	req := GeocodeRequest{
		Query: address,
		Limit: 1,
	}

	resp, err := c.Geocode(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("no results found for address: %s", address)
	}

	return &resp.Items[0], nil
}

// ReverseGeocode converts coordinates to an address
func (c *Client) ReverseGeocode(ctx context.Context, lat, lng float64) (*GeocodeItem, error) {
	params := url.Values{}
	params.Set("at", fmt.Sprintf("%f,%f", lat, lng))
	params.Set("apiKey", c.apiKey)
	params.Set("limit", "1")

	fullURL := fmt.Sprintf("%s/v1/revgeocode?%s", c.geocodeHost, params.Encode())

	resp, err := c.doRequest(ctx, http.MethodGet, fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to reverse geocode: %w", err)
	}
	defer resp.Body.Close()

	var geocodeResponse GeocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&geocodeResponse); err != nil {
		return nil, fmt.Errorf("failed to decode reverse geocode response: %w", err)
	}

	if len(geocodeResponse.Items) == 0 {
		return nil, fmt.Errorf("no results found for coordinates: %f, %f", lat, lng)
	}

	return &geocodeResponse.Items[0], nil
}

// CalculateDistance calculates the distance and duration between two points
func (c *Client) CalculateDistance(ctx context.Context, origin, destination Coordinates) (distanceMeters int, durationSeconds int, err error) {
	req := RouteRequest{
		Origin:        origin,
		Destination:   destination,
		TransportMode: "car",
		ReturnOptions: []string{"summary"},
	}

	routeResp, err := c.GetRoute(ctx, req)
	if err != nil {
		return 0, 0, err
	}

	if len(routeResp.Routes) == 0 || len(routeResp.Routes[0].Sections) == 0 {
		return 0, 0, fmt.Errorf("no route found")
	}

	summary := routeResp.Routes[0].Sections[0].Summary
	if summary == nil {
		return 0, 0, fmt.Errorf("no summary information in route")
	}

	return summary.Length, summary.Duration, nil
}

// LookupRequest contains parameters for the Lookup API
type LookupRequest struct {
	ID       string // Required: HERE ID of the place to look up
	Language string // Optional: language for the response
}

// Lookup retrieves detailed information about a place by its HERE ID
func (c *Client) Lookup(ctx context.Context, req LookupRequest) (*GeocodeItem, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("ID cannot be empty")
	}

	params := url.Values{}
	params.Set("id", req.ID)
	params.Set("apiKey", c.apiKey)

	if req.Language != "" {
		params.Set("lang", req.Language)
	}

	fullURL := fmt.Sprintf("%s/v1/lookup?%s", c.lookupHost, params.Encode())

	resp, err := c.doRequest(ctx, http.MethodGet, fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup: %w", err)
	}
	defer resp.Body.Close()

	var item GeocodeItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode lookup response: %w", err)
	}

	return &item, nil
}

// LookupByID is a simple helper to lookup a place by its HERE ID
func (c *Client) LookupByID(ctx context.Context, id string) (*GeocodeItem, error) {
	return c.Lookup(ctx, LookupRequest{ID: id})
}
