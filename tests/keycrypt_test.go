package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TMS360/backend-pkg/keycrypt"
)

const keycryptTestSecret = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // base64 of 32 bytes

// DEV-3295 AC4: a reader started without the secret must refuse to start with
// a message that names the variable.
func TestKeycrypt_LoadFromEnv_MissingSecretNamesTheVariable(t *testing.T) {
	t.Setenv(keycrypt.EnvSecret, "")

	err := keycrypt.LoadFromEnv()

	require.ErrorIs(t, err, keycrypt.ErrKeyMissing)
	assert.Contains(t, err.Error(), "INTEGRATION_KEYS_SECRET")
}

func TestKeycrypt_LoadFromEnv_RejectsAShortSecret(t *testing.T) {
	t.Setenv(keycrypt.EnvSecret, "too-short")

	require.Error(t, keycrypt.LoadFromEnv())
}

func TestKeycrypt_LoadFromEnv_AcceptsHexAndBase64(t *testing.T) {
	t.Cleanup(func() { keycrypt.SetDefault(nil) })
	for _, secret := range []string{keycryptTestSecret, strings.Repeat("ab", 32)} {
		t.Setenv(keycrypt.EnvSecret, secret)
		require.NoError(t, keycrypt.LoadFromEnv(), secret)
	}
}

// The text form round-trips, and the same plain key never seals to the same
// text twice (random nonce).
func TestKeycrypt_SealSettingRoundTrip(t *testing.T) {
	t.Setenv(keycrypt.EnvSecret, keycryptTestSecret)
	require.NoError(t, keycrypt.LoadFromEnv())
	t.Cleanup(func() { keycrypt.SetDefault(nil) })

	a, err := keycrypt.SealSetting("c1", "here_api_key", "k-123")
	require.NoError(t, err)
	b, err := keycrypt.SealSetting("c1", "here_api_key", "k-123")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(a, keycrypt.TextPrefix))
	assert.NotEqual(t, a, b)
	assert.NotContains(t, a, "k-123")

	plain, err := keycrypt.OpenSetting("c1", "here_api_key", a)
	require.NoError(t, err)
	assert.Equal(t, "k-123", plain)
}

// Sealing an already sealed value is a no-op (DEV-3296 re-runnable conversion).
func TestKeycrypt_SealSettingTwiceIsANoOp(t *testing.T) {
	t.Setenv(keycrypt.EnvSecret, keycryptTestSecret)
	t.Cleanup(func() { keycrypt.SetDefault(nil) })

	once, err := keycrypt.SealSetting("c1", "relay_api_key", "r-1")
	require.NoError(t, err)
	twice, err := keycrypt.SealSetting("c1", "relay_api_key", once)
	require.NoError(t, err)

	assert.Equal(t, once, twice)
}

// A plain value passes through untouched even when no secret is configured, so
// a reader can be deployed before any key is sealed.
func TestKeycrypt_OpenSetting_PlainValuePassesThrough(t *testing.T) {
	t.Setenv(keycrypt.EnvSecret, "")
	keycrypt.SetDefault(nil)

	plain, err := keycrypt.OpenSetting("c1", "here_api_key", "plain-key")

	require.NoError(t, err)
	assert.Equal(t, "plain-key", plain)
}

// A sealed value is bound to its company AND its setting key.
func TestKeycrypt_OpenSetting_BoundToCompanyAndKey(t *testing.T) {
	t.Setenv(keycrypt.EnvSecret, keycryptTestSecret)
	t.Cleanup(func() { keycrypt.SetDefault(nil) })
	sealed, err := keycrypt.SealSetting("c1", "here_api_key", "k")
	require.NoError(t, err)

	_, err = keycrypt.OpenSetting("c2", "here_api_key", sealed)
	assert.ErrorIs(t, err, keycrypt.ErrUnreadable)
	_, err = keycrypt.OpenSetting("c1", "google_maps_api_key", sealed)
	assert.ErrorIs(t, err, keycrypt.ErrUnreadable)
}

// The binary form stays byte-compatible with the RingCentral personal-token
// columns sealed before the move (version byte + 12-byte nonce + ciphertext).
func TestKeycrypt_BinaryFormatIsVersionNonceCiphertext(t *testing.T) {
	c, err := keycrypt.NewCipher(keycryptTestSecret)
	require.NoError(t, err)

	sealed, err := c.Seal([]byte("token"), nil)
	require.NoError(t, err)

	assert.Equal(t, byte(1), sealed[0])
	assert.Len(t, sealed, 1+12+len("token")+16)
	plain, err := c.Open(sealed, nil)
	require.NoError(t, err)
	assert.Equal(t, "token", string(plain))
}
