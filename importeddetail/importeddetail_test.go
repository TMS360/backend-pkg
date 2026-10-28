package importeddetail_test

import (
	"encoding/base64"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/importeddetail"
	"github.com/TMS360/backend-pkg/keycrypt"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setCipher(t *testing.T) {
	t.Helper()
	c, err := keycrypt.NewCipher(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	require.NoError(t, err)
	keycrypt.SetDefault(c)
}

func TestSealOpen_BoundToCompanyRecordKey(t *testing.T) {
	setCipher(t)
	s, err := importeddetail.Seal("c1", importeddetail.RecordDriver, "r1", "ssn", "123-45-6789")
	require.NoError(t, err)
	assert.NotContains(t, s, "6789")

	got, err := importeddetail.Open("c1", importeddetail.RecordDriver, "r1", "ssn", s)
	require.NoError(t, err)
	assert.Equal(t, "123-45-6789", got)

	for _, tc := range [][4]string{
		{"c2", importeddetail.RecordDriver, "r1", "ssn"},
		{"c1", importeddetail.RecordTruck, "r1", "ssn"},
		{"c1", importeddetail.RecordDriver, "r2", "ssn"},
		{"c1", importeddetail.RecordDriver, "r1", "ein"},
	} {
		_, err := importeddetail.Open(tc[0], tc[1], tc[2], tc[3], s)
		assert.ErrorIs(t, err, keycrypt.ErrUnreadable, "%v", tc)
	}
}

func TestSealedFormatsDoNotCross(t *testing.T) {
	setCipher(t)
	imp, err := importeddetail.Seal("c1", importeddetail.RecordDriver, "r1", "k", "secret")
	require.NoError(t, err)
	_, err = keycrypt.OpenSetting("c1", "k", imp)
	assert.ErrorIs(t, err, keycrypt.ErrUnreadable, "an imported detail must not open as an integration key")

	key, err := keycrypt.SealSetting("c1", "k", "secret")
	require.NoError(t, err)
	_, err = importeddetail.Open("c1", importeddetail.RecordDriver, "r1", "k", key)
	assert.ErrorIs(t, err, keycrypt.ErrUnreadable, "an integration key must not open as an imported detail")

	_, err = importeddetail.Open("c1", importeddetail.RecordDriver, "r1", "k", "plain text")
	assert.ErrorIs(t, err, keycrypt.ErrUnreadable)
}

func TestMask(t *testing.T) {
	for _, tc := range []struct{ key, in, want string }{
		{"ssn", "123-45-6789", "•••-••-6789"},
		{"account_number", "000123457710", "••••••7710"},
		{"routing_number", "021000021", "••••••0021"},
		{"ein", "12-3456789", "••••••6789"},
		{"fuel_card_number", "7071 0000 1234 5678", "••••••5678"},
		{"birth_date", "1984-03-07", "••/••/1984"},
		{"birth_date", "03/07/1984", "••/••/1984"},
		{"emergency_contact_phone", "+1 555 010 2030", "••••••"},
		{"marital_status", "married", "••••••"},
		{"ssn", "12", "••••••"},
	} {
		assert.Equal(t, tc.want, importeddetail.Mask(tc.key, tc.in), "%s=%s", tc.key, tc.in)
	}
	assert.Equal(t, "••••••", importeddetail.MaskNone("ssn", "123-45-6789"))
}

func TestPrivateDataView_DefaultGrants(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	code := string(enums.PermPrivateDataView)

	assert.True(t, enums.IsCustomPermissionCode(code), "must be grantable")
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleAccounting, enums.UserRoleHr} {
		assert.Truef(t, middleware.HasPermission(defaults[role], code), "%s must hold %s", role, code)
	}
	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleManager, enums.UserRoleFleet, enums.UserRoleSafety,
		enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace, enums.UserRoleAuditor,
		enums.UserRoleDriver, enums.UserRoleOther, enums.UserRoleBrokerAdmin, enums.UserRoleBrokerUser,
	} {
		assert.Falsef(t, middleware.HasPermission(defaults[role], code), "%s must not hold %s by default", role, code)
	}
}
