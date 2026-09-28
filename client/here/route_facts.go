package here

import (
	"context"
	"strings"
)

// routeFactSpans are the span attributes CalculateMultiStopRoute asks for. They
// ride on the same /v8/routes transaction, so the facts cost no extra HERE call
// (DEV-1831 was a 429 from parallel calls) (DEV-2527).
var routeFactSpans = []string{"countryCode", "stateCode", "functionalClass", "routeNumbers"}

// RouteFacts is what a driver-preference check needs to know about one leg.
type RouteFacts struct {
	// HasTolls is nil when toll data was not requested (see WithTolls).
	HasTolls       *bool
	UsesInterstate bool
	States         []string // HERE stateCode, in route order, deduplicated
	Countries      []string // ISO 3166-1 alpha-3, in route order, deduplicated
}

type tollsKey struct{}

// WithTolls asks CalculateMultiStopRoute to add `return=tolls`. HERE may bill
// toll data separately, so callers enable it per company.
func WithTolls(ctx context.Context) context.Context {
	return context.WithValue(ctx, tollsKey{}, true)
}

func tollsFromCtx(ctx context.Context) bool {
	on, _ := ctx.Value(tollsKey{}).(bool)
	return on
}

// sectionFacts reduces a section's spans (and tolls, when asked for) to facts.
// A section without spans yields nil: HERE sent no data, so nothing is known.
func sectionFacts(sec RouteSection, tollsRequested bool) *RouteFacts {
	if len(sec.Spans) == 0 {
		return nil
	}
	f := &RouteFacts{}
	if tollsRequested {
		has := len(sec.Tolls) > 0
		f.HasTolls = &has
	}
	for _, sp := range sec.Spans {
		if sp.FunctionalClass == 1 {
			f.UsesInterstate = true
		}
		for _, rn := range sp.RouteNumbers {
			if strings.HasPrefix(rn.Value, "I-") {
				f.UsesInterstate = true
			}
		}
		f.States = appendNew(f.States, sp.StateCode)
		f.Countries = appendNew(f.Countries, sp.CountryCode)
	}
	return f
}

func appendNew(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
