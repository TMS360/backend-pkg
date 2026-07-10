package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/go-redis/redis/v8"
)

// GetCompanyTimezone reads the company's timezone setting (default UTC) so the daily
// cap resets on the company-local day.
func GetCompanyTimezone(ctx context.Context) string {
	var tz string
	_ = cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyTimezone), &tz)
	if tz == "" {
		return "UTC"
	}
	return tz
}

// SamsaraAssetTrackingOn reports whether the company records Samsara GPS actual
// mileage (the default) rather than HERE estimates. Requires an actor in ctx:
// cache.Get prefixes the key with "{companyID}:". An unset setting means enabled
// (default-on preserves existing tenants' behaviour); a cache read failure fails
// closed to OFF so a Redis blip never silently switches a tenant to live-GPS
// deadhead origins (DEV-1197).
func SamsaraAssetTrackingOn(ctx context.Context) bool {
	var v string
	err := cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeySamsaraAssetTrackingEnabled), &v)
	return samsaraTrackingFromCache(v, err)
}

// SamsaraAssetTrackingOnForCompany is the actor-less variant for gRPC/background
// paths where ctx carries no actor: it builds the company-scoped key explicitly
// and unmarshals the JSON-encoded string, mirroring provider.fetchAPIKey.
func SamsaraAssetTrackingOnForCompany(ctx context.Context, companyID string) bool {
	key := fmt.Sprintf("%s:setting:%s", companyID, enums.CompanySettingsGeneralKeySamsaraAssetTrackingEnabled)
	data, err := cache.Client().Get(ctx, key).Bytes()
	var v string
	if err == nil {
		err = json.Unmarshal(data, &v)
	}
	return samsaraTrackingFromCache(v, err)
}

func samsaraTrackingFromCache(v string, err error) bool {
	switch {
	case err == nil:
		return v != "false"
	case errors.Is(err, redis.Nil):
		slog.Info("samsara asset tracking: setting unset, cache miss defaulted to ON")
		return true
	default:
		slog.Error("samsara asset tracking: cache read failed, failing closed OFF", "error", err)
		return false
	}
}
