package toll

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fleetHook serves /vehicles and /equipment one result per page, recording
// each query.
func fleetHook(vehicles, equipment []map[string]any, seen *[]url.Values) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		var all []map[string]any
		switch r.URL.Path {
		case "/vehicles":
			all = vehicles
		case "/equipment":
			all = equipment
		default:
			return false
		}
		*seen = append(*seen, r.URL.Query())
		page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
		var next any
		if page < len(all) {
			next = "more"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pagingInfo": map[string]any{"nextPage": next},
			"results":    all[page-1 : page],
		})
		return true
	}
}

func TestBestPassAPI_VehiclesAndEquipmentReadEveryPage(t *testing.T) {
	vehicles := []map[string]any{
		{"unit": "101", "vin": "1xkyd49x0lj123456", "licensePlate": map[string]any{"number": "P-123", "stateProv": "IL"}},
		{"unit": "$IL_P777", "vin": nil, "licensePlate": map[string]any{"number": "P777", "stateProv": "IL"}},
	}
	equipment := []map[string]any{
		{"equipmentNumber": "B0001", "statusCode": "ACTIVE", "assignedVehicle": map[string]any{"unit": "101", "activeDate": "2026-01-01T00:00:00Z"}},
		{"equipmentNumber": "B0002", "statusCode": "LOST", "assignedVehicle": map[string]any{"unit": "101", "activeDate": "2026-01-01T00:00:00Z", "inactiveDate": "2026-06-01T00:00:00Z"}},
		{"equipmentNumber": "B0003", "statusCode": "INACTIVE", "assignedVehicle": nil},
	}
	var seen []url.Values
	c, _ := newFakeClient(t, &fakeBestPass{hook: fleetHook(vehicles, equipment, &seen)})

	vs, err := c.Vehicles(t.Context(), "59471")
	require.NoError(t, err)
	es, err := c.Equipment(t.Context(), "59471")
	require.NoError(t, err)

	require.Len(t, vs, 2)
	assert.Equal(t, BestPassVehicle{Unit: "101", VIN: "1xkyd49x0lj123456", LicensePlate: BestPassPlate{Number: "P-123", StateProv: "IL"}}, vs[0])
	assert.Empty(t, vs[1].VIN, "a null VIN reads as empty")

	require.Len(t, es, 3)
	assert.Equal(t, "101", es[0].AssignedUnit())
	assert.Equal(t, "LOST", es[1].StatusCode)
	assert.Empty(t, es[1].AssignedUnit(), "an ended assignment puts the box on no vehicle")
	assert.Empty(t, es[2].AssignedUnit())

	require.Len(t, seen, 5, "two vehicle pages and three equipment pages")
	for _, q := range seen {
		assert.Equal(t, "59471", q.Get("accountNumber"))
	}
	assert.Empty(t, seen[0].Get("activeOnly"))
	assert.Equal(t, "false", seen[2].Get("activeOnly"), "lost and inactive boxes must come back too")
}

func TestBestPassAPI_VehiclesRefusedIsAuthError(t *testing.T) {
	f := &fakeBestPass{hook: func(w http.ResponseWriter, _ *http.Request) bool { w.WriteHeader(http.StatusForbidden); return true }}
	c, _ := newFakeClient(t, f)

	_, err := c.Vehicles(t.Context(), "59471")
	assert.True(t, IsAuthError(err))
	_, err = c.Equipment(t.Context(), "59471")
	assert.True(t, IsAuthError(err))
}
