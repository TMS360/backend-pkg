// Package keycrypt seals company integration keys (HERE, Samsara, Relay,
// Google, USPS, RingCentral, Gemini…) at rest, in the database and in Redis
// (DEV-3295).
//
// Hashing cannot work here: the real key has to reach the provider on every
// call. So the key is stored encrypted and opened only at the moment of use.
//
// AES-256-GCM, moved here from tms-auth's RingCentral personal-token cipher
// (DEV-2111) so every service opens values with one implementation:
// authenticated, so a tampered value fails to open rather than decrypting into
// a plausible-looking key.
package keycrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// cipherVersion is the first byte of every sealed value. It is what makes a
// later secret change a migration rather than an outage.
const cipherVersion byte = 1

// TextPrefix marks a sealed value stored as text (a company setting). A value
// without it is a plain key written before the encryption rollout and is used
// as is.
const TextPrefix = "tmsenc:v1:"

var (
	// ErrKeyMissing means no encryption secret was configured.
	ErrKeyMissing = errors.New("keycrypt: encryption secret is not configured")
	// ErrUnreadable is the one error for every failure to open: a caller must
	// not learn from it whether the secret is wrong, the value truncated or the
	// ciphertext altered.
	ErrUnreadable = errors.New("keycrypt: sealed value is unreadable")
)

// Cipher seals and opens values with one 32-byte secret.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher accepts the secret as hex or base64 (standard or url, padded or
// not) and insists on 32 bytes — AES-256. A shorter secret is refused rather
// than stretched: silently accepting a weak key is worse than failing to boot.
func NewCipher(rawKey string) (*Cipher, error) {
	raw := strings.TrimSpace(rawKey)
	if raw == "" {
		return nil, ErrKeyMissing
	}

	key, err := decodeKey(raw)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("keycrypt: failed to build the cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("keycrypt: failed to build the cipher: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func decodeKey(raw string) ([]byte, error) {
	if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if key, err := enc.DecodeString(raw); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	return nil, errors.New("keycrypt: the secret is not valid base64 or hex for a 32-byte key")
}

// Seal encrypts plain into version byte + nonce + ciphertext. The nonce is
// random per value, so two identical keys never produce identical output. aad
// binds the value to where it belongs (nil when there is no such context).
func (c *Cipher) Seal(plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("keycrypt: failed to generate a nonce: %w", err)
	}
	out := make([]byte, 0, 1+len(nonce)+len(plain)+c.aead.Overhead())
	out = append(out, cipherVersion)
	out = append(out, nonce...)
	return c.aead.Seal(out, nonce, plain, aad), nil
}

// Open decrypts a value produced by Seal with the same aad.
func (c *Cipher) Open(sealed, aad []byte) ([]byte, error) {
	nonceSize := c.aead.NonceSize()
	if len(sealed) < 1+nonceSize {
		return nil, ErrUnreadable
	}
	if sealed[0] != cipherVersion {
		return nil, fmt.Errorf("keycrypt: sealed value has an unknown cipher version %d", sealed[0])
	}
	nonce := sealed[1 : 1+nonceSize]
	plain, err := c.aead.Open(nil, nonce, sealed[1+nonceSize:], aad)
	if err != nil {
		return nil, ErrUnreadable
	}
	return plain, nil
}

// SealText is Seal in the text form a setting value is stored in:
// TextPrefix + base64url(sealed).
func (c *Cipher) SealText(plain, aad string) (string, error) {
	sealed, err := c.Seal([]byte(plain), []byte(aad))
	if err != nil {
		return "", err
	}
	return TextPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// OpenText reverses SealText.
func (c *Cipher) OpenText(text, aad string) (string, error) {
	if !IsSealed(text) {
		return "", ErrUnreadable
	}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(text, TextPrefix))
	if err != nil {
		return "", ErrUnreadable
	}
	plain, err := c.Open(sealed, []byte(aad))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// IsSealed reports whether a stored setting value is in the sealed text form.
func IsSealed(value string) bool {
	return strings.HasPrefix(value, TextPrefix)
}
