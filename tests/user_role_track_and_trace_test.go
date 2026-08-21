package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1824 / BL-4 §4.17 — `track_and_trace` is a GLOBAL built-in office role
// with the dispatcher's access and none of the dispatcher's Teams side effects.
//
// Why a separate built-in and not "just give them dispatcher": tms-teams reacts
// to the dispatcher role NAME — it auto-creates a team for the user and lists
// them in the "add dispatcher to team" pickers. Track & Trace staff belong in
// neither, so the only clean separation is a different role name carrying the
// same permission set. Nothing in tms-teams changes; the exclusion is a
// consequence of the name.

// The name string is the contract: BuildUserClaims (tms-auth) puts only
// roles.unique_name into the JWT, the seeder derives both roles.name and
// roles.unique_name from String(), and @hasPerm/@hasRole compare it
// case-sensitively. Renaming it after ship silently changes access.
func TestUserRoleTrackAndTrace_CanonicalString(t *testing.T) {
	assert.Equal(t, "track_and_trace", string(enums.UserRoleTrackAndTrace))
	assert.Equal(t, "track_and_trace", enums.UserRoleTrackAndTrace.String())

	// The one thing it must never be: the dispatcher's own name. That is what
	// keeps tms-teams out of the picture.
	assert.NotEqual(t, enums.UserRoleDispatcher.String(), enums.UserRoleTrackAndTrace.String())
}

// IsValid gates role assignment; a role that is defined but not valid can be
// seeded and still be rejected everywhere it is used.
func TestUserRoleTrackAndTrace_IsValid(t *testing.T) {
	assert.True(t, enums.UserRoleTrackAndTrace.IsValid())
	assert.True(t, enums.UserRoleEnum("track_and_trace").IsValid())
}

// AC4 support: same band as dispatcher. Hierarchy drives the strictly-below
// check in createUser and assignPermissionsTo*, so a different band would make
// the two roles behave differently for the same person.
func TestUserRoleTrackAndTrace_SharesTheDispatcherBand(t *testing.T) {
	h, ok := enums.UserRoleHierarchy[enums.UserRoleTrackAndTrace]
	require.True(t, ok, "absent from the hierarchy means the seeder falls back to band 4")

	assert.EqualValues(t, 3, h)
	assert.Equal(t, enums.UserRoleHierarchy[enums.UserRoleDispatcher], h, "peer with dispatcher")
	assert.Greater(t, h, enums.UserRoleHierarchy[enums.UserRoleManager], "below manager")
}

// AC4: a user holding BOTH roles keeps the dispatcher's authority — holding
// track_and_trace must never weaken anybody.
func TestUserRoleTrackAndTrace_EffectiveHierarchyResolves(t *testing.T) {
	assert.EqualValues(t, 3, enums.EffectiveHierarchy([]string{"track_and_trace"}))
	assert.EqualValues(t, 3, enums.EffectiveHierarchy([]string{"track_and_trace", "dispatcher"}))
	assert.EqualValues(t, 1, enums.EffectiveHierarchy([]string{"track_and_trace", "admin"}))
}

// AC1: on a NEW company signup SetDefaultRolePerms seeds from this map, so the
// two sets must be identical here — element-for-element, dispatcher's flat
// custom perms (calls_view / calls_play) included.
func TestUserRoleTrackAndTrace_DefaultPermsEqualTheDispatcherSet(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	tnt, ok := defaults[enums.UserRoleTrackAndTrace]
	require.True(t, ok, "no default grants means the role exists but grants nothing")
	require.NotEmpty(t, tnt)

	assert.ElementsMatch(t, defaults[enums.UserRoleDispatcher], tnt,
		"track_and_trace is defined as 'the dispatcher set' — it must not drift")
	assert.Subset(t, tnt, enums.ModulePermissionCodes(), "the module baseline is included")
}

// The set is derived, not copy-pasted: mutating one role's returned slice must
// not touch the other's, and each call must hand out fresh slices.
func TestUserRoleTrackAndTrace_SlicesAreIndependentCopies(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	tnt := defaults[enums.UserRoleTrackAndTrace]
	require.NotEmpty(t, tnt)

	tnt[0] = "mutated"

	fresh := enums.DefaultRolePermissions()
	assert.NotEqual(t, "mutated", fresh[enums.UserRoleTrackAndTrace][0])
	assert.NotEqual(t, "mutated", fresh[enums.UserRoleDispatcher][0])
}

// Adding a role must not disturb the existing bands or grants.
func TestUserRoleTrackAndTrace_DoesNotDisturbExistingRoles(t *testing.T) {
	assert.EqualValues(t, 0, enums.UserRoleHierarchy[enums.UserRoleSuperAdmin])
	assert.EqualValues(t, 1, enums.UserRoleHierarchy[enums.UserRoleAdmin])
	assert.EqualValues(t, 2, enums.UserRoleHierarchy[enums.UserRoleManager])
	assert.EqualValues(t, 3, enums.UserRoleHierarchy[enums.UserRoleDispatcher])

	defaults := enums.DefaultRolePermissions()
	_, ok := defaults[enums.UserRoleSuperAdmin]
	assert.False(t, ok, "super_admin still bypasses permission checks")
	assert.Contains(t, defaults[enums.UserRoleAdmin], string(enums.PermTripFinancialsEdit))
	assert.Contains(t, defaults[enums.UserRoleDispatcher], string(enums.PermCallsView))
	assert.NotContains(t, defaults[enums.UserRoleDriver], string(enums.PermCallsView))
}
