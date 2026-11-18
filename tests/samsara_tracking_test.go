package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/settings"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/suite"
)

// SamsaraTrackingSuite covers DEV-2570: Samsara asset tracking is never on for a
// company without an active Samsara key, whatever the switch says. With a key,
// the switch keeps its old meaning (unsaved = on, saved "false" = off).
type SamsaraTrackingSuite struct {
	suite.Suite
}

func TestSamsaraTrackingSuite(t *testing.T) {
	suite.Run(t, new(SamsaraTrackingSuite))
}

var (
	samsaraKey    = string(enums.CompanySettingsIntegrationKeySamsaraAPIKey)
	trackingKey   = string(enums.CompanySettingsGeneralKeySamsaraAssetTrackingEnabled)
	errRedisBlip  = errors.New("dial tcp: i/o timeout")
	activeKeyOnly = map[string]string{samsaraKey: "samsara_live_abc"}
)

// fakeCache is the company's cached settings: a missing entry reads as redis.Nil,
// exactly like a key tms-auth never cached (or evicted on deactivation).
type fakeCache struct {
	values map[string]string
	fail   map[string]error
}

func (c fakeCache) read(key string, dest *string) error {
	if err, ok := c.fail[key]; ok {
		return err
	}
	v, ok := c.values[key]
	if !ok {
		return redis.Nil
	}
	*dest = v
	return nil
}

func (s *SamsaraTrackingSuite) on(values map[string]string) bool {
	return settings.SamsaraAssetTrackingOnFrom(fakeCache{values: values}.read)
}

// AC1: no key, switch never saved — the state every new company starts in.
func (s *SamsaraTrackingSuite) TestNoKey_Unsaved_Off() {
	s.False(s.on(map[string]string{}))
}

// Edge: saved "on" in the past but no key now — off, and the saved value is
// only read, never rewritten, so it applies again once a key is entered.
func (s *SamsaraTrackingSuite) TestNoKey_SavedOn_Off() {
	values := map[string]string{trackingKey: "true"}
	s.False(s.on(values))
	s.Equal("true", values[trackingKey])

	values[samsaraKey] = "samsara_live_abc"
	s.True(s.on(values))
}

// AC4: active key, switch never saved — on, same as before.
func (s *SamsaraTrackingSuite) TestActiveKey_Unsaved_On() {
	s.True(s.on(activeKeyOnly))
}

// AC5: active key, switch saved "off" — stays off.
func (s *SamsaraTrackingSuite) TestActiveKey_SavedOff_Off() {
	s.False(s.on(map[string]string{samsaraKey: "samsara_live_abc", trackingKey: "false"}))
}

func (s *SamsaraTrackingSuite) TestActiveKey_SavedOn_On() {
	s.True(s.on(map[string]string{samsaraKey: "samsara_live_abc", trackingKey: "true"}))
}

// AC6 + edge "disabled key": tms-auth evicts samsara_api_key from the cache on
// disable/delete, so the very next read is off; entering the key again writes it
// back and tracking is on again.
func (s *SamsaraTrackingSuite) TestKeyDisabledThenReEntered() {
	values := map[string]string{samsaraKey: "samsara_live_abc"}
	s.True(s.on(values))

	delete(values, samsaraKey)
	s.False(s.on(values), "key disabled or deleted: off from the next read")

	values[samsaraKey] = "samsara_live_new"
	s.True(s.on(values), "key entered again: back on")
}

// A key saved blank is not a key (provider.fetchAPIKey rejects it the same way).
func (s *SamsaraTrackingSuite) TestBlankKey_Off() {
	s.False(s.on(map[string]string{samsaraKey: "  "}))
}

// Edge: cache read fails — off, on either read.
func (s *SamsaraTrackingSuite) TestCacheReadFails_Off() {
	s.False(settings.SamsaraAssetTrackingOnFrom(fakeCache{
		values: map[string]string{},
		fail:   map[string]error{samsaraKey: errRedisBlip},
	}.read), "api key read failed")

	s.False(settings.SamsaraAssetTrackingOnFrom(fakeCache{
		values: activeKeyOnly,
		fail:   map[string]error{trackingKey: errRedisBlip},
	}.read), "switch read failed")
}

// The actor-less reader (gRPC GetDeadheadOrigin, asked by asset tracking) goes
// through the same rule: an unreachable Redis reads off, never on.
func (s *SamsaraTrackingSuite) TestForCompany_UnreachableCache_Off() {
	prev := cache.Client()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	cache.Init(rdb)
	defer func() {
		cache.Init(prev)
		_ = rdb.Close()
	}()

	s.False(settings.SamsaraAssetTrackingOnForCompany(context.Background(), "company-1"))
}
