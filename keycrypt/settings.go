package keycrypt

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// EnvSecret is the environment variable holding the integration-key secret.
// It must be the same value on tms-auth (which seals) and on every service that
// reads a company integration key, per environment.
const EnvSecret = "INTEGRATION_KEYS_SECRET"

var (
	defaultMu     sync.RWMutex
	defaultCipher *Cipher
)

// LoadFromEnv builds the process-wide cipher from EnvSecret. Every service
// that reads integration keys calls it at boot and refuses to start on error:
// a reader without the secret would turn every sealed key into "not
// connected" at runtime, which is far harder to notice than a failed deploy.
func LoadFromEnv() error {
	c, err := NewCipher(os.Getenv(EnvSecret))
	if err != nil {
		return fmt.Errorf("%s: %w", EnvSecret, err)
	}
	SetDefault(c)
	return nil
}

// SetDefault installs the process-wide cipher (tests, tools).
func SetDefault(c *Cipher) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultCipher = c
}

// Default returns the process-wide cipher, loading it from EnvSecret when
// nothing was installed. Other sealed-value formats (importeddetail) build on it.
func Default() (*Cipher, error) { return getDefault() }

func getDefault() (*Cipher, error) {
	defaultMu.RLock()
	c := defaultCipher
	defaultMu.RUnlock()
	if c != nil {
		return c, nil
	}
	// A one-off command that skipped LoadFromEnv still works when the
	// variable is set.
	if err := LoadFromEnv(); err != nil {
		return nil, err
	}
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultCipher, nil
}

// settingAAD binds a sealed value to its company and setting key, so a sealed
// key copied into another company's (or another integration's) slot does not
// open.
func settingAAD(companyID, settingKey string) string {
	return companyID + ":setting:" + settingKey
}

// OpenSetting returns the usable value of a stored company integration
// setting. A plain value (written before the rollout) is returned as is; a
// sealed one is opened with the process-wide cipher. The error never carries
// the value.
func OpenSetting(companyID, settingKey, stored string) (string, error) {
	// A value sealed in another format (an imported detail, "tmsimp:") is never
	// a usable key: refuse it instead of handing it out as a plain value.
	if strings.HasPrefix(stored, "tmsimp:") {
		return "", ErrUnreadable
	}
	if !IsSealed(stored) {
		return stored, nil
	}
	c, err := getDefault()
	if err != nil {
		return "", err
	}
	plain, err := c.OpenText(stored, settingAAD(companyID, settingKey))
	if err != nil {
		return "", fmt.Errorf("keycrypt: %s for company %s: %w", settingKey, companyID, err)
	}
	return plain, nil
}

// SealSetting seals a company integration setting for storage. An already
// sealed value is returned unchanged, so sealing twice is a no-op.
func SealSetting(companyID, settingKey, plain string) (string, error) {
	if IsSealed(plain) {
		return plain, nil
	}
	c, err := getDefault()
	if err != nil {
		return "", err
	}
	return c.SealText(plain, settingAAD(companyID, settingKey))
}
