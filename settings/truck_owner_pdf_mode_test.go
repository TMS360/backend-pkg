package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// DEV-2914. A company that never saved the setting — or whose read fails, or
// whose stored value is junk — gets the one-PDF packaging it always had.
func TestTruckOwnerPDFModeFromCache(t *testing.T) {
	cases := []struct {
		name  string
		value string
		err   error
		want  TruckOwnerPDFMode
	}{
		{name: "per_truck", value: "per_truck", want: TruckOwnerPDFModePerTruck},
		{name: "single", value: "single", want: TruckOwnerPDFModeSingle},
		{name: "mixed case and padding still parses", value: " Per_Truck ", want: TruckOwnerPDFModePerTruck},
		{name: "never saved reads single", err: redis.Nil, want: TruckOwnerPDFModeSingle},
		{name: "read failure reads single", err: errors.New("dial tcp: connection refused"), want: TruckOwnerPDFModeSingle},
		{name: "empty value reads single", value: "", want: TruckOwnerPDFModeSingle},
		{name: "unrecognized value reads single", value: "zip", want: TruckOwnerPDFModeSingle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, truckOwnerPDFModeFromCache(tc.value, tc.err))
		})
	}
}

func TestTruckOwnerPDFModeForCompany_NoRedisReadsSingle(t *testing.T) {
	require.Equal(t, TruckOwnerPDFModeSingle, TruckOwnerPDFModeForCompany(context.Background(), "c1"))
}

func TestTruckOwnerPDFMode_IsValid(t *testing.T) {
	for _, m := range AllTruckOwnerPDFModes {
		require.True(t, m.IsValid(), m)
	}
	for _, bad := range []TruckOwnerPDFMode{"", "zip", "SINGLE", "per-truck"} {
		require.False(t, bad.IsValid(), bad)
	}
	require.True(t, TruckOwnerPDFModePerTruck.IsPerTruck())
	require.False(t, TruckOwnerPDFModeSingle.IsPerTruck())
}

// The stored spellings are a wire contract with tms-auth's save rule and the
// office app; renaming one silently resets every company to single.
func TestTruckOwnerPDFMode_WireValues(t *testing.T) {
	require.Equal(t, "single", TruckOwnerPDFModeSingle.String())
	require.Equal(t, "per_truck", TruckOwnerPDFModePerTruck.String())
	require.Equal(t, TruckOwnerPDFModeSingle, DefaultTruckOwnerPDFMode)
	require.Equal(t, "truck_owner_pdf_mode", enums.CompanySettingsGeneralKeyTruckOwnerPDFMode.String())
	require.True(t, enums.CompanySettingsGeneralKeyTruckOwnerPDFMode.IsValid())
	require.Contains(t, enums.AllCompanySettingsGeneralKey, enums.CompanySettingsGeneralKeyTruckOwnerPDFMode)
}
