package tests

import (
	"context"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/companytz"
	"github.com/go-redis/redis/v8"

	"github.com/stretchr/testify/suite"
)

// CompanyTimezoneSuite covers DEV-2095: one shared answer to "what time is it for
// this tenant", and one shared conversion into the digits the fleet's naive
// timestamp columns hold.
//
// The reader half needs Redis, so what is pinned here is the part that carries
// the semantics — the conversion, the fallbacks, and the promise that a service
// with no cache configured degrades quietly instead of panicking.
type CompanyTimezoneSuite struct {
	suite.Suite
}

func TestCompanyTimezoneSuite(t *testing.T) {
	suite.Run(t, new(CompanyTimezoneSuite))
}

const chicago = "America/Chicago"

// naive is how a wall-clock column stores a reading: the digits, labeled UTC.
func naive(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.UTC)
}

// instantAt is the absolute moment at which a person in tz reads those digits.
func (s *CompanyTimezoneSuite) instantAt(tz string, y int, m time.Month, d, h, min int) time.Time {
	loc, err := time.LoadLocation(tz)
	s.Require().NoError(err)
	return time.Date(y, m, d, h, min, 0, 0, loc)
}

// The DEV-2076 case, now guarded in the shared package: a naive column and the
// tenant clock line up, while the same moment in UTC does not.
func (s *CompanyTimezoneSuite) TestWallClockMatchesNaiveColumns() {
	appointment := naive(2026, time.September, 4, 16, 0)
	instant := s.instantAt(chicago, 2026, time.September, 4, 14, 17)

	now, ok := companytz.WallClock(instant, chicago)
	s.Require().True(ok)

	s.True(now.Before(appointment), "2:17 PM has not reached a 4:00 PM appointment")
	s.True(instant.UTC().After(appointment), "the UTC clock has — this is the bug being prevented")
}

// An IANA name, never a fixed offset: the same tenant is UTC-6 in winter and
// UTC-5 in summer, and the digits stay the digits either way.
func (s *CompanyTimezoneSuite) TestWallClockHoldsAcrossDST() {
	for _, c := range []struct {
		name  string
		month time.Month
	}{
		{"CST (winter)", time.January},
		{"CDT (summer)", time.July},
	} {
		s.Run(c.name, func() {
			instant := s.instantAt(chicago, 2026, c.month, 15, 16, 0)

			now, ok := companytz.WallClock(instant, chicago)
			s.Require().True(ok)

			s.Equal(naive(2026, c.month, 15, 16, 0), now)
			s.NotEqual(now, instant.UTC())
		})
	}
}

// DEV-4604 AC1: an unset timezone is Central, not UTC, and is not a
// misconfiguration — it must not generate a warning.
func (s *CompanyTimezoneSuite) TestUnsetIsCentralAndNotAnError() {
	s.Equal(chicago, companytz.Default)

	instant := s.instantAt(chicago, 2026, time.September, 4, 14, 17)
	for _, tz := range []string{"", companytz.Default} {
		now, ok := companytz.WallClock(instant, tz)
		s.True(ok, "an unset timezone is a default, not a misconfiguration")
		s.Equal(naive(2026, time.September, 4, 14, 17), now, "tz=%q", tz)

		loc, ok := companytz.ParseLocation(tz)
		s.True(ok)
		s.Equal(chicago, loc.String(), "tz=%q", tz)
	}
}

// DEV-4604 AC2: a company that chose UTC on purpose still gets UTC.
func (s *CompanyTimezoneSuite) TestExplicitUTCStaysUTC() {
	instant := time.Date(2026, time.September, 4, 19, 17, 0, 0, time.UTC)

	now, ok := companytz.WallClock(instant, "UTC")
	s.True(ok)
	s.Equal(instant, now)

	loc, ok := companytz.ParseLocation("UTC")
	s.True(ok)
	s.Equal("UTC", loc.String())
}

// DEV-4604 AC3: a company that chose another zone still gets that zone.
func (s *CompanyTimezoneSuite) TestOtherZoneIsKept() {
	instant := s.instantAt("America/New_York", 2026, time.September, 4, 15, 17)

	now, ok := companytz.WallClock(instant, "America/New_York")
	s.True(ok)
	s.Equal(naive(2026, time.September, 4, 15, 17), now)

	loc, ok := companytz.ParseLocation("America/New_York")
	s.True(ok)
	s.Equal("America/New_York", loc.String())
}

// DEV-4604 AC4: a stored zone that does not resolve falls back to Central, and
// stays distinguishable from unset (ok=false) so the caller still logs it.
func (s *CompanyTimezoneSuite) TestUnknownZoneFallsBackToCentralAndReportsIt() {
	instant := s.instantAt(chicago, 2026, time.September, 4, 14, 17)

	now, ok := companytz.WallClock(instant, "Mars/Olympus_Mons")
	s.False(ok)
	s.Equal(naive(2026, time.September, 4, 14, 17), now)

	loc, ok := companytz.ParseLocation("Mars/Olympus_Mons")
	s.False(ok)
	s.Equal(chicago, loc.String())
}

// ParseLocation never hands back nil, and WallClockIn tolerates one anyway —
// this is the semantic the two backend-workspaces resolvers disagreed on.
func (s *CompanyTimezoneSuite) TestLocationIsNeverNil() {
	for _, tz := range []string{"", companytz.Default, chicago, "Not/AZone"} {
		loc, _ := companytz.ParseLocation(tz)
		s.NotNil(loc, "tz=%q", tz)
	}

	instant := time.Date(2026, time.September, 4, 19, 17, 0, 0, time.UTC)
	s.Equal(instant, companytz.WallClockIn(instant, nil))
}

// The embedded IANA database means a slim image with no zoneinfo directory still
// resolves real zones instead of silently flattening every tenant to UTC.
func (s *CompanyTimezoneSuite) TestZoneDatabaseIsAvailable() {
	for _, tz := range []string{chicago, "America/New_York", "Asia/Tashkent", "Europe/Moscow"} {
		_, ok := companytz.ParseLocation(tz)
		s.True(ok, "tz=%q must resolve from the embedded tzdata", tz)
	}
}

// No cache configured (unit tests, a service that never called cache.Init) must
// degrade to the default rather than panic on a nil client — both variants.
func (s *CompanyTimezoneSuite) TestNoCacheDegradesQuietly() {
	ctx := context.Background()

	s.Equal(chicago, companytz.Timezone(ctx))
	s.Equal(chicago, companytz.TimezoneForCompany(ctx, "8b1f0f3e-0000-4000-8000-000000000000"))
	s.Equal(chicago, companytz.Location(ctx).String())
	s.Equal(chicago, companytz.LocationForCompany(ctx, "8b1f0f3e-0000-4000-8000-000000000000").String())
	central, _ := companytz.WallClock(time.Now(), chicago)
	s.WithinDuration(central, companytz.Now(ctx), time.Minute)
	s.WithinDuration(central, companytz.NowForCompany(ctx, "8b1f0f3e-0000-4000-8000-000000000000"), time.Minute)
}

// DEV-4604 edge case: the cache is configured but unreachable. The read fails,
// and the answer is Central, not UTC, without a panic.
func (s *CompanyTimezoneSuite) TestUnreachableCacheAnswersCentral() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
	cache.Init(rdb)
	s.T().Cleanup(func() {
		cache.Init(nil)
		_ = rdb.Close()
	})

	ctx := context.Background()
	s.Equal(chicago, companytz.TimezoneForCompany(ctx, "8b1f0f3e-0000-4000-8000-000000000000"))
	s.Equal(chicago, companytz.LocationForCompany(ctx, "8b1f0f3e-0000-4000-8000-000000000000").String())
}

// An empty company id cannot address a tenant key; answering Default is the only
// honest result and must not read an unprefixed key.
func (s *CompanyTimezoneSuite) TestForCompanyRejectsEmptyID() {
	s.Equal(companytz.Default, companytz.TimezoneForCompany(context.Background(), ""))
}
