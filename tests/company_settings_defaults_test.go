package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-4570: a sign-up Dispatcher (and Track & Trace, derived from it) reads
// company settings but cannot save them; admin still can.
func TestDefaultRolePermissions_CompanySettingsEdit(t *testing.T) {
	view := "settings.company.view"
	edit := string(enums.PermSettingsCompanyEdit)
	defaults := enums.DefaultRolePermissions()

	cases := []struct {
		role       enums.UserRoleEnum
		view, edit bool
	}{
		{enums.UserRoleAdmin, true, true},
		{enums.UserRoleDispatcher, true, false},
		{enums.UserRoleTrackAndTrace, true, false},
	}
	for _, tc := range cases {
		perms, ok := defaults[tc.role]
		require.Truef(t, ok, "%s must have default grants", tc.role)
		assert.Equalf(t, tc.view, middleware.HasPermission(perms, view), "%s: %s", tc.role, view)
		assert.Equalf(t, tc.edit, middleware.HasPermission(perms, edit), "%s: %s", tc.role, edit)
	}
}

// Only company-settings edit moves: expanded to leaves, the dispatcher's set is
// the old `settings`-module set minus settings.company.edit.
func TestDefaultRolePermissions_DispatcherKeepsOtherSettings(t *testing.T) {
	edit := string(enums.PermSettingsCompanyEdit)
	for _, role := range []enums.UserRoleEnum{enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace} {
		perms := enums.DefaultRolePermissions()[role]
		assert.NotContainsf(t, perms, enums.SettingsModuleCode, "%s must not hold the whole settings module", role)

		before := append([]string{enums.SettingsModuleCode}, withoutSettingsCodes(perms)...)
		want := without(enums.ExpandPermissions(before), edit)
		assert.ElementsMatchf(t, want, enums.ExpandPermissions(perms), "%s: only settings.company.edit may change", role)

		for _, code := range []string{"settings.doc_types.view", "settings.load_status.edit", "settings.compliance.view", "settings.files.view"} {
			assert.Truef(t, middleware.HasPermission(perms, code), "%s keeps %s", role, code)
		}
	}
}

// A settings entity added later reaches the dispatcher without touching this
// list; only settings.company is ever left out.
func TestSettingsEntitiesWithoutCompany_IsEverySettingsEntityButCompany(t *testing.T) {
	ents := enums.SettingsEntitiesWithoutCompany()
	assert.NotContains(t, ents, "settings.company")
	assert.ElementsMatch(t,
		without(enums.ExpandPermissions([]string{enums.SettingsModuleCode}),
			"settings.company.view", string(enums.PermSettingsCompanyEdit)),
		enums.ExpandPermissions(ents))
}

// The narrowed shape survives a role-editor save: RollupPermissions does not
// collapse it back into `settings`.
func TestDefaultRolePermissions_NarrowedSettingsSurvivesRollup(t *testing.T) {
	perms := enums.DefaultRolePermissions()[enums.UserRoleDispatcher]
	rolled := enums.RollupPermissions(enums.ExpandPermissions(perms))
	assert.NotContains(t, rolled, enums.SettingsModuleCode)
	assert.False(t, middleware.HasPermission(rolled, string(enums.PermSettingsCompanyEdit)))
	assert.True(t, middleware.HasPermission(rolled, "settings.company.view"))
}

// A custom role given settings.company.edit on purpose passes the gate.
func TestCompanySettingsEdit_CustomRoleGrant(t *testing.T) {
	assert.True(t, middleware.HasPermission([]string{"settings.company.view", "settings.company.edit"}, string(enums.PermSettingsCompanyEdit)))
	assert.True(t, middleware.HasPermission([]string{"settings.company"}, string(enums.PermSettingsCompanyEdit)))
}

func withoutSettingsCodes(codes []string) []string {
	var out []string
	for _, c := range codes {
		if !middleware.HasPermission([]string{enums.SettingsModuleCode}, c) {
			out = append(out, c)
		}
	}
	return out
}
