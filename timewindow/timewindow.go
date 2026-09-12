// Package timewindow defines the relative reporting windows the whole platform
// shares: today so far, yesterday, "last N days", "last week", "last N weeks",
// "last N months", week/month/year to date.
//
// It exists so a board column and a report mean the SAME thing by "last week"
// (decision D-6, DEV-1383). Every window is relative — never a pair of calendar
// dates — so a board stays correct as the calendar moves instead of needing
// someone to edit it every Monday.
//
// Conventions, once, here:
//
//   - A week runs Monday 00:00 through Sunday 24:00. "Last week" is the last
//     COMPLETE such week, never a trailing seven days. "Last N weeks" is the
//     same rule N times over.
//   - A day runs from local midnight to local midnight, so "yesterday" is the
//     last COMPLETE local day. The windows that end AT READ TIME ("today so
//     far", "last N days") deliberately include today's partial day — that is
//     what a dispatcher means by "the last 7 days".
//   - There are no minute or hour windows: a figure that moves while someone
//     reads it is noise, not a total.
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
	"strings"
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
	// TodaySoFar is the local day so far: today's midnight until now. It is the
	// day-sized counterpart of WEEK_TO_DATE, and like every to-date window it
	// has exactly one instance.
	TodaySoFar Kind = "TODAY_SO_FAR"
	// Yesterday is the last COMPLETE local day.
	Yesterday Kind = "YESTERDAY"
	// LastNDays is the last Days calendar days ENDING AT NOW: today so far plus
	// the Days-1 complete days before it — the way a sheet covers "the last 7
	// days". It therefore always includes a partial day.
	LastNDays Kind = "LAST_N_DAYS"
	// LastNWeeks is the last Weeks COMPLETE weeks, ending where this week
	// starts. LAST_N_WEEKS with one week is exactly LAST_WEEK; like it, it is
	// never a trailing 7xN days (the bridge week is six or eight days long).
	LastNWeeks Kind = "LAST_N_WEEKS"
)

// The counted windows are capped. A window wider than these is a report, not a
// board column, and would blow the page read budget. The caps are part of the
// refusal message, so a client is told the allowed range instead of guessing.
const (
	// MaxMonths caps LAST_N_MONTHS: longer than five years is a report.
	MaxMonths = 60
	// MaxDays caps LAST_N_DAYS: a month of days. Beyond that, ask in weeks.
	MaxDays = 31
	// MaxWeeks caps LAST_N_WEEKS: a quarter of weeks. Beyond that, ask in months.
	MaxWeeks = 12
)

// AllKinds is the complete whitelist, in menu order (shortest period first).
// Minute- and hour-sized windows are deliberately absent: a board figure that
// moves while it is read is noise, not a total.
var AllKinds = []Kind{
	TodaySoFar, Yesterday, LastNDays,
	LastWeek, LastNWeeks, LastNMonths,
	WeekToDate, MonthToDate, YearToDate,
}

// KindNames lists the whitelist for a refusal message: an unknown window is
// always answered with what IS allowed, never coerced into a default.
func KindNames() string {
	names := make([]string, len(AllKinds))
	for i, k := range AllKinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}

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
		return "", fmt.Errorf("unknown time window %q (known: %s)", s, KindNames())
	}
	return k, nil
}

// Window is a relative window: a Kind plus, for a counted kind, how far back it
// reaches. Exactly one count belongs to a window and only to the kind that
// names it (LAST_N_DAYS carries Days, never Weeks), so a stored window can
// never be read two ways.
//
// The JSON form is the wire/stored form: {"kind":"LAST_N_DAYS","days":7}.
type Window struct {
	Kind   Kind `json:"kind"`
	Days   int  `json:"days,omitempty"`
	Weeks  int  `json:"weeks,omitempty"`
	Months int  `json:"months,omitempty"`
}

// Count-carrying constructors for the common cases.
func LastDays(n int) Window   { return Window{Kind: LastNDays, Days: n} }
func LastWeeks(n int) Window  { return Window{Kind: LastNWeeks, Weeks: n} }
func LastMonths(n int) Window { return Window{Kind: LastNMonths, Months: n} }

// Validate reports whether the window is well formed. It is meant to run at
// startup over every declared entry, so a bad window can never reach a query.
func (w Window) Validate() error {
	if !w.Kind.Valid() {
		return fmt.Errorf("unknown time window %q (known: %s)", w.Kind, KindNames())
	}
	for _, c := range w.counts() {
		if w.Kind == c.kind {
			if c.got < 1 || c.got > c.max {
				return fmt.Errorf("%s needs a %s count between 1 and %d, got %d",
					c.kind, c.unit, c.max, c.got)
			}
			continue
		}
		// A count that belongs to another kind is refused rather than ignored:
		// {"kind":"YESTERDAY","days":7} means two different things to two readers.
		if c.got != 0 {
			return fmt.Errorf("%s takes no %s count, got %d", w.Kind, c.unit, c.got)
		}
	}
	return nil
}

// countSpec pairs a counted kind with the field that carries its count.
type countSpec struct {
	kind Kind
	unit string
	got  int
	max  int
}

func (w Window) counts() []countSpec {
	return []countSpec{
		{LastNDays, "day", w.Days, MaxDays},
		{LastNWeeks, "week", w.Weeks, MaxWeeks},
		{LastNMonths, "month", w.Months, MaxMonths},
	}
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
//
// Monday-first. A caller that must follow the company's first-day-of-week
// setting (DEV-1909) uses ResolveOn — or ResolveWeeks when the company changed
// the day and the bridge week is not seven days long.
func (w Window) Resolve(now time.Time, loc *time.Location) (Range, error) {
	return w.ResolveOn(now, loc, time.Monday)
}

// ResolveOn is Resolve for a company whose week starts on firstDay.
func (w Window) ResolveOn(now time.Time, loc *time.Location, firstDay time.Weekday) (Range, error) {
	if loc == nil {
		loc = time.UTC
	}
	return w.ResolveWeeks(now, loc, func(t time.Time) time.Time {
		return StartOfWeekOn(t, loc, firstDay)
	})
}

// ResolveWeeks is ResolveOn for a company whose weeks are not all one weekday
// wide. When the first day of the week is changed, one bridge week is six or
// eight days long (DEV-1909), so "last week" is NOT "seven days before this
// week" — the caller passes its own cut instead.
//
// weekStart must answer the start of the week holding t, in loc. A nil weekStart
// falls back to Monday, which is what every window did before the setting
// existed. The windows that do not involve a week ignore it.
func (w Window) ResolveWeeks(now time.Time, loc *time.Location, weekStart func(time.Time) time.Time) (Range, error) {
	if err := w.Validate(); err != nil {
		return Range{}, err
	}
	if loc == nil {
		loc = time.UTC
	}
	if weekStart == nil {
		weekStart = func(t time.Time) time.Time { return StartOfWeekOn(t, loc, time.Monday) }
	}
	local := now.In(loc)

	switch w.Kind {
	case LastWeek:
		thisStart := weekStart(local)
		// Step back ONE INSTANT and ask again — never "minus seven days". Across
		// the bridge week seven days lands mid-week.
		return Range{From: weekStart(thisStart.Add(-time.Nanosecond)), To: thisStart}, nil
	case WeekToDate:
		return Range{From: weekStart(local), To: local}, nil
	case MonthToDate:
		return Range{From: startOfDay(local, loc).AddDate(0, 0, 1-local.Day()), To: local}, nil
	case YearToDate:
		return Range{From: time.Date(local.Year(), time.January, 1, 0, 0, 0, 0, loc), To: local}, nil
	case LastNMonths:
		return Range{From: monthsBefore(local, w.Months, loc), To: local}, nil
	case TodaySoFar:
		return Range{From: startOfDay(local, loc), To: local}, nil
	case Yesterday:
		start := startOfDay(local, loc)
		// AddDate on a local midnight lands on the next local midnight, so a DST
		// day (23 or 25 hours long) is still exactly one day.
		return Range{From: start.AddDate(0, 0, -1), To: start}, nil
	case LastNDays:
		// N CALENDAR days ending at read time: today so far plus the N-1 whole
		// days before it. "Last 1 day" is therefore today so far, not yesterday.
		return Range{From: startOfDay(local, loc).AddDate(0, 0, -(w.Days - 1)), To: local}, nil
	case LastNWeeks:
		// The last N COMPLETE weeks, cut the same way LAST_WEEK is cut: step back
		// one week at a time instead of subtracting 7xN days, so a bridge week
		// (six or eight days, DEV-1909) cannot land the range mid-week.
		thisStart := weekStart(local)
		from := thisStart
		for i := 0; i < w.Weeks; i++ {
			from = weekStart(from.Add(-time.Nanosecond))
		}
		return Range{From: from, To: thisStart}, nil
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
func (w Window) Label() string { return w.LabelOn(time.Monday) }

// LabelOn is Label for a company whose week starts on firstDay: the wording of
// "last week" names the days the reader actually sees (DEV-1909).
func (w Window) LabelOn(firstDay time.Weekday) string {
	switch w.Kind {
	case LastWeek:
		last := time.Weekday((int(firstDay) + 6) % 7)
		return fmt.Sprintf("last week (%s–%s)", shortDay(firstDay), shortDay(last))
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
	case TodaySoFar:
		return "today so far"
	case Yesterday:
		return "yesterday"
	case LastNDays:
		if w.Days == 1 {
			// The same window as TODAY_SO_FAR, so it is named the same way.
			return "today so far"
		}
		return fmt.Sprintf("last %d days", w.Days)
	case LastNWeeks:
		if w.Weeks == 1 {
			return Window{Kind: LastWeek}.LabelOn(firstDay)
		}
		// "complete" is the point: this week is not in it.
		return fmt.Sprintf("last %d complete weeks", w.Weeks)
	}
	return string(w.Kind)
}

// shortDay is the three-letter weekday used in a window label.
func shortDay(d time.Weekday) string { return d.String()[:3] }
