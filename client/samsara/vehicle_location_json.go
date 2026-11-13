package samsara

import "encoding/json"

// UnmarshalJSON accepts the fuel level under either key. The stats feed
// names it "fuelPercents"; the snapshot answer of /fleet/vehicles/stats
// names it "fuelPercent". When both are present, "fuelPercents" wins.
func (v *VehicleLocation) UnmarshalJSON(data []byte) error {
	type plain VehicleLocation
	var aux struct {
		plain
		FuelPercent *FuelPercent `json:"fuelPercent"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*v = VehicleLocation(aux.plain)
	if v.FuelPercents == nil {
		v.FuelPercents = aux.FuelPercent
	}
	return nil
}
