package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/go-redis/redis/v8"
)

// TruckOwnerPDFMode is how a Truck Owner settlement PDF is packaged (DEV-2914).
type TruckOwnerPDFMode string

const (
	// TruckOwnerPDFModeSingle is one PDF: the summary page first, then one
	// section per truck.
	TruckOwnerPDFModeSingle TruckOwnerPDFMode = "single"
	// TruckOwnerPDFModePerTruck is a summary PDF plus one PDF per truck.
	TruckOwnerPDFModePerTruck TruckOwnerPDFMode = "per_truck"
)

// DefaultTruckOwnerPDFMode is what an unconfigured (or unreadable) tenant gets:
// the one-PDF packaging every company had before the setting existed.
const DefaultTruckOwnerPDFMode = TruckOwnerPDFModeSingle

// AllTruckOwnerPDFModes lists the accepted values, in the order they are shown.
var AllTruckOwnerPDFModes = []TruckOwnerPDFMode{TruckOwnerPDFModeSingle, TruckOwnerPDFModePerTruck}

func (m TruckOwnerPDFMode) IsValid() bool {
	switch m {
	case TruckOwnerPDFModeSingle, TruckOwnerPDFModePerTruck:
		return true
	}
	return false
}

// IsPerTruck is the predicate the PDF builder branches on.
func (m TruckOwnerPDFMode) IsPerTruck() bool { return m == TruckOwnerPDFModePerTruck }

func (m TruckOwnerPDFMode) String() string { return string(m) }

// TruckOwnerPDFModeFor reads the company's Truck Owner PDF packaging. Requires an
// actor in ctx: cache.Get prefixes the key with "{companyID}:". Use the
// ForCompany variant on gRPC/Kafka/background paths.
func TruckOwnerPDFModeFor(ctx context.Context) TruckOwnerPDFMode {
	var v string
	err := cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyTruckOwnerPDFMode), &v)
	return truckOwnerPDFModeFromCache(v, err)
}

// TruckOwnerPDFModeForCompany is the actor-less variant: it builds the
// company-scoped key explicitly, mirroring EmptyMilesWorkflowForCompany.
func TruckOwnerPDFModeForCompany(ctx context.Context, companyID string) TruckOwnerPDFMode {
	if cache.Client() == nil {
		return DefaultTruckOwnerPDFMode
	}
	key := fmt.Sprintf("%s:setting:%s", companyID, enums.CompanySettingsGeneralKeyTruckOwnerPDFMode)
	data, err := cache.Client().Get(ctx, key).Bytes()
	var v string
	if err == nil {
		err = json.Unmarshal(data, &v)
	}
	return truckOwnerPDFModeFromCache(v, err)
}

// truckOwnerPDFModeFromCache maps a raw cache read to a mode. A miss, a read
// failure and an unrecognized stored value all read as single.
func truckOwnerPDFModeFromCache(v string, err error) TruckOwnerPDFMode {
	switch {
	case err == nil:
		if m := TruckOwnerPDFMode(strings.ToLower(strings.TrimSpace(v))); m.IsValid() {
			return m
		}
		slog.Warn("truck owner pdf mode: unrecognized value, defaulting to single", "value", v)
		return DefaultTruckOwnerPDFMode
	case errors.Is(err, redis.Nil):
		return DefaultTruckOwnerPDFMode
	default:
		slog.Error("truck owner pdf mode: cache read failed, defaulting to single", "error", err)
		return DefaultTruckOwnerPDFMode
	}
}
