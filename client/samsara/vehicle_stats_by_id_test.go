package samsara

import (
	"context"
	"strings"
	"testing"
)

// DEV-4313: one truck's live read asks for exactly that vehicle and the types given.
func TestVehiclesStatsWithTypesFiltersByVehicle(t *testing.T) {
	var path string
	c := newStatsTestClient(t, `{"data":[{"id":"42","gps":{"time":"2026-11-14T10:00:00Z","latitude":41.5,"longitude":-87.6},"fuelPercent":{"time":"2026-11-14T09:00:00Z","value":70}}],"pagination":{}}`, &path)

	got, err := c.GetVehiclesStatsWithTypes(context.Background(), []string{StatTypeGPS, StatTypeFuelPercents}, []string{"42"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(path, "vehicleIds=42") || !strings.Contains(path, "types=gps,fuelPercents") {
		t.Errorf("path %q", path)
	}
	if len(got) != 1 || got[0].Gps == nil || got[0].FuelPercents == nil || got[0].FuelPercents.Value != 70 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestAllVehiclesStatsWithTypesHasNoVehicleFilter(t *testing.T) {
	var path string
	c := newStatsTestClient(t, `{"data":[],"pagination":{}}`, &path)
	if _, err := c.GetAllVehiclesStatsWithTypes(context.Background(), nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	if strings.Contains(path, "vehicleIds") {
		t.Errorf("path %q filters vehicles", path)
	}
}
