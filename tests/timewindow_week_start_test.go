package tests

import (
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/settings"
	"github.com/TMS360/backend-pkg/timewindow"
	"github.com/stretchr/testify/require"
)

// DEV-1911. A report window and a board window must answer the same dates for
// the same company, and both must follow the company's first day of the week
// (DEV-1909) instead of assuming Monday.

func TestResolveOnSundayCompany(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	// Wednesday 2026-09-09.
	now := time.Date(2026, 9, 9, 15, 4, 0, 0, loc)

	wtd, err := timewindow.Window{Kind: timewindow.WeekToDate}.ResolveOn(now, loc, time.Sunday)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, loc), wtd.From)
	require.Equal(t, now, wtd.To)

	lw, err := timewindow.Window{Kind: timewindow.LastWeek}.ResolveOn(now, loc, time.Sunday)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 30, 0, 0, 0, 0, loc), lw.From)
	require.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, loc), lw.To)
}

func TestResolveOnMondayIsUnchanged(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	now := time.Date(2026, 9, 9, 15, 4, 0, 0, loc)

	for _, w := range []timewindow.Window{
		{Kind: timewindow.LastWeek},
		{Kind: timewindow.WeekToDate},
		{Kind: timewindow.MonthToDate},
		{Kind: timewindow.YearToDate},
		timewindow.LastMonths(3),
	} {
		old, err := w.Resolve(now, loc)
		require.NoError(t, err)
		on, err := w.ResolveOn(now, loc, time.Monday)
		require.NoError(t, err)
		require.Equal(t, old, on, "%s changed for a Monday company", w.Kind)
	}
}

// The bridge week is six or eight days long, so "last week" cannot be "minus
// seven days" — it is "the week before this one", asked of the rule itself.
func TestResolveWeeksAcrossTheBridgeWeek(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	// Monday -> Sunday from Sunday 2026-09-06: the bridge week is Mon 08-31 .. Sat 09-05 (6 days).
	rule := settings.NewWeekRule(settings.FirstDayOfWeekSunday, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC))
	cut := func(ts time.Time) time.Time { return rule.WeekStartIn(ts, loc) }

	// Standing inside the first new Sunday week, "last week" is the 6-day bridge.
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, loc)
	lw, err := timewindow.Window{Kind: timewindow.LastWeek}.ResolveWeeks(now, loc, cut)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 31, 0, 0, 0, 0, loc), lw.From)
	require.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, loc), lw.To)
	require.Equal(t, 6*24*time.Hour, lw.To.Sub(lw.From))

	// A plain firstDay cut would have answered a 7-day window starting 08-30.
	naive, err := timewindow.Window{Kind: timewindow.LastWeek}.ResolveOn(now, loc, time.Sunday)
	require.NoError(t, err)
	require.NotEqual(t, naive.From, lw.From)

	// A week that started before the change still reads as a Monday week.
	before := time.Date(2026, 8, 26, 8, 0, 0, 0, loc)
	wtd, err := timewindow.Window{Kind: timewindow.WeekToDate}.ResolveWeeks(before, loc, cut)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 24, 0, 0, 0, 0, loc), wtd.From)
}

func TestWeekStartInKeepsTheCalendarDay(t *testing.T) {
	loc := mustLoc(t, "America/Chicago")
	rule := settings.NewWeekRule(settings.FirstDayOfWeekSunday, time.Time{})

	// Late evening in Chicago is already the next UTC day; the week must still be
	// cut on the day the user sees.
	ts := time.Date(2026, 9, 12, 23, 30, 0, 0, loc)
	require.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, loc), rule.WeekStartIn(ts, loc))
}

func TestLabelOnNamesTheCompanysDays(t *testing.T) {
	w := timewindow.Window{Kind: timewindow.LastWeek}
	require.Equal(t, "last week (Mon–Sun)", w.Label())
	require.Equal(t, "last week (Mon–Sun)", w.LabelOn(time.Monday))
	require.Equal(t, "last week (Sun–Sat)", w.LabelOn(time.Sunday))
	require.Equal(t, "week to date", timewindow.Window{Kind: timewindow.WeekToDate}.LabelOn(time.Sunday))
}
