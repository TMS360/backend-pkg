package tests

import (
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/timewindow"

	"github.com/stretchr/testify/require"
)

// Day-sized relative windows (DEV-2197): today so far, yesterday, last N days
// and last N complete weeks, next to the week/month windows DEV-1383 shipped.
//
// Every assertion is about the SHAPE of the window — where it starts, where it
// ends, and what it refuses — because a window that is one day off produces a
// total that looks completely plausible.

// A Wednesday 09:30 in Chicago, the company's own timezone.
var (
	twChicago  = mustLoad("America/Chicago")
	twNow      = time.Date(2026, 8, 19, 9, 30, 0, 0, twChicago)
	twMidnight = time.Date(2026, 8, 19, 0, 0, 0, 0, twChicago)
)

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func resolve(t *testing.T, w timewindow.Window) timewindow.Range {
	t.Helper()
	r, err := w.Resolve(twNow, twChicago)
	require.NoError(t, err)
	return r
}

func TestTimeWindow_TodaySoFarIsMidnightUntilNow(t *testing.T) {
	r := resolve(t, timewindow.Window{Kind: timewindow.TodaySoFar})
	require.Equal(t, twMidnight, r.From)
	require.Equal(t, twNow.In(twChicago), r.To)
	require.Equal(t, "today so far", timewindow.Window{Kind: timewindow.TodaySoFar}.Label())
}

func TestTimeWindow_YesterdayIsTheLastCompleteDay(t *testing.T) {
	r := resolve(t, timewindow.Window{Kind: timewindow.Yesterday})
	require.Equal(t, time.Date(2026, 8, 18, 0, 0, 0, 0, twChicago), r.From)
	// Half-open: it ends exactly where today starts, so a stamp at midnight
	// belongs to today and the two windows can never double-count it.
	require.Equal(t, twMidnight, r.To)
	require.False(t, r.Contains(twMidnight))
	require.True(t, r.Contains(time.Date(2026, 8, 18, 23, 59, 59, 0, twChicago)))
}

func TestTimeWindow_LastNDaysIsNCalendarDaysEndingNow(t *testing.T) {
	r := resolve(t, timewindow.LastDays(7))
	// Today so far plus the six whole days before it.
	require.Equal(t, time.Date(2026, 8, 13, 0, 0, 0, 0, twChicago), r.From)
	require.Equal(t, twNow.In(twChicago), r.To)
	require.Equal(t, "last 7 days", timewindow.LastDays(7).Label())

	// "Last 1 day" is today so far — the same window, named the same way.
	one := resolve(t, timewindow.LastDays(1))
	require.Equal(t, resolve(t, timewindow.Window{Kind: timewindow.TodaySoFar}), one)
	require.Equal(t, "today so far", timewindow.LastDays(1).Label())
}

func TestTimeWindow_LastNDaysUsesTheCompanyMidnight(t *testing.T) {
	// The same instant read by a Chicago company and a UTC one: 09:30 Chicago is
	// 14:30 UTC on the same date here, but the window STARTS at a different
	// instant because midnight is a different instant.
	chicago := resolve(t, timewindow.LastDays(7))
	utc, err := timewindow.LastDays(7).Resolve(twNow, time.UTC)
	require.NoError(t, err)
	require.NotEqual(t, chicago.From, utc.From)
	require.Equal(t, 0, utc.From.Hour())
	require.Equal(t, time.UTC, utc.From.Location())
}

func TestTimeWindow_DayWindowsSurviveDaylightSaving(t *testing.T) {
	ny := mustLoad("America/New_York")
	// 8 March 2026 is a 23-hour day (clocks go forward). "Yesterday" read on the
	// 9th must still be that whole day, midnight to midnight — not 24 hours.
	now := time.Date(2026, 3, 9, 10, 0, 0, 0, ny)
	r, err := timewindow.Window{Kind: timewindow.Yesterday}.Resolve(now, ny)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 8, 0, 0, 0, 0, ny), r.From)
	require.Equal(t, time.Date(2026, 3, 9, 0, 0, 0, 0, ny), r.To)
	require.Equal(t, 23*time.Hour, r.To.Sub(r.From))
}

func TestTimeWindow_LastNWeeksIsNCompleteWeeks(t *testing.T) {
	r := resolve(t, timewindow.LastWeeks(3))
	// Monday-first: this week starts 17 Aug, so three complete weeks are
	// 27 July 00:00 through 17 Aug 00:00.
	require.Equal(t, time.Date(2026, 7, 27, 0, 0, 0, 0, twChicago), r.From)
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, twChicago), r.To)

	// One week is exactly LAST_WEEK — same window, same wording.
	one := resolve(t, timewindow.LastWeeks(1))
	require.Equal(t, resolve(t, timewindow.Window{Kind: timewindow.LastWeek}), one)
	require.Equal(t, timewindow.Window{Kind: timewindow.LastWeek}.Label(), timewindow.LastWeeks(1).Label())
	require.Equal(t, "last 3 complete weeks", timewindow.LastWeeks(3).Label())
}

func TestTimeWindow_LastNWeeksFollowsTheCompanyFirstDay(t *testing.T) {
	// A Sunday-first company: the same Wednesday sits in the week that started
	// Sunday 16 Aug, so two complete weeks run 2 Aug through 16 Aug.
	r, err := timewindow.LastWeeks(2).ResolveOn(twNow, twChicago, time.Sunday)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 2, 0, 0, 0, 0, twChicago), r.From)
	require.Equal(t, time.Date(2026, 8, 16, 0, 0, 0, 0, twChicago), r.To)
}

func TestTimeWindow_CountedWindowsRefuseCountsOutsideTheRange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		window timewindow.Window
		says   string
	}{
		{"zero days", timewindow.LastDays(0), "between 1 and 31"},
		{"32 days", timewindow.LastDays(32), "between 1 and 31"},
		{"zero weeks", timewindow.LastWeeks(0), "between 1 and 12"},
		{"13 weeks", timewindow.LastWeeks(13), "between 1 and 12"},
		{"zero months", timewindow.LastMonths(0), "between 1 and 60"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.window.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.says)
			_, err = tc.window.Resolve(twNow, twChicago)
			require.Error(t, err, "a window that does not validate must never resolve")
		})
	}
}

func TestTimeWindow_ACountBelongsToOneKindOnly(t *testing.T) {
	err := timewindow.Window{Kind: timewindow.Yesterday, Days: 7}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "takes no day count")

	err = timewindow.Window{Kind: timewindow.LastNDays, Days: 7, Weeks: 2}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "takes no week count")
}

func TestTimeWindow_UnknownPeriodNamesTheAllowedOnes(t *testing.T) {
	// Minute and hour windows are not a thing here, and the refusal lists what
	// IS allowed instead of quietly falling back to last week.
	for _, name := range []string{"LAST_15_MINUTES", "LAST_N_HOURS", "today"} {
		_, err := timewindow.ParseKind(name)
		require.Error(t, err)
		require.Contains(t, err.Error(), "TODAY_SO_FAR")
		require.Contains(t, err.Error(), "LAST_N_DAYS")
		require.Contains(t, err.Error(), "LAST_WEEK")
	}
	err := timewindow.Window{Kind: "LAST_N_HOURS"}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "LAST_N_WEEKS")
}

func TestTimeWindow_ExistingWindowsAreUnchanged(t *testing.T) {
	// The DEV-1383 windows must resolve exactly as before: adding kinds is
	// additive, and a column stored then keeps its number.
	week := resolve(t, timewindow.Window{Kind: timewindow.LastWeek})
	require.Equal(t, time.Date(2026, 8, 10, 0, 0, 0, 0, twChicago), week.From)
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, twChicago), week.To)

	ytd := resolve(t, timewindow.Window{Kind: timewindow.YearToDate})
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, twChicago), ytd.From)
	require.Equal(t, twNow.In(twChicago), ytd.To)

	months := resolve(t, timewindow.LastMonths(3))
	require.Equal(t, time.Date(2026, 5, 19, 9, 30, 0, 0, twChicago), months.From)
}
