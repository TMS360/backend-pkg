package here

import (
	"net/url"
	"strconv"
	"strings"
)

// TruckAttributes describes the vehicle for HERE Routing v8 truck mode.
// All fields are optional; zero values are omitted from the request.
// Units: weights in kilograms, dimensions in centimeters.
type TruckAttributes struct {
	GrossWeightKg         int      // truck[grossWeight]
	WeightPerAxleKg       int      // truck[weightPerAxle]
	HeightCm              int      // truck[height]
	WidthCm               int      // truck[width]
	LengthCm              int      // truck[length]
	AxleCount             int      // truck[axleCount]
	TrailerCount          int      // truck[trailerCount]
	TunnelCategory        string   // vehicle[tunnelCategory] — B|C|D|E
	ShippedHazardousGoods []string // vehicle[shippedHazardousGoods]
}

func (t *TruckAttributes) applyTo(params url.Values) {
	if t == nil {
		return
	}
	setIntPositive := func(key string, v int) {
		if v > 0 {
			params.Set(key, strconv.Itoa(v))
		}
	}
	setIntPositive("truck[grossWeight]", t.GrossWeightKg)
	setIntPositive("truck[weightPerAxle]", t.WeightPerAxleKg)
	setIntPositive("truck[height]", t.HeightCm)
	setIntPositive("truck[width]", t.WidthCm)
	setIntPositive("truck[length]", t.LengthCm)
	setIntPositive("truck[axleCount]", t.AxleCount)
	setIntPositive("truck[trailerCount]", t.TrailerCount)

	if t.TunnelCategory != "" {
		params.Set("vehicle[tunnelCategory]", t.TunnelCategory)
	}
	if len(t.ShippedHazardousGoods) > 0 {
		params.Set("vehicle[shippedHazardousGoods]", strings.Join(t.ShippedHazardousGoods, ","))
	}
}

// AvoidOptions controls HERE `avoid[*]` and `exclude[*]` parameters.
type AvoidOptions struct {
	Features  []string // avoid[features]: tollRoad, ferry, dirtRoad, motorway, tunnel, controlledAccessHighway
	Countries []string // exclude[countries]: ISO3
}

func (a *AvoidOptions) applyTo(params url.Values) {
	if a == nil {
		return
	}
	if len(a.Features) > 0 {
		params.Set("avoid[features]", strings.Join(a.Features, ","))
	}
	if len(a.Countries) > 0 {
		params.Set("exclude[countries]", strings.Join(a.Countries, ","))
	}
}

// RouteAction is one turn-by-turn maneuver from HERE `return=actions,instructions`.
type RouteAction struct {
	Action      string `json:"action"`                // depart, turn, arrive, continue, roundaboutExit, ...
	Duration    int    `json:"duration"`              // sec
	Length      int    `json:"length"`                // m
	Instruction string `json:"instruction,omitempty"` // human-readable step
	Offset      int    `json:"offset,omitempty"`      // index in section.polyline
	Direction   string `json:"direction,omitempty"`   // left, right, slightLeft, ...
	Severity    string `json:"severity,omitempty"`    // light, quite, heavy
	ExitNumber  string `json:"exitNumber,omitempty"`
	NextRoad    *Road  `json:"nextRoad,omitempty"`
	CurrentRoad *Road  `json:"currentRoad,omitempty"`
}

type Road struct {
	Name   []NameLang `json:"name,omitempty"`
	Number []NameLang `json:"number,omitempty"`
}

type NameLang struct {
	Value    string `json:"value"`
	Language string `json:"language,omitempty"`
}

// RouteSpan is one slice of the polyline with attributes (HERE `return=spans`).
type RouteSpan struct {
	Offset           int        `json:"offset"`
	Length           int        `json:"length,omitempty"`
	Duration         int        `json:"duration,omitempty"`
	Names            []NameLang `json:"names,omitempty"`
	SpeedLimit       float64    `json:"speedLimit,omitempty"` // m/s
	FunctionalClass  int        `json:"functionalClass,omitempty"`
	CountryCode      string     `json:"countryCode,omitempty"`
}

// RouteNotice flags constraint violations or warnings (e.g. truck restriction).
type RouteNotice struct {
	Title    string `json:"title"`
	Code     string `json:"code"`
	Severity string `json:"severity"` // info | critical
}
