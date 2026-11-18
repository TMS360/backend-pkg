// Package companytz is the fleet's single answer to "what time is it for this
// tenant, and what digits does that put in our timestamp columns".
//
// # Why this is shared and not per-service
//
// tms-auth publishes the company's IANA timezone to the settings cache at
// {companyID}:setting:timezone. Before this package, six services each read that
// slot their own way and converted it their own way — including two packages
// inside one service that disagreed on what to do when the read failed. That
// duplication produced DEV-2076: the Late PU predicate compared naive
// wall-clock columns against time.Now().UTC(), so every Central-time carrier saw
// loads flagged late up to six hours before their pickup window even opened.
//
// # The two clocks
//
// Much of the fleet stores NAIVE wall-clock timestamps (DEV-1204): appointment
// windows, check-in times, and anything a person reads off a clock at a facility.
// Those columns hold digits, labeled UTC as a storage convention rather than as a
// claim about an instant. Comparing such a column against a real UTC instant puts
// the two sides on different clocks, and the gap between them is the tenant's zone
// offset.
//
// So: compare naive columns against Now(ctx) / WallClock(...), never against
// time.Now().UTC(). Compare genuine instants (created_at, recorded_at) against
// time.Now() as usual.
//
// # Which company
//
// Timezone/Location/Now read the ACTING user's company out of the context. On
// gRPC, Kafka and cron paths there is no actor, and those would silently read an
// unprefixed key and fall back to UTC — use the ForCompany variants there. This
// mirrors the pairing settings.SamsaraAssetTrackingOnForCompany already uses.
//
// # Not in scope
//
// The zone of a PLACE (a facility two states away) is a different question and is
// resolved from coordinates, not from this setting — see tms-loads' geotz.
package companytz

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	// The IANA database is embedded rather than read from the host. Every
	// service image currently installs tzdata by hand; the first one that
	// forgets silently degrades every tenant to UTC, which is the same class of
	// failure this package exists to remove. A few hundred KB of binary is the
	// cheaper side of that trade.
	_ "time/tzdata"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
)

// Default is what an unconfigured, unreachable or unparseable timezone resolves
// to. It is deliberately UTC and not the Central default tms-auth shows in its
// settings screen: a tenant whose setting is switched off must behave exactly
// like one that never had it, and every consumer of this package predates the
// setting existing.
const Default = "UTC"

// settingKey is the unscoped cache key. cache.Get prefixes the acting company;
// cache.ScopedKey prefixes an explicit one. Defined once so the two can never
// drift apart.
func settingKey() string {
	return fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyTimezone)
}

// Timezone returns the acting company's IANA timezone name, or Default.
//
// Requires an actor in ctx. On an actor-less path use TimezoneForCompany.
func Timezone(ctx context.Context) string {
	if cache.Client() == nil {
		return Default
	}
	var tz string
	_ = cache.Get(ctx, settingKey(), &tz)
	return orDefault(tz)
}

// TimezoneForCompany returns the named company's IANA timezone name, or Default.
// This is the variant for gRPC, Kafka consumers and cron jobs, where ctx carries
// no actor and Timezone would read an unprefixed key and always answer UTC.
func TimezoneForCompany(ctx context.Context, companyID string) string {
	if cache.Client() == nil || companyID == "" {
		return Default
	}
	var tz string
	_ = cache.GetGlobal(ctx, cache.ScopedKey(companyID, settingKey()), &tz)
	return orDefault(tz)
}

// Location returns the acting company's *time.Location. It never returns nil:
// an absent, unreachable or unparseable setting yields time.UTC, so a caller can
// always use the result directly.
func Location(ctx context.Context) *time.Location {
	return locationOf(ctx, Timezone(ctx))
}

// LocationForCompany is the actor-less variant of Location.
func LocationForCompany(ctx context.Context, companyID string) *time.Location {
	return locationOf(ctx, TimezoneForCompany(ctx, companyID))
}

// Now is the acting company's wall clock: the digits a person sitting in that
// company's timezone reads right now, labeled UTC the way naive timestamp
// columns are stored. Feed it to any comparison against such a column.
func Now(ctx context.Context) time.Time {
	return nowIn(ctx, Timezone(ctx))
}

// NowForCompany is the actor-less variant of Now.
func NowForCompany(ctx context.Context, companyID string) time.Time {
	return nowIn(ctx, TimezoneForCompany(ctx, companyID))
}

// WallClock reinterprets the absolute instant t as the wall-clock digits read in
// tz, labeled UTC — the storage convention of the naive timestamp columns.
// Comparing such a column against WallClock(...) compares like with like.
//
// ok is false only when a NON-EMPTY tz could not be resolved, i.e. a real
// misconfiguration worth logging. An unset timezone is not an error: it means the
// company never picked one, and Default is the documented answer.
func WallClock(t time.Time, tz string) (wallClock time.Time, ok bool) {
	loc, ok := ParseLocation(tz)
	return WallClockIn(t, loc), ok
}

// WallClockIn is WallClock for callers that already hold a *time.Location. A nil
// loc is treated as UTC.
func WallClockIn(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	return time.Date(
		local.Year(), local.Month(), local.Day(),
		local.Hour(), local.Minute(), local.Second(), local.Nanosecond(),
		time.UTC,
	)
}

// ParseLocation resolves an IANA timezone name, falling back to time.UTC. It
// never returns nil. ok is false only for a non-empty name that failed to load.
//
// Note that time.LoadLocation("") succeeds and yields UTC, which is why an empty
// name is reported as ok: it is indistinguishable from an unset setting and must
// not be logged as a misconfiguration.
func ParseLocation(tz string) (loc *time.Location, ok bool) {
	if tz == "" || tz == Default {
		return time.UTC, true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC, false
	}
	return loc, true
}

func orDefault(tz string) string {
	if tz == "" {
		return Default
	}
	return tz
}

func locationOf(ctx context.Context, tz string) *time.Location {
	loc, ok := ParseLocation(tz)
	if !ok {
		warnUnknown(ctx, tz)
	}
	return loc
}

func nowIn(ctx context.Context, tz string) time.Time {
	wallClock, ok := WallClock(time.Now(), tz)
	if !ok {
		warnUnknown(ctx, tz)
	}
	return wallClock
}

func warnUnknown(ctx context.Context, tz string) {
	slog.WarnContext(ctx, "company timezone: unknown zone, falling back to UTC", "timezone", tz)
}
