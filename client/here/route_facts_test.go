package here

import (
	"context"
	"reflect"
	"testing"
)

// Chicago → Pittsburgh shape: IL/IN/OH/PA on I-80/I-76 with a toll section.
const factsBody = `{"routes":[{"id":"r1","sections":[{"id":"s0","summary":{"length":740000,"duration":27000,"baseDuration":26000},"polyline":"p",
"tolls":[{"countryCode":"USA"}],
"spans":[
 {"offset":0,"countryCode":"USA","stateCode":"IL","functionalClass":2},
 {"offset":5,"countryCode":"USA","stateCode":"IN","functionalClass":1,"routeNumbers":[{"value":"I-80"}]},
 {"offset":9,"countryCode":"USA","stateCode":"IN","functionalClass":1},
 {"offset":12,"countryCode":"USA","stateCode":"OH","functionalClass":1},
 {"offset":20,"countryCode":"USA","stateCode":"PA","functionalClass":2,"routeNumbers":[{"value":"I-76"}]}]}]}]}`

func twoStops() []Coordinates {
	return []Coordinates{{Latitude: 41.88, Longitude: -87.63}, {Latitude: 40.44, Longitude: -79.99}}
}

func TestRouteFacts_SameCallCarriesSpansAndTolls(t *testing.T) {
	svc, calls, reqs := newCountingService(t, factsBody)

	got, err := svc.CalculateMultiStopRoute(WithTolls(context.Background()), twoStops(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls() != 1 {
		t.Fatalf("HERE calls = %d, want 1", calls())
	}
	q := reqs()[0].URL.Query()
	if q.Get("spans") != "countryCode,stateCode,functionalClass,routeNumbers" {
		t.Errorf("spans=%q", q.Get("spans"))
	}
	if q.Get("return") != "summary,polyline,tolls" {
		t.Errorf("return=%q", q.Get("return"))
	}

	f := got.Legs[0].Facts
	if f == nil {
		t.Fatal("Facts nil")
	}
	if f.HasTolls == nil || !*f.HasTolls {
		t.Errorf("HasTolls = %v, want true", f.HasTolls)
	}
	if !f.UsesInterstate {
		t.Error("UsesInterstate = false")
	}
	if want := []string{"IL", "IN", "OH", "PA"}; !reflect.DeepEqual(f.States, want) {
		t.Errorf("States = %v, want %v", f.States, want)
	}
	if want := []string{"USA"}; !reflect.DeepEqual(f.Countries, want) {
		t.Errorf("Countries = %v, want %v", f.Countries, want)
	}
}

// Without WithTolls the request must not ask for tolls, and the fact stays unknown.
func TestRouteFacts_TollsOffByDefault(t *testing.T) {
	svc, _, reqs := newCountingService(t, factsBody)

	got, err := svc.CalculateMultiStopRoute(context.Background(), twoStops(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := reqs()[0].URL.Query().Get("return"); r != "summary,polyline" {
		t.Errorf("return=%q", r)
	}
	if got.Legs[0].Facts.HasTolls != nil {
		t.Errorf("HasTolls = %v, want nil (not requested)", *got.Legs[0].Facts.HasTolls)
	}
}

// A HERE answer without spans means nothing is known — not "no tolls, no states".
func TestRouteFacts_NoSpansIsUnknown(t *testing.T) {
	svc, _, _ := newCountingService(t, sectionsBody([]int{100}, []int{10}, []int{11}))

	got, err := svc.CalculateMultiStopRoute(WithTolls(context.Background()), twoStops(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Legs[0].Facts != nil {
		t.Errorf("Facts = %+v, want nil", got.Legs[0].Facts)
	}
}

func TestRouteFacts_NoInterstateOnLocalRoads(t *testing.T) {
	f := sectionFacts(RouteSection{Spans: []RouteSpan{{FunctionalClass: 3, RouteNumbers: []RouteNumber{{Value: "US-6"}}}}}, false)
	if f.UsesInterstate {
		t.Error("UsesInterstate = true on FC3 / US-6")
	}
}
