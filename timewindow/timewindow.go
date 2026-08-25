// Package timewindow defines the relative reporting windows the whole platform
// shares: "last week", "last N months", week/month/year to date.
//
// It exists so a board column and a report mean the SAME thing by "last week"
// (decision D-6, DEV-1383). Every window is relative — never a pair of calendar
// dates — so a board stays correct as the calendar moves instead of needing
// someone to edit it every Monday.
//
// Conventions, once, here:
//
//   - A week runs Monday 00:00 through Sunday 24:00. "Last week" is the last
//     COMPLETE such week, never a trailing seven days.
//   - Every boundary is computed in the company's own timezone, not the
//     server's, and the range is half-open: [From, To). A record stamped
//     exactly at To belongs to the next window, so two adjacent windows can
//     never double-count a row.
//   - The window is resolved at read time. Two readers seconds apart across a
//     week or month boundary correctly see different totals — that is the point
//     of a relative window, not a bug.
//   - A company that changes its timezone re-anchors the boundaries at the next
//     read (the rule: boundaries always use the timezone configured NOW). The
//     window is never recomputed retroactively for past reads.
package timewindow

import (
	"fmt"
	"time"
)

// Kind is the relative window shape. The values are the wire form (GraphQL
// enum / stored column config), so they are stable strings.
type Kind string

const (
	// LastWeek is the last complete Monday-to-Sunday week.
	LastWeek Kind = "LAST_WEEK"
	// LastNMonths is the rolling window of Months calendar months back from now.
	LastNMonths Kind = "LAST_N_MONTHS"
	// WeekToDate starts at this week's Monday 00:00.
	WeekToDate Kind = "WEEK_TO_DATE"
	// MonthToDate starts at the first day of this month, 00:00.
	MonthToDate Kind = "MONTH_TO_DATE"
	// YearToDate starts at January 1st, 00:00.
	YearToDate Kind = "YEAR_TO_DATE"
)

// MaxMonths caps LAST_N_MONTHS. A window longer than five years is a report,
// not a board column, and would blow the page read budget.
const MaxMonths = 60

// AllKinds is the complete whitelist, in menu order.
var AllKinds = []Kind{LastWeek, LastNMonths, WeekToDate, MonthToDate, YearToDate}

func (k Kind) Valid() bool {
	for _, v := range AllKinds {
		if v == k {
			return true
		}
	}
	return false
}

// ParseKind accepts the wire form and rejects anything else. Callers must not
// coerce unknown text into a default window: a wrong window is a wrong number
// that nobody notices.
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	if !k.Valid() {
		return "", fmt.Errorf("unknown time window %q (known: %v)", s, AllKinds)
	}
	return k, nil
}

// Window is a relative window: a Kind plus, for LAST_N_MONTHS only, how many
// months back it reaches.
type Window struct {
	Kind   Kind
	Months int
}

// Months-carrying constructor for the common case.
func LastMonths(n int) Window { return Window{Kind: LastNMonths, Months: n} }

// Validate reports whether the window is well formed. It is meant to run at
// startup over every declared entry, so a bad window can never reach a query.
func (w Window) Validate() error {
	if !w.Kind.Valid() {
		return fmt.Errorf("unknown time window %q", w.Kind)
	}
	if w.Kind == LastNMonths {
		if w.Months < 1 || w.Months > MaxMonths {
			return fmt.Errorf("%s needs months between 1 and %d, got %d", LastNMonths, MaxMonths, w.Months)
		}
		return nil
	}
	if w.Months != 0 {
		return fmt.Errorf("%s takes no month count, got %d", w.Kind, w.Months)
	}
	return nil
}

// Range is the resolved half-open interval [From, To).
type Range struct {
	From time.Time
	To   time.Time
}

// Contains applies the half-open rule.
func (r Range) Contains(t time.Time) bool {
	return !t.Before(r.From) && t.Before(r.To)
}

// Resolve turns the relative window into concrete bounds in loc. now is the
// clock seam (tests pass a fixed instant); a nil loc means UTC.
func (w Window) Resolve(now time.Time, loc *time.Location) (Range, error) {
	if err := w.Validate(); err != nil {
		return Range{}, err
	}
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)

	switch w.Kind {
	case LastWeek:
		thisMonday := StartOfWeek(local, loc)
		return Range{From: thisMonday.AddDate(0, 0, -7), To: thisMonday}, nil
	case WeekToDate:
		return Range{From: StartOfWeek(local, loc), To: local}, nil
	case MonthToDate:
		return Range{From: startOfDay(local, loc).AddDate(0, 0, 1-local.Day()), To: local}, nil
	case YearToDate:
		return Range{From: time.Date(local.Year(), time.January, 1, 0, 0, 0, 0, loc), To: local}, nil
	case LastNMonths:
		return Range{From: monthsBefore(local, w.Months, loc), To: local}, nil
	}
	// Unreachable: Validate has already rejected every other kind.
	return Range{}, fmt.Errorf("unknown time window %q", w.Kind)
}

// StartOfWeek is Monday 00:00 of the week containing t, in loc. Exported
// because "the week starts on Monday" must have exactly one implementation.
//
// Report windows still run Monday-first; a caller that must follow the company's
// first-day-of-week setting (DEV-1909) uses StartOfWeekOn with
// settings.FirstDayOfWeekFor(...).Weekday().
func StartOfWeek(t time.Time, loc *time.Location) time.Time {
	return StartOfWeekOn(t, loc, time.Monday)
}

// StartOfWeekOn is firstDay 00:00 of the week containing t, in loc.
func StartOfWeekOn(t time.Time, loc *time.Location, firstDay time.Weekday) time.Time {
	local := t.In(loc)
	// Days elapsed since the week's first day, 0..6 (Go counts Sunday as 0).
	offset := (int(local.Weekday()) - int(firstDay) + 7) % 7
	return startOfDay(local, loc).AddDate(0, 0, -offset)
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// monthsBefore steps n calendar months back and CLAMPS the day into the target
// month. Go's AddDate normalises overflow (31 March minus one month would land
// on 3 March), which would silently widen the window; "three months before 31
// May" is 28/29 February, not 3 March.
func monthsBefore(t time.Time, n int, loc *time.Location) time.Time {
	local := t.In(loc)
	year, month := local.Year(), int(local.Month())-n
	for month <= 0 {
		month += 12
		year--
	}
	day := local.Day()
	if last := daysIn(year, time.Month(month), loc); day > last {
		day = last
	}
	return time.Date(year, time.Month(month), day,
		local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), loc)
}

func daysIn(year int, month time.Month, loc *time.Location) int {
	return time.Date(year, month+1, 1, 0, 0, 0, 0, loc).AddDate(0, 0, -1).Day()
}

// Label is the human wording of the window, used in a column's own label so a
// reader always sees which period they are looking at.
func (w Window) Label() string {
	switch w.Kind {
	case LastWeek:
		return "last week (Mon–Sun)"
	case WeekToDate:
		return "week to date"
	case MonthToDate:
		return "month to date"
	case YearToDate:
		return "year to date"
	case LastNMonths:
		if w.Months == 1 {
			return "last month"
		}
		return fmt.Sprintf("last %d months", w.Months)
	}
	return string(w.Kind)
}
