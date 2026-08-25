package settings

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// DEV-1909 — the bridge week. Calendar arithmetic only: no cache, no timezone.
//
// Reference dates: 2026-09-06 is a Sunday, 2026-09-07 the Monday after it,
// 2026-08-31 the Monday before it.

// Monday → Sunday: the last Monday week is cut at the new Sunday, six days long.
func TestBridgeWeekIsSixDaysGoingToSunday(t *testing.T) {
	rule := NewWeekRule(FirstDayOfWeekSunday, day("2026-09-06"))

	bridge := rule.WeekOf(day("2026-09-02")) // a Wednesday inside the bridge
	require.True(t, bridge.Bridge)
	require.True(t, bridge.IsShort())
	require.Equal(t, 6, bridge.Days)
	require.Equal(t, day("2026-08-31"), bridge.Start, "Monday")
	require.Equal(t, day("2026-09-06"), bridge.EndExclusive, "ends where the first Sunday week starts")

	// The week before the bridge is a normal Monday week…
	before := rule.WeekOf(day("2026-08-26"))
	require.False(t, before.Bridge)
	require.Equal(t, 7, before.Days)
	require.Equal(t, time.Monday, before.FirstDay)

	// …and the week after it is a normal Sunday week.
	after := rule.WeekOf(day("2026-09-06"))
	require.False(t, after.Bridge)
	require.Equal(t, 7, after.Days)
	require.Equal(t, day("2026-09-06"), after.Start)
	require.Equal(t, time.Sunday, after.FirstDay)
}

// Sunday → Monday: the single leftover Sunday joins the following Monday week —
// eight days. A one-day week would charge a full weekly amount twice, a day apart.
func TestBridgeWeekIsEightDaysGoingBackToMonday(t *testing.T) {
	rule := NewWeekRule(FirstDayOfWeekMonday, day("2026-09-07"))

	bridge := rule.WeekOf(day("2026-09-06")) // the orphan Sunday
	require.True(t, bridge.Bridge)
	require.True(t, bridge.IsLong())
	require.Equal(t, 8, bridge.Days)
	require.Equal(t, day("2026-09-06"), bridge.Start)
	require.Equal(t, day("2026-09-14"), bridge.EndExclusive)

	// Every day of those eight resolves to the same week — no day is in two weeks.
	for _, d := range []string{"2026-09-06", "2026-09-07", "2026-09-10", "2026-09-13"} {
		require.Equal(t, bridge.Start, rule.WeekOf(day(d)).Start, d)
	}
	require.Equal(t, day("2026-09-14"), rule.WeekOf(day("2026-09-14")).Start, "the next week starts clean")
}

// Never a one-day week, in either direction.
func TestBridgeWeekIsNeverOneDay(t *testing.T) {
	for _, rule := range []WeekRule{
		NewWeekRule(FirstDayOfWeekSunday, day("2026-09-06")),
		NewWeekRule(FirstDayOfWeekMonday, day("2026-09-07")),
	} {
		for i := 0; i < 30; i++ {
			w := rule.WeekOf(day("2026-08-24").AddDate(0, 0, i))
			require.GreaterOrEqual(t, w.Days, 6, "week starting %s", w.Start)
			require.LessOrEqual(t, w.Days, 8, "week starting %s", w.Start)
		}
	}
}

// Walking weeks must step to the next week START: +7 days lands mid-week across
// the bridge, which is what silently double-counts or drops a day.
func TestNextWeekStartWalksOverTheBridge(t *testing.T) {
	rule := NewWeekRule(FirstDayOfWeekSunday, day("2026-09-06"))

	start := rule.WeekOf(day("2026-08-24")).Start
	got := []time.Time{start}
	for i := 0; i < 3; i++ {
		start = rule.NextWeekStart(start)
		got = append(got, start)
	}

	require.Equal(t, []time.Time{
		day("2026-08-24"), // Monday week
		day("2026-08-31"), // the 6-day bridge
		day("2026-09-06"), // first Sunday week
		day("2026-09-13"),
	}, got)
}

// Every day belongs to exactly one week: walking from a start covers the days in
// order with no gap and no overlap.
func TestWeeksPartitionTheCalendar(t *testing.T) {
	rule := NewWeekRule(FirstDayOfWeekSunday, day("2026-09-06"))

	cursor := rule.WeekOf(day("2026-08-17")).Start
	for i := 0; i < 6; i++ {
		w := rule.WeekOf(cursor)
		require.Equal(t, cursor, w.Start)
		require.Equal(t, w.Days, int(w.EndExclusive.Sub(w.Start).Hours()/24))
		cursor = w.EndExclusive
	}
}

// A company that never changed the day has no bridge at all.
func TestNoChangeMeansNoBridge(t *testing.T) {
	rule := NewWeekRule(FirstDayOfWeekMonday, time.Time{})

	w := rule.WeekOf(day("2026-09-09"))
	require.False(t, w.Bridge)
	require.Equal(t, 7, w.Days)
	require.Equal(t, day("2026-09-07"), w.Start)
}

// The cut is naive: the same calendar day gives the same week whatever location
// the caller's time carries. Time zones are out of scope by decision, not delay.
func TestWeekCutIsNotTimezoneConverted(t *testing.T) {
	rule := NewWeekRule(FirstDayOfWeekSunday, day("2026-09-06"))
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	utcMorning := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
	tokyoEvening := time.Date(2026, 9, 7, 23, 0, 0, 0, tokyo)

	require.Equal(t, rule.WeekOf(utcMorning).Start, rule.WeekOf(tokyoEvening).Start,
		"the digits typed decide the week, never a conversion")
}
