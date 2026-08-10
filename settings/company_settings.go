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

// EmptyMilesWorkflow is when a trip's empty (deadhead) miles get written.
type EmptyMilesWorkflow string

const (
	// EmptyMilesWorkflowAuto recomputes and persists empty miles on every
	// lifecycle step that can move the deadhead origin. Historical behaviour.
	EmptyMilesWorkflowAuto EmptyMilesWorkflow = "auto"
	// EmptyMilesWorkflowDeferred leaves empty miles NULL until dispatch has
	// checked the origin and explicitly calculated. Nothing writes them by itself.
	EmptyMilesWorkflowDeferred EmptyMilesWorkflow = "deferred"
)

// DefaultEmptyMilesWorkflow is what an unconfigured (or unreadable) tenant gets.
// Deliberately the permissive end: a company that predates the setting, or one
// whose Redis blipped, must keep behaving exactly as it did before the feature
// shipped. Flipping an unreadable tenant to deferred would silently stop empty
// miles from ever being written and stall their driver pay.
const DefaultEmptyMilesWorkflow = EmptyMilesWorkflowAuto

func (w EmptyMilesWorkflow) IsValid() bool {
	switch w {
	case EmptyMilesWorkflowAuto, EmptyMilesWorkflowDeferred:
		return true
	}
	return false
}

// IsDeferred is the predicate every auto-write path gates on.
func (w EmptyMilesWorkflow) IsDeferred() bool { return w == EmptyMilesWorkflowDeferred }

func (w EmptyMilesWorkflow) String() string { return string(w) }

// EmptyMilesWorkflowFor reads the company's empty-miles mode. Requires an actor
// in ctx: cache.Get prefixes the key with "{companyID}:". Use the ForCompany
// variant on gRPC/Kafka/background paths, where ctx carries no actor and this
// would silently read an unprefixed key and fall back to auto.
func EmptyMilesWorkflowFor(ctx context.Context) EmptyMilesWorkflow {
	var v string
	err := cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyEmptyMilesWorkflow), &v)
	return emptyMilesWorkflowFromCache(v, err)
}

// EmptyMilesWorkflowForCompany is the actor-less variant: it builds the
// company-scoped key explicitly and unmarshals the JSON-encoded string, mirroring
// SamsaraAssetTrackingOnForCompany.
func EmptyMilesWorkflowForCompany(ctx context.Context, companyID string) EmptyMilesWorkflow {
	key := fmt.Sprintf("%s:setting:%s", companyID, enums.CompanySettingsGeneralKeyEmptyMilesWorkflow)
	data, err := cache.Client().Get(ctx, key).Bytes()
	var v string
	if err == nil {
		err = json.Unmarshal(data, &v)
	}
	return emptyMilesWorkflowFromCache(v, err)
}

// emptyMilesWorkflowFromCache maps a raw cache read to a mode. Both a miss and a
// read failure degrade to auto — see DefaultEmptyMilesWorkflow. An unrecognized
// stored value degrades the same way rather than guessing.
func emptyMilesWorkflowFromCache(v string, err error) EmptyMilesWorkflow {
	switch {
	case err == nil:
		if w := EmptyMilesWorkflow(strings.ToLower(strings.TrimSpace(v))); w.IsValid() {
			return w
		}
		slog.Warn("empty miles workflow: unrecognized value, defaulting to auto", "value", v)
		return DefaultEmptyMilesWorkflow
	case errors.Is(err, redis.Nil):
		return DefaultEmptyMilesWorkflow
	default:
		slog.Error("empty miles workflow: cache read failed, defaulting to auto", "error", err)
		return DefaultEmptyMilesWorkflow
	}
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
