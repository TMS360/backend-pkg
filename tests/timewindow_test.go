package tests

import (
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/timewindow"
	"github.com/stretchr/testify/require"
)

// DEV-1383. One definition of "last week" for boards and reports (D-6): Monday
// to Sunday, in the COMPANY's timezone, half-open so adjacent windows never
// double-count.

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func TestWindowLastWeekIsTheLastCompleteMondayToSunday(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	// Wednesday 2026-08-19, 09:30 local.
	now := time.Date(2026, 8, 19, 9, 30, 0, 0, loc)

	got, err := timewindow.Window{Kind: timewindow.LastWeek}.Resolve(now, loc)
	require.NoError(t, err)

	require.Equal(t, time.Date(2026, 8, 10, 0, 0, 0, 0, loc), got.From, "previous Monday 00:00")
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, loc), got.To, "this Monday 00:00, exclusive")
	require.Equal(t, time.Monday, got.From.Weekday())

	// Half-open: Sunday 23:59:59 is in, this Monday 00:00:00 is out.
	require.True(t, got.Contains(time.Date(2026, 8, 16, 23, 59, 59, 0, loc)))
	require.False(t, got.Contains(got.To))
	require.False(t, got.Contains(time.Date(2026, 8, 9, 23, 59, 59, 0, loc)))
}

// A Monday read must not collapse to an empty window: on Monday, "last week" is
// the week that just ended.
func TestWindowLastWeekOnAMondayReadsTheWeekJustEnded(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	now := time.Date(2026, 8, 17, 0, 5, 0, 0, loc) // Monday, five past midnight

	got, err := timewindow.Window{Kind: timewindow.LastWeek}.Resolve(now, loc)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 10, 0, 0, 0, 0, loc), got.From)
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, loc), got.To)
}

// AC2: the boundary is the COMPANY's midnight, not the server's. At the same
// instant two companies on opposite sides of the dateline sit in different
// weeks, so their "last week" windows are a whole week apart.
func TestWindowBoundaryFollowsTheCompanyTimezone(t *testing.T) {
	tashkent := mustLoc(t, "Asia/Tashkent")  // UTC+5
	chicago := mustLoc(t, "America/Chicago") // UTC-5

	// 2026-08-17T02:00Z — already Monday 07:00 in Tashkent, still Sunday 21:00
	// in Chicago.
	instant := time.Date(2026, 8, 17, 2, 0, 0, 0, time.UTC)

	tw, err := timewindow.Window{Kind: timewindow.LastWeek}.Resolve(instant, tashkent)
	require.NoError(t, err)
	cw, err := timewindow.Window{Kind: timewindow.LastWeek}.Resolve(instant, chicago)
	require.NoError(t, err)

	// Tashkent has rolled into a new week: last week is 10–16 August.
	require.Equal(t, time.Date(2026, 8, 10, 0, 0, 0, 0, tashkent), tw.From)
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, tashkent), tw.To)
	// Chicago is still inside the week of 10–16, so its last COMPLETE week is
	// the one before: 3–9 August.
	require.Equal(t, time.Date(2026, 8, 3, 0, 0, 0, 0, chicago), cw.From)
	require.Equal(t, time.Date(2026, 8, 10, 0, 0, 0, 0, chicago), cw.To)

	// One trip, stamped mid-week: counted for the Tashkent company, not for the
	// Chicago one — the reason a rollup must be resolved per company timezone.
	trip := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	require.True(t, tw.Contains(trip))
	require.False(t, cw.Contains(trip))

	// The Tashkent boundary is its own midnight (2026-08-16T19:00Z), to the
	// second: one minute earlier is inside the window, the boundary itself is not.
	require.True(t, tw.Contains(time.Date(2026, 8, 16, 18, 59, 0, 0, time.UTC)))
	require.False(t, tw.Contains(time.Date(2026, 8, 16, 19, 0, 0, 0, time.UTC)))
}

func TestWindowToDateVariants(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	now := time.Date(2026, 8, 19, 9, 30, 0, 0, loc) // Wednesday

	wtd, err := timewindow.Window{Kind: timewindow.WeekToDate}.Resolve(now, loc)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, loc), wtd.From)
	require.Equal(t, now, wtd.To)

	mtd, err := timewindow.Window{Kind: timewindow.MonthToDate}.Resolve(now, loc)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, loc), mtd.From)

	ytd, err := timewindow.Window{Kind: timewindow.YearToDate}.Resolve(now, loc)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, loc), ytd.From)
	require.Equal(t, now, ytd.To)
}

// Go's AddDate normalises overflow (31 May minus 3 months would land on 3 March
// via 31 February); the window must clamp into the shorter month instead.
func TestWindowLastNMonthsClampsShortMonths(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, loc)

	got, err := timewindow.LastMonths(3).Resolve(now, loc)
	require.NoError(t, err)
	require.Equal(t, time.February, got.From.Month())
	require.Equal(t, 28, got.From.Day(), "2026 is not a leap year")
	require.Equal(t, now, got.To)

	leap, err := timewindow.LastMonths(1).Resolve(time.Date(2028, 3, 31, 8, 0, 0, 0, loc), loc)
	require.NoError(t, err)
	require.Equal(t, time.February, leap.From.Month())
	require.Equal(t, 29, leap.From.Day(), "2028 is a leap year")
}

func TestWindowLastNMonthsCrossesTheYear(t *testing.T) {
	got, err := timewindow.LastMonths(3).Resolve(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), time.UTC)
	require.NoError(t, err)
	require.Equal(t, 2025, got.From.Year())
	require.Equal(t, time.November, got.From.Month())
}

func TestWindowValidationRejectsBadShapes(t *testing.T) {
	require.Error(t, timewindow.Window{Kind: "LAST_FORTNIGHT"}.Validate())
	require.Error(t, timewindow.Window{Kind: timewindow.LastNMonths}.Validate(), "months required")
	require.Error(t, timewindow.LastMonths(0).Validate())
	require.Error(t, timewindow.LastMonths(timewindow.MaxMonths+1).Validate())
	require.Error(t, timewindow.Window{Kind: timewindow.YearToDate, Months: 3}.Validate(), "months must not be set")

	require.NoError(t, timewindow.LastMonths(3).Validate())
	require.NoError(t, timewindow.Window{Kind: timewindow.LastWeek}.Validate())

	_, err := timewindow.ParseKind("YEAR_TO_DATE")
	require.NoError(t, err)
	_, err = timewindow.ParseKind("year_to_date")
	require.Error(t, err, "no silent coercion into a default window")

	_, err = timewindow.Window{Kind: "NOPE"}.Resolve(time.Now(), time.UTC)
	require.Error(t, err)
}

// The label is what a column header repeats to the reader, so the period is
// never implicit.
func TestWindowLabels(t *testing.T) {
	require.Equal(t, "last week (Mon–Sun)", timewindow.Window{Kind: timewindow.LastWeek}.Label())
	require.Equal(t, "year to date", timewindow.Window{Kind: timewindow.YearToDate}.Label())
	require.Equal(t, "last 3 months", timewindow.LastMonths(3).Label())
	require.Equal(t, "last month", timewindow.LastMonths(1).Label())
}

// A nil location must not panic: it means UTC.
func TestWindowNilLocationIsUTC(t *testing.T) {
	got, err := timewindow.Window{Kind: timewindow.YearToDate}.Resolve(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC), nil)
	require.NoError(t, err)
	require.Equal(t, time.UTC, got.From.Location())
	require.Equal(t, 1, got.From.Day())
}
