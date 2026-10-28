package importeddetail

import (
	"encoding/base64"
	"strings"

	"github.com/TMS360/backend-pkg/keycrypt"
)

// TextPrefix marks a sealed imported detail. It differs from
// keycrypt.TextPrefix on purpose: keycrypt.OpenSetting refuses it, and Open
// refuses a sealed integration key.
const TextPrefix = "tmsimp:v1:"

func aad(companyID, recordType, recordID, key string) []byte {
	return []byte("importeddetail|" + companyID + "|" + recordType + "|" + recordID + "|" + key)
}

// IsSealed reports whether a stored value is a sealed imported detail.
func IsSealed(v string) bool { return strings.HasPrefix(v, TextPrefix) }

// Seal seals a private value for one company, record and key with the
// process-wide keycrypt cipher. The error never carries the value.
func Seal(companyID, recordType, recordID, key, value string) (string, error) {
	c, err := keycrypt.Default()
	if err != nil {
		return "", err
	}
	sealed, err := c.Seal([]byte(value), aad(companyID, recordType, recordID, key))
	if err != nil {
		return "", err
	}
	return TextPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal. A value sealed for another company, record or key, or in
// another format (an integration key), fails with keycrypt.ErrUnreadable.
func Open(companyID, recordType, recordID, key, stored string) (string, error) {
	if !IsSealed(stored) {
		return "", keycrypt.ErrUnreadable
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, TextPrefix))
	if err != nil {
		return "", keycrypt.ErrUnreadable
	}
	c, err := keycrypt.Default()
	if err != nil {
		return "", err
	}
	plain, err := c.Open(raw, aad(companyID, recordType, recordID, key))
	if err != nil {
		return "", keycrypt.ErrUnreadable
	}
	return string(plain), nil
}
