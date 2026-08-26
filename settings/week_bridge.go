package settings

import (
	"time"

	"github.com/TMS360/backend-pkg/utils"
)

// DEV-1909 — the bridge week.
//
// Moving the first day of the week leaves one week that belongs to neither
// order. Its length is NOT seven days, and which way it bends depends on the
// direction of the change:
//
//   - Monday → Sunday: the last Monday week is cut short at the new Sunday —
//     Monday through Saturday, six days, standing alone.
//   - Sunday → Monday: only the single leftover Sunday would be left over, and a
//     one-day week would charge a full weekly amount twice a day apart. It joins
//     the following Monday week instead: eight days.
//
// In both directions every day belongs to exactly one week, and the bridge week
// carries exactly one weekly charge.
//
// Anything that walks week by week must step with NextWeekStart, never by adding
// seven days: over the bridge, seven days lands mid-week.
//
// Dates here are naive calendar days (UTC midnight) and are never converted
// between zones — the digits the user typed are the digits the week is cut on
// (DEV-1031 / DEV-1148). Report windows keep their own deliberate conversion in
// timewindow.StartOfWeekOn.

// Week is one company week: the days [Start, EndExclusive).
type Week struct {
	Start        time.Time
	EndExclusive time.Time
	// FirstDay is the weekday Start falls on — the day in force for THIS week,
	// which over the bridge is the old one.
	FirstDay time.Weekday
	// Days is the real length: 7 normally, 6 or 8 across the bridge.
	Days int
	// Bridge marks the one week that joins the old order to the new one. A client
	// showing weekly totals should label it, since it is not a full week.
	Bridge bool
}

// IsShort / IsLong describe the bridge week without the caller comparing numbers.
func (w Week) IsShort() bool { return w.Days < 7 }
func (w Week) IsLong() bool  { return w.Days > 7 }

// WeekRule cuts a company's calendar into weeks: which day they start on, and —
// when the day was changed — from which date the new day applies.
type WeekRule struct {
	// Day is the configured (new) first day.
	Day FirstDayOfWeek
	// From is the date the new day starts counting from. Zero means the company
	// never changed the day, so every week starts on Day.
	From time.Time
}

// NewWeekRule builds the rule. from may be the zero time (no change recorded).
// A from that does not itself fall on day is moved forward to the next such day,
// the same correction the reader applies — the save path refuses to store one.
func NewWeekRule(day FirstDayOfWeek, from time.Time) WeekRule {
	if !day.IsValid() {
		day = DefaultFirstDayOfWeek
	}
	if from.IsZero() {
		return WeekRule{Day: day}
	}
	return WeekRule{Day: day, From: nextWeekdayOnOrAfter(from, day.Weekday())}
}

// previousDay is the day in force before the change. Only two are supported, so
// the old day is simply the other one.
func (r WeekRule) previousDay() FirstDayOfWeek {
	if r.Day == FirstDayOfWeekSunday {
		return FirstDayOfWeekMonday
	}
	return FirstDayOfWeekSunday
}

// DayFor answers which day starts the week that holds ref — the same question
// FirstDayOfWeekFor answers, once the setting has been read.
func (r WeekRule) DayFor(ref time.Time) FirstDayOfWeek {
	if r.From.IsZero() || dateOnly(ref).Before(r.From) {
		if r.From.IsZero() {
			return r.Day
		}
		return r.previousDay()
	}
	return r.Day
}

// WeekOf returns the week holding ref, including the bridge week's real length.
func (r WeekRule) WeekOf(ref time.Time) Week {
	ref = dateOnly(ref)
	if r.From.IsZero() {
		return plainWeek(ref, r.Day.Weekday())
	}

	bridge, hasBridge := r.bridgeWeek()
	switch {
	case hasBridge && !ref.Before(bridge.Start) && ref.Before(bridge.EndExclusive):
		return bridge
	case ref.Before(r.From):
		return plainWeek(ref, r.previousDay().Weekday())
	default:
		return plainWeek(ref, r.Day.Weekday())
	}
}

// NextWeekStart is the first day of the week after the one holding ref. The ONLY
// safe way to walk weeks: adding seven days lands mid-week across the bridge.
func (r WeekRule) NextWeekStart(ref time.Time) time.Time {
	return r.WeekOf(ref).EndExclusive
}

// bridgeWeek computes the one week that joins the two orders.
//
// The gap is how many days the last old week has left when the new day arrives:
// six for Monday → Sunday (Monday..Saturday, a short standalone week), one for
// Sunday → Monday — and that single day is merged into the following week rather
// than standing as a one-day week.
func (r WeekRule) bridgeWeek() (Week, bool) {
	if r.From.IsZero() {
		return Week{}, false
	}
	oldStart := utils.GetWeekStartOn(r.From.AddDate(0, 0, -1), r.previousDay().Weekday())
	gap := int(r.From.Sub(oldStart).Hours() / 24)
	if gap <= 0 {
		return Week{}, false // the change lands exactly on an old week start: no orphan days
	}
	if gap == 1 {
		// Merge the orphan day into the first new week: 1 + 7 = 8 days.
		end := r.From.AddDate(0, 0, 7)
		return Week{
			Start:        oldStart,
			EndExclusive: end,
			FirstDay:     oldStart.Weekday(),
			Days:         gap + 7,
			Bridge:       true,
		}, true
	}
	return Week{
		Start:        oldStart,
		EndExclusive: r.From,
		FirstDay:     oldStart.Weekday(),
		Days:         gap,
		Bridge:       true,
	}, true
}

// WeekStartIn is the start of the week holding t, stamped at midnight in loc —
// the cut a report or board window asks for (timewindow.Window.ResolveWeeks).
//
// The rule itself answers in naive UTC calendar days on purpose (DEV-1031 /
// DEV-1148): the digits the user typed decide the week. A window, though, opens
// at LOCAL midnight, so the calendar day the rule picked is re-stamped in loc
// rather than converted — no instant is shifted across a day boundary.
func (r WeekRule) WeekStartIn(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	start := r.WeekOf(dateOnlyIn(local)).Start
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
}

// dateOnlyIn is the calendar day of local as a naive UTC date, which is the only
// shape the rule reasons about.
func dateOnlyIn(local time.Time) time.Time {
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func plainWeek(ref time.Time, firstDay time.Weekday) Week {
	start := utils.GetWeekStartOn(ref, firstDay)
	return Week{
		Start:        start,
		EndExclusive: start.AddDate(0, 0, 7),
		FirstDay:     firstDay,
		Days:         7,
	}
}
