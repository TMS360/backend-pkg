package samsara

import (
	"encoding/json"
	"testing"
)

func TestVehicleLocationReadsEitherFuelKey(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *float64
	}{
		{"snapshot singular", `{"id":"1","gps":{"latitude":1,"longitude":2},"fuelPercent":{"value":41,"time":"t"}}`, fuelPtr(41)},
		{"plural", `{"id":"1","fuelPercents":{"value":55,"time":"t"}}`, fuelPtr(55)},
		{"both, plural wins", `{"id":"1","fuelPercents":{"value":55},"fuelPercent":{"value":41}}`, fuelPtr(55)},
		{"none", `{"id":"1"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp VehicleLocationResponse
			if err := json.Unmarshal([]byte(`{"data":[`+tc.body+`]}`), &resp); err != nil {
				t.Fatal(err)
			}
			got := resp.Data[0].FuelPercents
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("want no fuel, got %v", got.Value)
			case tc.want != nil && (got == nil || got.Value != *tc.want):
				t.Fatalf("want fuel %v, got %+v", *tc.want, got)
			}
			if resp.Data[0].ID != "1" {
				t.Fatalf("other fields lost: id=%q", resp.Data[0].ID)
			}
		})
	}
}

func fuelPtr(v float64) *float64 { return &v }
