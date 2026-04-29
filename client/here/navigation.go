package here

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrNoTruckRoute is returned when HERE cannot build a route for the given
// truck/avoid constraints (typically a 400 from the API or empty result).
var ErrNoTruckRoute = errors.New("here: no truck route found for given constraints")

// TruckNavRequest is the high-level input for truck turn-by-turn navigation.
type TruckNavRequest struct {
	Origin        Coordinates
	Destination   Coordinates
	DepartureTime *time.Time
	Truck         TruckAttributes
	Avoid         *AvoidOptions
	Lang          string // default "en-US"
	Currency      string // default "USD"
	Alternatives  int
}

// TruckNavMultiStopRequest is the multi-stop variant; all stops in one HERE call.
type TruckNavMultiStopRequest struct {
	Stops         []Coordinates // [origin, ...via, destination], len >= 2
	DepartureTime *time.Time
	Truck         TruckAttributes
	Avoid         *AvoidOptions
	Lang          string
	Currency      string
	Alternatives  int
}

// TruckNavResult is a parsed, navigation-ready route.
type TruckNavResult struct {
	RouteID                    string
	DistanceMeters             int
	DurationSeconds            int // with traffic
	BaseDurationSeconds        int // without traffic
	EstimatedArrival           time.Time
	PolylineEncoded            string // raw HERE flexible polyline (first section)
	PolylineDecoded            []DecodedCoordinate
	Steps                      []NavStep
	Tolls                      []TollItem
	TotalTollCost              *float64
	TollCurrency               *string
	Notices                    []RouteNotice
	Alternatives               []*TruckNavResult `json:",omitempty"`
}

// NavStep is one driver-facing maneuver, ready to render in a UI.
type NavStep struct {
	Index       int
	Action      string
	Direction   string
	Instruction string
	DistanceM   int
	DurationSec int
	RoadName    string
	PolyStart   int // index in TruckNavResult.PolylineDecoded
	PolyEnd     int
}

// NavigationService is the high-level truck navigation API.
type NavigationService interface {
	BuildTruckRoute(ctx context.Context, req TruckNavRequest) (*TruckNavResult, error)
	BuildTruckRouteMultiStop(ctx context.Context, req TruckNavMultiStopRequest) (*TruckNavResult, error)
	RerouteFrom(
		ctx context.Context,
		currentPos Coordinates,
		remainingStops []Coordinates,
		truck TruckAttributes,
		departureTime *time.Time,
	) (*TruckNavResult, error)
}

type navigationService struct {
	client *Client
}

// NewNavigationService creates a NavigationService backed by the given HERE client.
// It is independent of the legacy Service interface — both can coexist.
func NewNavigationService(client *Client) NavigationService {
	return &navigationService{client: client}
}

// navReturnOptions is the fixed set of HERE `return=` fields needed for navigation.
var navReturnOptions = []string{
	"polyline",
	"summary",
	"actions",
	"instructions",
	"tolls",
	"spans",
	"notices",
	"travelSummary",
}

func (s *navigationService) BuildTruckRoute(ctx context.Context, req TruckNavRequest) (*TruckNavResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("here: navigation client is not configured")
	}

	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}
	lang := req.Lang
	if lang == "" {
		lang = "en-US"
	}

	rr := RouteRequest{
		Origin:        req.Origin,
		Destination:   req.Destination,
		DepartureTime: req.DepartureTime,
		TransportMode: "truck",
		Currency:      currency,
		ReturnOptions: navReturnOptions,
		TruckAttrs:    cloneTruck(req.Truck),
		Avoid:         req.Avoid,
		Alternatives:  req.Alternatives,
		Lang:          lang,
	}

	resp, err := s.client.GetRoute(ctx, rr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoTruckRoute, err)
	}
	if len(resp.Routes) == 0 || len(resp.Routes[0].Sections) == 0 {
		return nil, ErrNoTruckRoute
	}

	primary := buildResultFromRoute(resp.Routes[0], req.DepartureTime)

	// Map alternatives, if any.
	for i := 1; i < len(resp.Routes); i++ {
		alt := buildResultFromRoute(resp.Routes[i], req.DepartureTime)
		primary.Alternatives = append(primary.Alternatives, alt)
	}

	return primary, nil
}

func (s *navigationService) BuildTruckRouteMultiStop(ctx context.Context, req TruckNavMultiStopRequest) (*TruckNavResult, error) {
	if len(req.Stops) < 2 {
		return nil, fmt.Errorf("here: multi-stop requires at least 2 stops")
	}

	via := []Coordinates{}
	if len(req.Stops) > 2 {
		via = req.Stops[1 : len(req.Stops)-1]
	}

	rr := TruckNavRequest{
		Origin:        req.Stops[0],
		Destination:   req.Stops[len(req.Stops)-1],
		DepartureTime: req.DepartureTime,
		Truck:         req.Truck,
		Avoid:         req.Avoid,
		Lang:          req.Lang,
		Currency:      req.Currency,
		Alternatives:  req.Alternatives,
	}

	// Inject via points directly into the underlying RouteRequest.
	currency := rr.Currency
	if currency == "" {
		currency = "USD"
	}
	lang := rr.Lang
	if lang == "" {
		lang = "en-US"
	}

	low := RouteRequest{
		Origin:        rr.Origin,
		Destination:   rr.Destination,
		DepartureTime: rr.DepartureTime,
		TransportMode: "truck",
		Currency:      currency,
		ReturnOptions: navReturnOptions,
		Via:           via,
		TruckAttrs:    cloneTruck(rr.Truck),
		Avoid:         rr.Avoid,
		Alternatives:  rr.Alternatives,
		Lang:          lang,
	}

	resp, err := s.client.GetRoute(ctx, low)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoTruckRoute, err)
	}
	if len(resp.Routes) == 0 || len(resp.Routes[0].Sections) == 0 {
		return nil, ErrNoTruckRoute
	}

	primary := buildResultFromRoute(resp.Routes[0], req.DepartureTime)
	for i := 1; i < len(resp.Routes); i++ {
		primary.Alternatives = append(primary.Alternatives, buildResultFromRoute(resp.Routes[i], req.DepartureTime))
	}
	return primary, nil
}

func (s *navigationService) RerouteFrom(
	ctx context.Context,
	currentPos Coordinates,
	remainingStops []Coordinates,
	truck TruckAttributes,
	departureTime *time.Time,
) (*TruckNavResult, error) {
	if len(remainingStops) == 0 {
		return nil, fmt.Errorf("here: no remaining stops to reroute to")
	}
	stops := make([]Coordinates, 0, 1+len(remainingStops))
	stops = append(stops, currentPos)
	stops = append(stops, remainingStops...)

	return s.BuildTruckRouteMultiStop(ctx, TruckNavMultiStopRequest{
		Stops:         stops,
		DepartureTime: departureTime,
		Truck:         truck,
	})
}

// cloneTruck returns a pointer to a copy when at least one field is set,
// otherwise nil (so the request omits truck params entirely).
func cloneTruck(t TruckAttributes) *TruckAttributes {
	if t.GrossWeightKg == 0 && t.WeightPerAxleKg == 0 && t.HeightCm == 0 &&
		t.WidthCm == 0 && t.LengthCm == 0 && t.AxleCount == 0 &&
		t.TrailerCount == 0 && t.TunnelCategory == "" &&
		len(t.ShippedHazardousGoods) == 0 {
		return nil
	}
	clone := t
	return &clone
}

// buildResultFromRoute aggregates a HERE Route into a TruckNavResult.
// Distance/duration are summed across all sections so multi-stop is handled.
func buildResultFromRoute(r Route, departureTime *time.Time) *TruckNavResult {
	out := &TruckNavResult{RouteID: r.ID}

	// Take polyline of the first section only — multi-section polylines are
	// concatenated separately by HERE; consumers that need every section can
	// inspect the underlying RouteResponse.
	out.PolylineEncoded = r.Sections[0].Polyline
	if out.PolylineEncoded != "" {
		if coords, err := DecodeFlexiblePolyline(out.PolylineEncoded); err == nil {
			out.PolylineDecoded = coords
		}
	}

	stepIdx := 0
	var totalToll float64
	var tollCurrency string
	var hasToll bool

	for _, section := range r.Sections {
		if section.Summary != nil {
			out.DistanceMeters += section.Summary.Length
			out.DurationSeconds += section.Summary.Duration
			out.BaseDurationSeconds += section.Summary.BaseDuration
		}

		out.Tolls = append(out.Tolls, section.Tolls...)
		out.Notices = append(out.Notices, section.Notices...)

		for _, action := range section.Actions {
			step := NavStep{
				Index:       stepIdx,
				Action:      action.Action,
				Direction:   action.Direction,
				Instruction: action.Instruction,
				DistanceM:   action.Length,
				DurationSec: action.Duration,
				PolyStart:   sanitizeOffset(action.Offset, len(out.PolylineDecoded)),
			}
			if action.NextRoad != nil && len(action.NextRoad.Name) > 0 {
				step.RoadName = action.NextRoad.Name[0].Value
			} else if action.CurrentRoad != nil && len(action.CurrentRoad.Name) > 0 {
				step.RoadName = action.CurrentRoad.Name[0].Value
			}
			out.Steps = append(out.Steps, step)
			stepIdx++
		}

		for _, toll := range section.Tolls {
			for _, fare := range toll.Fares {
				price := fare.Price
				if fare.ConvertedPrice != nil {
					price = *fare.ConvertedPrice
				}
				totalToll += price.Value
				if tollCurrency == "" {
					tollCurrency = price.Currency
				}
				hasToll = true
			}
		}
	}

	// Fill PolyEnd by chaining adjacent steps; last step ends at last point.
	for i := range out.Steps {
		if i+1 < len(out.Steps) {
			out.Steps[i].PolyEnd = out.Steps[i+1].PolyStart
		} else if len(out.PolylineDecoded) > 0 {
			out.Steps[i].PolyEnd = len(out.PolylineDecoded) - 1
		}
	}

	if hasToll {
		out.TotalTollCost = &totalToll
		out.TollCurrency = &tollCurrency
	}

	departure := time.Now()
	if departureTime != nil {
		departure = *departureTime
	}
	out.EstimatedArrival = departure.Add(time.Duration(out.DurationSeconds) * time.Second)

	return out
}

// sanitizeOffset clamps a HERE action offset into the decoded polyline range.
// Some HERE responses contain offsets equal to len(polyline); also guards against
// unexpected negatives.
func sanitizeOffset(offset, length int) int {
	if length == 0 {
		return 0
	}
	if offset < 0 {
		return 0
	}
	if offset >= length {
		return length - 1
	}
	return offset
}
