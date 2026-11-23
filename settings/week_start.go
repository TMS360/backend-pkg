package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/go-redis/redis/v8"
)

// DEV-1909 — the company's first day of the week.
//
// Some companies count a week from Monday, others from Sunday. Every service
// used to count from Monday on its own; this is the one place that answers
// "which day starts the week that holds this date?" so a later change is made
// once, here.
//
// The answer is DATE-AWARE on purpose. The setting is stored with the date the
// new day starts counting from, and a week that begins before that date keeps
// the old day — otherwise flipping the setting would silently re-cut weeks that
// payroll, audits and boards have already been written against.

// FirstDayOfWeek is the weekday a company's week starts on.
type FirstDayOfWeek string

const (
	FirstDayOfWeekMonday FirstDayOfWeek = "monday"
	FirstDayOfWeekSunday FirstDayOfWeek = "sunday"
)

// DefaultFirstDayOfWeek is what an unconfigured — or unreadable — tenant gets.
// Monday, because that is what every service counted before this setting
// existed: a company with nothing saved, and a company whose Redis blipped, must
// both keep cutting weeks exactly where they did yesterday.
const DefaultFirstDayOfWeek = FirstDayOfWeekMonday

func (d FirstDayOfWeek) IsValid() bool {
	switch d {
	case FirstDayOfWeekMonday, FirstDayOfWeekSunday:
		return true
	}
	return false
}

func (d FirstDayOfWeek) String() string { return string(d) }

// Weekday converts to Go's weekday, which is what the week helpers take
// (utils.GetWeekStartOn, timewindow.StartOfWeekOn). An unrecognized value
// answers Monday rather than panicking — same rule as every other read here.
func (d FirstDayOfWeek) Weekday() time.Weekday {
	if d == FirstDayOfWeekSunday {
		return time.Sunday
	}
	return time.Monday
}

// ParseFirstDayOfWeek accepts the stored spelling (case- and space-insensitive)
// and reports whether it is one this system knows.
func ParseFirstDayOfWeek(v string) (FirstDayOfWeek, bool) {
	d := FirstDayOfWeek(strings.ToLower(strings.TrimSpace(v)))
	return d, d.IsValid()
}

// EffectiveFromLayout is how the effective-from date is stored: a plain calendar
// day. A DATE, never an instant — "from which week does the new day count" is a
// day-level question, and storing a timestamp would make the answer depend on
// whose timezone parsed it. Timezones are explicitly out of this ticket's scope.
const EffectiveFromLayout = "2006-01-02"

// FirstDayOfWeekFor answers which day starts the week that holds ref, for the
// company of the actor in ctx (cache.Get prefixes the key with "{companyID}:").
//
// Use FirstDayOfWeekForCompany on gRPC / Kafka / cron paths: without an actor
// this reads an unprefixed key and silently answers Monday for everyone.
func FirstDayOfWeekFor(ctx context.Context, ref time.Time) FirstDayOfWeek {
	return WeekRuleFor(ctx).DayFor(ref)
}

// WeekRuleFor reads the whole rule — the day AND the date it starts from — for the
// company of the actor in ctx. Callers that cut or walk weeks want this rather
// than FirstDayOfWeekFor: only the rule knows the bridge week's real length.
func WeekRuleFor(ctx context.Context) WeekRule {
	var day string
	dayErr := cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyFirstDayOfWeek), &day)
	var from string
	fromErr := cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyFirstDayOfWeekEffectiveFrom), &from)
	return ResolveWeekRule(day, dayErr, from, fromErr)
}

// FirstDayOfWeekForCompany is the actor-less variant for background work: it
// builds the company-scoped key explicitly and unmarshals the JSON-encoded
// string, mirroring EmptyMilesWorkflowForCompany.
func FirstDayOfWeekForCompany(ctx context.Context, companyID string, ref time.Time) FirstDayOfWeek {
	return WeekRuleForCompany(ctx, companyID).DayFor(ref)
}

// WeekRuleForCompany is the actor-less rule reader for background work.
func WeekRuleForCompany(ctx context.Context, companyID string) WeekRule {
	day, dayErr := rawCompanySetting(ctx, companyID, enums.CompanySettingsGeneralKeyFirstDayOfWeek)
	from, fromErr := rawCompanySetting(ctx, companyID, enums.CompanySettingsGeneralKeyFirstDayOfWeekEffectiveFrom)
	return ResolveWeekRule(day, dayErr, from, fromErr)
}

func rawCompanySetting(ctx context.Context, companyID string, key enums.CompanySettingsGeneralKey) (string, error) {
	data, err := cache.Client().Get(ctx, fmt.Sprintf("%s:setting:%s", companyID, key)).Bytes()
	if err != nil {
		return "", err
	}
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return "", err
	}
	return v, nil
}

// ResolveFirstDayOfWeek turns two raw cache reads plus a reference date into the
// day that starts ref's week. Kept as the simple question; ResolveWeekRule
// answers the full one.
func ResolveFirstDayOfWeek(day string, dayErr error, from string, fromErr error, ref time.Time) FirstDayOfWeek {
	return ResolveWeekRule(day, dayErr, from, fromErr).DayFor(ref)
}

// ResolveWeekRule turns two raw cache reads into the company's week rule.
// Exported and free of Redis so the rule itself is unit-tested; the readers above
// are the only callers that matter.
//
// The rules, in order:
//   - nothing saved, an unknown word, or an unreadable cache → Monday (with a log
//     line for the last two): a company that predates the setting keeps counting
//     exactly as before, and a Redis blip never re-cuts anybody's week;
//   - a non-Monday day saved WITHOUT its start date → Monday plus an error log.
//     Half a setting is a broken setting, and guessing a date here would move
//     weeks nobody asked to move;
//   - a start date that is not itself the configured weekday → moved forward to
//     the next such weekday, with a warning. The save path refuses this, so a row
//     like that means somebody edited the database by hand.
//
// A start date already in the past is fine — the change is simply already in force.
func ResolveWeekRule(day string, dayErr error, from string, fromErr error) WeekRule {
	monday := WeekRule{Day: DefaultFirstDayOfWeek}

	if dayErr != nil {
		if !errors.Is(dayErr, redis.Nil) {
			slog.Error("first day of week: cache read failed, defaulting to monday", "error", dayErr)
		}
		return monday
	}
	configured, ok := ParseFirstDayOfWeek(day)
	if !ok {
		if strings.TrimSpace(day) != "" {
			slog.Error("first day of week: unrecognized value, defaulting to monday", "value", day)
		}
		return monday
	}

	if fromErr != nil {
		if errors.Is(fromErr, redis.Nil) {
			if configured == DefaultFirstDayOfWeek {
				return monday // Monday needs no start date: it IS the old day
			}
			slog.Error("first day of week: day is set but its start date is missing, defaulting to monday",
				"value", day)
		} else {
			slog.Error("first day of week: start date read failed, defaulting to monday", "error", fromErr)
		}
		return monday
	}
	trimmed := strings.TrimSpace(from)
	if trimmed == "" {
		if configured == DefaultFirstDayOfWeek {
			return monday
		}
		slog.Error("first day of week: day is set but its start date is empty, defaulting to monday", "value", day)
		return monday
	}
	start, err := time.Parse(EffectiveFromLayout, trimmed)
	if err != nil {
		slog.Error("first day of week: unparsable start date, defaulting to monday",
			"value", from, "error", err)
		return monday
	}
	return NewWeekRule(configured, start)
}

// nextWeekdayOnOrAfter returns d itself when it already falls on want, else the
// next day that does. Both the warning and the correction live here: a start date
// that is not a first day would otherwise cut one ragged week.
func nextWeekdayOnOrAfter(d time.Time, want time.Weekday) time.Time {
	d = dateOnly(d)
	shift := (int(want) - int(d.Weekday()) + 7) % 7
	if shift == 0 {
		return d
	}
	corrected := d.AddDate(0, 0, shift)
	slog.Warn("first day of week: start date is not a first day, moving it forward",
		"stored", d.Format(EffectiveFromLayout),
		"used", corrected.Format(EffectiveFromLayout),
		"first_day", want.String())
	return corrected
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
