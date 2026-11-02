package tests

import (
	"errors"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/settings"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

// DEV-3675: the "lock load notes" switch is off unless a saved "true" says
// otherwise; a missing or unreadable cache entry is off.
func TestLoadNotesLockOn(t *testing.T) {
	key := string(enums.CompanySettingsGeneralKeyLockLoadNotes)
	assert.Equal(t, "lock_load_notes", key)
	assert.True(t, enums.CompanySettingsGeneralKeyLockLoadNotes.IsValid())

	reader := func(v string, err error) settings.SettingReader {
		return func(k string, dest *string) error {
			assert.Equal(t, key, k)
			*dest = v
			return err
		}
	}
	cases := []struct {
		name string
		v    string
		err  error
		want bool
	}{
		{"saved true", "true", nil, true},
		{"saved false", "false", nil, false},
		{"unknown value", "yes", nil, false},
		{"never saved", "", redis.Nil, false},
		{"cache down", "", errors.New("dial tcp: i/o timeout"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, settings.LoadNotesLockOnFrom(reader(tc.v, tc.err)))
		})
	}
}
