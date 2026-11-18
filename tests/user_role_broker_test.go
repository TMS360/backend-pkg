package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1857 — the broker portal is the THIRD login surface (BL-20), so its roles
// are a family of their own: valid role names that carry no office authority.

func TestBrokerRolesAreValidAndRoundTrip(t *testing.T) {
	for name, role := range map[string]enums.UserRoleEnum{
		"broker_admin": enums.UserRoleBrokerAdmin,
		"broker_user":  enums.UserRoleBrokerUser,
	} {
		require.True(t, role.IsValid(), "%s must be a known role", name)
		// The seeder writes roles.unique_name from String(), and only unique_name
		// reaches the JWT `roles` claim that @hasRole matches — an "unknown" here
		// would make the role unholdable.
		assert.Equal(t, name, role.String())
	}
}

// The office hierarchy is the authority ladder createUser / assignPermissionsTo*
// compare against. A broker must have no standing on it: otherwise a broker
// admin could act on a carrier's office users, and the role would show up in
// office role pickers.
func TestBrokerRolesStayOutOfTheOfficeHierarchy(t *testing.T) {
	for _, role := range enums.BrokerRoles {
		_, inHierarchy := enums.UserRoleHierarchy[role]
		assert.Falsef(t, inHierarchy, "%s must not be in the office hierarchy", role)
	}

	// EffectiveHierarchy therefore reports "no authority" for a broker session,
	// which is what keeps the strictly-below checks from ever passing.
	assert.EqualValues(t, int32(2147483647),
		enums.EffectiveHierarchy([]string{"broker_admin", "broker_user"}))

	// An office role is unaffected.
	assert.EqualValues(t, 1, enums.EffectiveHierarchy([]string{"admin"}))
}

func TestIsBrokerRoleTellsTheFamiliesApart(t *testing.T) {
	assert.True(t, enums.IsBrokerRole("broker_admin"))
	assert.True(t, enums.IsBrokerRole("broker_user"))
	assert.False(t, enums.IsBrokerRole("admin"))
	assert.False(t, enums.IsBrokerRole("dispatcher"))
	assert.False(t, enums.IsBrokerRole(""))

	assert.True(t, enums.HasBrokerRole([]string{"dispatcher", "broker_user"}))
	assert.False(t, enums.HasBrokerRole([]string{"dispatcher", "admin"}))
	assert.False(t, enums.HasBrokerRole(nil))
}
