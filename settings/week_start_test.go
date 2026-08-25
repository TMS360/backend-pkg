package settings

import (
	"errors"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/timewindow"
	"github.com/TMS360/backend-pkg/utils"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// DEV-1909: the rule behind "which day starts the week that holds this date?".
// Redis-free — the two readers only fetch the two raw strings this resolves.

func day(s string) time.Time {
	t, err := time.Parse(EffectiveFromLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

// AC1: a company with nothing saved counts weeks from Monday, exactly as before.
func TestFirstDayOfWeekDefaultsToMondayWhenUnset(t *testing.T) {
	got := ResolveFirstDayOfWeek("", redis.Nil, "", redis.Nil, day("2026-09-02"))
	require.Equal(t, FirstDayOfWeekMonday, got)
}

// AC2: a Sunday company answers Sunday from the start date on, Monday before it.
func TestFirstDayOfWeekIsDateAware(t *testing.T) {
	// 2026-09-06 is a Sunday.
	const from = "2026-09-06"

	require.Equal(t, FirstDayOfWeekMonday,
		ResolveFirstDayOfWeek("sunday", nil, from, nil, day("2026-09-05")),
		"the day before the change still belongs to a Monday week")
	require.Equal(t, FirstDayOfWeekSunday,
		ResolveFirstDayOfWeek("sunday", nil, from, nil, day("2026-09-06")),
		"the start date itself already counts")
	require.Equal(t, FirstDayOfWeekSunday,
		ResolveFirstDayOfWeek("sunday", nil, from, nil, day("2026-12-31")))
}

// Edge: a start date already in the past is accepted — the change is simply
// already in force (support can set one by hand).
func TestFirstDayOfWeekAcceptsPastStartDate(t *testing.T) {
	got := ResolveFirstDayOfWeek("sunday", nil, "2020-01-05", nil, day("2026-09-02"))
	require.Equal(t, FirstDayOfWeekSunday, got)
}

// Edge: the stored start date is not itself a first day (Sunday setting with a
// Wednesday date) — read moves forward to the next valid day rather than cutting
// one ragged week. The save path refuses to store this in the first place.
func TestFirstDayOfWeekMovesAnInvalidStartDateForward(t *testing.T) {
	// 2026-09-09 is a Wednesday; the next Sunday is 2026-09-13.
	const wednesday = "2026-09-09"

	require.Equal(t, FirstDayOfWeekMonday,
		ResolveFirstDayOfWeek("sunday", nil, wednesday, nil, day("2026-09-12")),
		"still the old day right up to the corrected start")
	require.Equal(t, FirstDayOfWeekSunday,
		ResolveFirstDayOfWeek("sunday", nil, wednesday, nil, day("2026-09-13")))
}

// AC5: a bad or missing value answers Monday — never half of the change.
func TestFirstDayOfWeekFallsBackToMondayOnBadInput(t *testing.T) {
	ref := day("2026-09-20")
	cases := map[string]FirstDayOfWeek{
		"unknown word":       ResolveFirstDayOfWeek("wednesday", nil, "2026-09-06", nil, ref),
		"missing start date": ResolveFirstDayOfWeek("sunday", nil, "", redis.Nil, ref),
		"unparsable date":    ResolveFirstDayOfWeek("sunday", nil, "06.09.2026", nil, ref),
		"cache down on day":  ResolveFirstDayOfWeek("", errors.New("dial tcp: refused"), "", nil, ref),
		"cache down on date": ResolveFirstDayOfWeek("sunday", nil, "", errors.New("dial tcp: refused"), ref),
	}
	for name, got := range cases {
		require.Equal(t, FirstDayOfWeekMonday, got, name)
	}
}

// A Monday company needs no start date: Monday IS the old day, so there is
// nothing to phase in.
func TestFirstDayOfWeekMondayNeedsNoStartDate(t *testing.T) {
	got := ResolveFirstDayOfWeek("Monday ", nil, "", redis.Nil, day("2026-09-20"))
	require.Equal(t, FirstDayOfWeekMonday, got, "value is trimmed and case-insensitive")
}

func TestFirstDayOfWeekWeekdayMapping(t *testing.T) {
	require.Equal(t, time.Monday, FirstDayOfWeekMonday.Weekday())
	require.Equal(t, time.Sunday, FirstDayOfWeekSunday.Weekday())
	require.Equal(t, time.Monday, FirstDayOfWeek("garbage").Weekday(), "never panics, answers the default")

	_, ok := ParseFirstDayOfWeek("SUNDAY")
	require.True(t, ok)
	_, ok = ParseFirstDayOfWeek("tuesday")
	require.False(t, ok)
}

// ── the week helpers the other services will call ────────────────────────

// Sunday 2026-09-06 through Saturday 2026-09-12 is ONE week for a Sunday
// company, and two different weeks for a Monday one.
func TestGetWeekStartOnHonoursTheFirstDay(t *testing.T) {
	sunday := day("2026-09-06")
	wednesday := day("2026-09-09")

	require.Equal(t, sunday, utils.GetWeekStartOn(wednesday, time.Sunday))
	require.Equal(t, day("2026-09-07"), utils.GetWeekStartOn(wednesday, time.Monday))
	require.Equal(t, day("2026-09-06"), utils.GetWeekStartOn(sunday, time.Sunday),
		"the first day is the start of its own week, not of the previous one")
	require.Equal(t, day("2026-08-31"), utils.GetWeekStartOn(sunday, time.Monday))

	// The Monday helpers are untouched by the new argument.
	require.Equal(t, utils.GetWeekStart(wednesday), utils.GetWeekStartOn(wednesday, time.Monday))
	require.Equal(t, utils.GetWeekEnd(wednesday), utils.GetWeekEndOn(wednesday, time.Monday))
}

func TestGetWeekEndOnClosesSixDaysLater(t *testing.T) {
	start, end := utils.GetWeekRangeOn(day("2026-09-09"), time.Sunday)

	require.Equal(t, day("2026-09-06"), start)
	require.Equal(t, time.Saturday, end.Weekday())
	require.Equal(t, day("2026-09-12").Add(24*time.Hour-time.Nanosecond), end)
}

func TestStartOfWeekOnUsesTheGivenLocation(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	// Sunday 2026-09-06 03:00 UTC is still Saturday 22:00 in Chicago (UTC-5).
	ref := time.Date(2026, 9, 6, 3, 0, 0, 0, time.UTC)

	got := timewindow.StartOfWeekOn(ref, loc, time.Sunday)
	require.Equal(t, time.Sunday, got.Weekday())
	require.Equal(t, 2026, got.Year())
	require.Equal(t, 30, got.Day(), "local Saturday belongs to the week that opened on 30 August")

	require.Equal(t, timewindow.StartOfWeek(ref, loc), timewindow.StartOfWeekOn(ref, loc, time.Monday))
}
