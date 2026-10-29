package toll

import (
	"context"
	"net/url"
	"time"
)

// BestPassActive is the only equipment status that means "in use". Every
// other status (LOST, INACTIVE, RETURN, UNKNOWN …) is a box BestPass no longer
// stands behind.
const BestPassActive = "ACTIVE"

// BestPassPlate is a vehicle's license plate as BestPass keeps it.
type BestPassPlate struct {
	Number    string `json:"number"`
	StateProv string `json:"stateProv"`
}

// BestPassVehicle is one vehicle of an account. Unit is BestPass's own key
// for it and is never a match key for a TMS truck.
type BestPassVehicle struct {
	Unit         string        `json:"unit"`
	VIN          string        `json:"vin"`
	LicensePlate BestPassPlate `json:"licensePlate"`
}

// BestPassAssignment is the vehicle a transponder is on, by unit.
type BestPassAssignment struct {
	Unit         string     `json:"unit"`
	InactiveDate *time.Time `json:"inactiveDate"`
}

// BestPassEquipment is one transponder of an account. AssignedVehicle names
// the vehicle by unit only; join it to Vehicles for VIN and plate.
type BestPassEquipment struct {
	Number          string              `json:"equipmentNumber"`
	StatusCode      string              `json:"statusCode"`
	AssignedVehicle *BestPassAssignment `json:"assignedVehicle"`
}

// AssignedUnit is the unit the transponder is on now, or "" when it sits on
// no vehicle or the assignment has ended.
func (e BestPassEquipment) AssignedUnit() string {
	if e.AssignedVehicle == nil || e.AssignedVehicle.InactiveDate != nil {
		return ""
	}
	return e.AssignedVehicle.Unit
}

// Vehicles lists one account's vehicles. Read only.
func (c *BestPassAPI) Vehicles(ctx context.Context, account string) ([]BestPassVehicle, error) {
	q := url.Values{}
	q.Set("accountNumber", account)
	return getAllPages[BestPassVehicle](ctx, c, "/vehicles", q)
}

// Equipment lists every transponder of one account, inactive and lost ones
// included. Read only.
func (c *BestPassAPI) Equipment(ctx context.Context, account string) ([]BestPassEquipment, error) {
	q := url.Values{}
	q.Set("accountNumber", account)
	q.Set("activeOnly", "false")
	return getAllPages[BestPassEquipment](ctx, c, "/equipment", q)
}
