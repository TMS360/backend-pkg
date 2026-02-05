package here

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// AddressData represents resolved address data from HERE API
// This struct is designed to be reusable across different services
type AddressData struct {
	// Full formatted address label
	Location string
	// Coordinates
	Latitude  float64
	Longitude float64
	// Address components
	CountryCode *string
	CountryName *string
	StateCode   *string
	State       *string
	County      *string
	City        *string
	District    *string
	Street      *string
	PostalCode  *string
	HouseNumber *string
	// Original HERE ID for reference
	HereID string
}

// RouteInfo represents route information between two points
type RouteInfo struct {
	// Distance in meters
	DistanceMeters int
	// Duration in seconds (without traffic)
	DurationSeconds int
	// Duration with traffic in seconds
	DurationWithTrafficSeconds int
	// Estimated arrival time
	EstimatedArrival time.Time
	// Toll cost if available
	TollCost *float64
	// Toll currency
	TollCurrency *string
}

// MultiStopRouteInfo represents route information for a multi-stop trip
type MultiStopRouteInfo struct {
	// Total distance for entire route in meters
	TotalDistanceMeters int
	// Total duration in seconds
	TotalDurationSeconds int
	// Per-leg information
	Legs []RouteLegInfo
	// Total toll cost
	TotalTollCost *float64
}

// RouteLegInfo represents one leg of a multi-stop route
type RouteLegInfo struct {
	// Leg index (0-based)
	Index int
	// Origin coordinates
	Origin Coordinates
	// Destination coordinates
	Destination Coordinates
	// Distance in meters
	DistanceMeters int
	// Duration in seconds
	DurationSeconds int
	// Estimated arrival at this stop
	EstimatedArrival time.Time
}

// Service provides HERE Maps API operations
// This service is designed to be reusable across different microservices
type Service interface {
	// LookupByID resolves a HERE ID to full address data
	LookupByID(ctx context.Context, hereID string) (*AddressData, error)

	// CalculateRoute calculates route between origin and destination
	CalculateRoute(ctx context.Context, origin, destination Coordinates, departureTime *time.Time) (*RouteInfo, error)

	// CalculateTruckRoute calculates route specifically for trucks
	CalculateTruckRoute(ctx context.Context, origin, destination Coordinates, departureTime *time.Time) (*RouteInfo, error)

	// GetETA calculates estimated time of arrival from origin to destination
	GetETA(ctx context.Context, origin, destination Coordinates) (time.Time, error)

	// GetDistance calculates distance in meters between two points
	GetDistance(ctx context.Context, origin, destination Coordinates) (int, error)

	// CalculateMultiStopRoute calculates route through multiple waypoints
	CalculateMultiStopRoute(ctx context.Context, waypoints []Coordinates, departureTime *time.Time) (*MultiStopRouteInfo, error)
}

type service struct {
	client *Client
}

// NewService creates a new HERE Service instance
func NewService(client *Client) Service {
	return &service{client: client}
}

// LookupByID resolves a HERE ID to full address data
func (s *service) LookupByID(ctx context.Context, hereID string) (*AddressData, error) {
	if s.client == nil {
		return nil, fmt.Errorf("HERE client is not configured")
	}

	if hereID == "" {
		return nil, fmt.Errorf("here_id cannot be empty")
	}

	item, err := s.client.LookupByID(ctx, hereID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup HERE ID %s: %w", hereID, err)
	}

	if item == nil {
		return nil, fmt.Errorf("no result found for HERE ID: %s", hereID)
	}

	data := &AddressData{
		HereID:   hereID,
		Location: item.Title,
	}

	// Set coordinates if available
	if item.Position != nil {
		data.Latitude = item.Position.Lat
		data.Longitude = item.Position.Lng
	}

	// Set address components if available
	if item.Address != nil {
		if item.Address.CountryCode != "" {
			data.CountryCode = &item.Address.CountryCode
		}
		if item.Address.CountryName != "" {
			data.CountryName = &item.Address.CountryName
		}
		if item.Address.StateCode != "" {
			data.StateCode = &item.Address.StateCode
		}
		if item.Address.State != "" {
			data.State = &item.Address.State
		}
		if item.Address.County != "" {
			data.County = &item.Address.County
		}
		if item.Address.City != "" {
			data.City = &item.Address.City
		}
		if item.Address.Street != "" {
			data.Street = &item.Address.Street
		}
		if item.Address.PostalCode != "" {
			data.PostalCode = &item.Address.PostalCode
		}
		if item.Address.HouseNumber != "" {
			data.HouseNumber = &item.Address.HouseNumber
		}
		// Use label as location if title is empty
		if data.Location == "" && item.Address.Label != "" {
			data.Location = item.Address.Label
		}
	}

	return data, nil
}

// CalculateRoute calculates route between origin and destination
func (s *service) CalculateRoute(ctx context.Context, origin, destination Coordinates, departureTime *time.Time) (*RouteInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("HERE client is not configured")
	}

	req := RouteRequest{
		Origin:        origin,
		Destination:   destination,
		DepartureTime: departureTime,
		TransportMode: "car",
		Currency:      "USD",
		ReturnOptions: []string{"tolls", "summary"},
	}

	resp, err := s.client.GetRoute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate route: %w", err)
	}

	if len(resp.Routes) == 0 || len(resp.Routes[0].Sections) == 0 {
		return nil, fmt.Errorf("no route found")
	}

	return s.parseRouteResponse(resp, departureTime), nil
}

// CalculateTruckRoute calculates route specifically for trucks
func (s *service) CalculateTruckRoute(ctx context.Context, origin, destination Coordinates, departureTime *time.Time) (*RouteInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("HERE client is not configured")
	}

	resp, err := s.client.GetTruckRoute(ctx, origin, destination, departureTime)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate truck route: %w", err)
	}

	if len(resp.Routes) == 0 || len(resp.Routes[0].Sections) == 0 {
		return nil, fmt.Errorf("no route found")
	}

	return s.parseRouteResponse(resp, departureTime), nil
}

// GetETA calculates estimated time of arrival from origin to destination
func (s *service) GetETA(ctx context.Context, origin, destination Coordinates) (time.Time, error) {
	now := time.Now()
	routeInfo, err := s.CalculateTruckRoute(ctx, origin, destination, &now)
	if err != nil {
		return time.Time{}, err
	}
	return routeInfo.EstimatedArrival, nil
}

// GetDistance calculates distance in meters between two points
func (s *service) GetDistance(ctx context.Context, origin, destination Coordinates) (int, error) {
	if s.client == nil {
		return 0, fmt.Errorf("HERE client is not configured")
	}

	distanceMeters, _, err := s.client.CalculateDistance(ctx, origin, destination)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate distance: %w", err)
	}

	return distanceMeters, nil
}

// CalculateMultiStopRoute calculates route through multiple waypoints
func (s *service) CalculateMultiStopRoute(ctx context.Context, waypoints []Coordinates, departureTime *time.Time) (*MultiStopRouteInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("HERE client is not configured")
	}

	if len(waypoints) < 2 {
		return nil, fmt.Errorf("at least 2 waypoints required")
	}

	result := &MultiStopRouteInfo{
		Legs: make([]RouteLegInfo, 0, len(waypoints)-1),
	}

	currentDeparture := departureTime
	if currentDeparture == nil {
		now := time.Now()
		currentDeparture = &now
	}

	var totalTollCost float64
	hasTolls := false

	for i := 0; i < len(waypoints)-1; i++ {
		origin := waypoints[i]
		dest := waypoints[i+1]

		routeInfo, err := s.CalculateTruckRoute(ctx, origin, dest, currentDeparture)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate route for leg %d: %w", i, err)
		}

		leg := RouteLegInfo{
			Index:            i,
			Origin:           origin,
			Destination:      dest,
			DistanceMeters:   routeInfo.DistanceMeters,
			DurationSeconds:  routeInfo.DurationSeconds,
			EstimatedArrival: routeInfo.EstimatedArrival,
		}

		result.Legs = append(result.Legs, leg)
		result.TotalDistanceMeters += routeInfo.DistanceMeters
		result.TotalDurationSeconds += routeInfo.DurationSeconds

		if routeInfo.TollCost != nil {
			totalTollCost += *routeInfo.TollCost
			hasTolls = true
		}

		// Next leg starts when this one arrives
		currentDeparture = &routeInfo.EstimatedArrival
	}

	if hasTolls {
		result.TotalTollCost = &totalTollCost
	}

	return result, nil
}

// parseRouteResponse extracts RouteInfo from HERE API response
func (s *service) parseRouteResponse(resp *RouteResponse, departureTime *time.Time) *RouteInfo {
	section := resp.Routes[0].Sections[0]

	info := &RouteInfo{}

	if section.Summary != nil {
		info.DistanceMeters = section.Summary.Length
		info.DurationSeconds = section.Summary.BaseDuration
		info.DurationWithTrafficSeconds = section.Summary.Duration
	}

	// Calculate ETA
	departure := time.Now()
	if departureTime != nil {
		departure = *departureTime
	}
	info.EstimatedArrival = departure.Add(time.Duration(info.DurationWithTrafficSeconds) * time.Second)

	// Extract toll information from the tolls array
	if len(section.Tolls) > 0 {
		var totalCost float64
		var currency string
		for _, toll := range section.Tolls {
			for _, fare := range toll.Fares {
				// Prefer convertedPrice (requested currency) over local price
				price := fare.Price
				if fare.ConvertedPrice != nil {
					price = *fare.ConvertedPrice
				}
				if val, err := strconv.ParseFloat(price.Value, 64); err == nil {
					totalCost += val
				}
				if currency == "" {
					currency = price.Currency
				}
			}
		}
		info.TollCost = &totalCost
		info.TollCurrency = &currency
	}

	return info
}
