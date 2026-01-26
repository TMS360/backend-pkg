package samsara

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// VehicleLocationData represents current vehicle location from Samsara
// This struct is designed to be reusable across different services
type VehicleLocationData struct {
	// Coordinates
	Latitude  float64
	Longitude float64
	// Location metadata
	Heading   float64   // Direction in degrees
	Speed     float64   // Speed in miles per hour
	Timestamp time.Time // When the location was recorded
	// Samsara identifiers
	SamsaraID   string
	VehicleName string
	VIN         string
}

// Service provides Samsara API operations for vehicle tracking
// This service is designed to be reusable across different microservices
type Service interface {
	// GetVehicleLocationByVIN gets the current location of a vehicle by its VIN
	GetVehicleLocationByVIN(ctx context.Context, vin string) (*VehicleLocationData, error)

	// GetVehicleLocationBySamsaraID gets the current location of a vehicle by its Samsara ID
	GetVehicleLocationBySamsaraID(ctx context.Context, samsaraID int64) (*VehicleLocationData, error)

	// GetAllVehiclesLocations gets current locations of all vehicles
	GetAllVehiclesLocations(ctx context.Context) ([]VehicleLocationData, error)
}

type service struct {
	client *Client
}

// NewService creates a new Samsara Service instance
func NewService(client *Client) Service {
	return &service{client: client}
}

// GetVehicleLocationByVIN gets the current location of a vehicle by its VIN
func (s *service) GetVehicleLocationByVIN(ctx context.Context, vin string) (*VehicleLocationData, error) {
	if s.client == nil {
		return nil, fmt.Errorf("samsara client is not configured")
	}

	if vin == "" {
		return nil, fmt.Errorf("VIN cannot be empty")
	}

	// Step 1: Find vehicle by VIN to get Samsara ID
	vehicle, err := s.client.GetVehicleByVIN(ctx, vin)
	if err != nil {
		return nil, fmt.Errorf("failed to find vehicle by VIN %s: %w", vin, err)
	}

	if vehicle == nil {
		return nil, fmt.Errorf("no vehicle found for VIN: %s", vin)
	}

	// Step 2: Parse Samsara ID
	samsaraID, err := strconv.ParseInt(vehicle.ID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid samsara vehicle ID format: %s", vehicle.ID)
	}

	// Step 3: Get vehicle coordinates
	location, err := s.client.GetVehicleCoordinates(ctx, samsaraID)
	if err != nil {
		return nil, fmt.Errorf("failed to get coordinates for vehicle %s (samsara ID: %d): %w", vin, samsaraID, err)
	}

	if location == nil || location.Gps == nil {
		return nil, fmt.Errorf("no GPS data available for vehicle: %s", vin)
	}

	// Parse timestamp
	var timestamp time.Time
	if location.Gps.Time != "" {
		timestamp, _ = time.Parse(time.RFC3339, location.Gps.Time)
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return &VehicleLocationData{
		Latitude:    location.Gps.Latitude,
		Longitude:   location.Gps.Longitude,
		Heading:     location.Gps.Heading,
		Speed:       location.Gps.Speed,
		Timestamp:   timestamp,
		SamsaraID:   vehicle.ID,
		VehicleName: vehicle.Name,
		VIN:         vehicle.Vin,
	}, nil
}

// GetVehicleLocationBySamsaraID gets the current location of a vehicle by its Samsara ID
func (s *service) GetVehicleLocationBySamsaraID(ctx context.Context, samsaraID int64) (*VehicleLocationData, error) {
	if s.client == nil {
		return nil, fmt.Errorf("samsara client is not configured")
	}

	location, err := s.client.GetVehicleCoordinates(ctx, samsaraID)
	if err != nil {
		return nil, fmt.Errorf("failed to get coordinates for samsara ID %d: %w", samsaraID, err)
	}

	if location == nil || location.Gps == nil {
		return nil, fmt.Errorf("no GPS data available for samsara ID: %d", samsaraID)
	}

	// Parse timestamp
	var timestamp time.Time
	if location.Gps.Time != "" {
		timestamp, _ = time.Parse(time.RFC3339, location.Gps.Time)
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return &VehicleLocationData{
		Latitude:    location.Gps.Latitude,
		Longitude:   location.Gps.Longitude,
		Heading:     location.Gps.Heading,
		Speed:       location.Gps.Speed,
		Timestamp:   timestamp,
		SamsaraID:   location.ID,
		VehicleName: location.Name,
		VIN:         location.Vin,
	}, nil
}

// GetAllVehiclesLocations gets current locations of all vehicles
func (s *service) GetAllVehiclesLocations(ctx context.Context) ([]VehicleLocationData, error) {
	if s.client == nil {
		return nil, fmt.Errorf("samsara client is not configured")
	}

	locations, err := s.client.GetAllVehiclesLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all vehicles locations: %w", err)
	}

	result := make([]VehicleLocationData, 0, len(locations))
	for _, loc := range locations {
		if loc.Gps == nil {
			continue
		}

		// Parse timestamp
		var timestamp time.Time
		if loc.Gps.Time != "" {
			timestamp, _ = time.Parse(time.RFC3339, loc.Gps.Time)
		}
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		result = append(result, VehicleLocationData{
			Latitude:    loc.Gps.Latitude,
			Longitude:   loc.Gps.Longitude,
			Heading:     loc.Gps.Heading,
			Speed:       loc.Gps.Speed,
			Timestamp:   timestamp,
			SamsaraID:   loc.ID,
			VehicleName: loc.Name,
			VIN:         loc.Vin,
		})
	}

	return result, nil
}
