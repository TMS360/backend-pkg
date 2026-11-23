package deployenv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// DEV-3306: only Railway's own "production" opens the real vendor servers.
func TestIsProduction_OnlyExactRailwayProduction(t *testing.T) {
	t.Setenv(RailwayEnvVar, "production")
	assert.True(t, IsProduction())
}

// DEV-3306 edge cases: staging, dev, a laptop/CI (no variable), a renamed or
// mistyped production — all fail safe to "not production".
func TestIsProduction_EverythingElseIsNotProduction(t *testing.T) {
	for _, name := range []string{"", "staging", "development", "prod", "Production", " production", "production-copy"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(RailwayEnvVar, name)
			assert.False(t, IsProduction())
		})
	}
}

// DEV-3306: the hand-typed APP_ENV is not a second way to say "production".
func TestIsProduction_IgnoresAppEnv(t *testing.T) {
	t.Setenv(RailwayEnvVar, "staging")
	t.Setenv("APP_ENV", "prod")
	assert.False(t, IsProduction())
}
