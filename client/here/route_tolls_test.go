package here

import (
	"encoding/json"
	"testing"
)

// The shape HERE v8 returns with `return=tolls&currency=USD` (DEV-4396).
const tollsSection = `{"id":"s0","tolls":[
 {"countryCode":"USA","tollSystem":"Indiana Toll Road","fares":[
  {"name":"Cash","price":{"value":45.5,"currency":"USD"},"convertedPrice":{"value":45.5,"currency":"USD"}},
  {"name":"E-ZPass","price":{"value":38.2,"currency":"USD"},"convertedPrice":{"value":38.2,"currency":"USD"}}]}],
"tollSystems":[{"name":"Indiana Toll Road"}]}`

func TestRouteSection_DecodesTollFares(t *testing.T) {
	var sec RouteSection
	if err := json.Unmarshal([]byte(tollsSection), &sec); err != nil {
		t.Fatal(err)
	}
	if len(sec.Tolls) != 1 || len(sec.Tolls[0].Fares) != 2 {
		t.Fatalf("tolls = %+v", sec.Tolls)
	}
	f := sec.Tolls[0].Fares[1]
	if f.Name != "E-ZPass" || f.ConvertedPrice == nil || f.ConvertedPrice.Value != 38.2 {
		t.Errorf("fare = %+v", f)
	}
	if sec.Tolls[0].TollSystem != "Indiana Toll Road" || len(sec.TollSystems) != 1 {
		t.Errorf("toll system = %q, %+v", sec.Tolls[0].TollSystem, sec.TollSystems)
	}
}
