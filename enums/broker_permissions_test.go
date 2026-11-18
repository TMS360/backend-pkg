package enums_test

import (
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func brokerPortalCodes() []string {
	return []string{
		string(enums.PermBrokerLoadsView),
		string(enums.PermBrokerLoadsManage),
		string(enums.PermBrokerOffersSend),
		string(enums.PermBrokerOffersRevoke),
		string(enums.PermBrokerCarriersFind),
	}
}

// The shape is the security property, so it is asserted directly rather than
// trusted to review. HasPermission matches any dotted prefix of the required
// code: a `broker.loads.create` would be satisfied by anyone holding `broker`,
// and a top-level `broker` module would be swept into ModulePermissionCodes and
// auto-granted to every built-in role at signup — driver included.
func TestBrokerPermissions_AreFlatAndGrantable(t *testing.T) {
	for _, code := range brokerPortalCodes() {
		assert.NotContainsf(t, code, ".", "%q must be flat: a dotted code is satisfied by its own prefix", code)
		assert.Truef(t, strings.HasPrefix(code, "broker_"),
			"%q must start with broker_ or tms-auth's FilterBrokerPerms strips it off a broker session", code)
		assert.Truef(t, enums.IsCustomPermissionCode(code), "%q must be a registered flat custom code", code)
		assert.Truef(t, enums.IsValidPermissionCode(code), "%q must validate so custom roles can grant it", code)
	}
}

// A flat code must not be reachable through a prefix. This is the assertion that
// would fail the moment someone "tidies" these into broker.loads.view.
func TestBrokerPermissions_NoPrefixSatisfiesThem(t *testing.T) {
	for _, code := range brokerPortalCodes() {
		assert.Falsef(t, middleware.HasPermission([]string{"broker"}, code),
			"holding a bare %q must not satisfy %q", "broker", code)
		assert.Falsef(t, middleware.HasPermission([]string{"shipments"}, code),
			"an office module must never satisfy %q", code)
		assert.Truef(t, middleware.HasPermission([]string{code}, code),
			"%q must be satisfied by holding exactly itself", code)
	}
}

// The broker roles hold the portal and NOTHING else. In particular they must not
// receive the office module baseline every other seeded role gets.
func TestBrokerRoles_HoldPortalOnly(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	for _, role := range enums.BrokerRoles {
		granted := defaults[role]
		require.NotEmptyf(t, granted, "%s must hold the portal codes", role)
		assert.ElementsMatchf(t, brokerPortalCodes(), granted,
			"%s must hold the broker portal and nothing else — no office module baseline", role)
	}
}

// BL-22 §22.1, now assertable against a real broker role rather than its
// customer-role analogue: the broker portal must never be granted the report
// builder, by default or by any prefix.
func TestBrokerRoles_NeverHoldReports(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	for _, role := range enums.BrokerRoles {
		granted := defaults[role]
		for _, code := range []string{string(enums.PermReportsRun), string(enums.PermReportsManage)} {
			assert.NotContainsf(t, granted, code, "%s must not hold %q", role, code)
			assert.Falsef(t, middleware.HasPermission(granted, code),
				"%s must not resolve %q by any prefix", role, code)
		}
	}
}

// And the reverse direction: no office role picks up a broker capability.
func TestOfficeRoles_NeverHoldBrokerPortal(t *testing.T) {
	defaults := enums.DefaultRolePermissions()

	office := []enums.UserRoleEnum{
		enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting,
		enums.UserRoleFleet, enums.UserRoleSafety, enums.UserRoleHr,
		enums.UserRoleAuditor, enums.UserRoleDispatcher, enums.UserRoleTrackAndTrace,
		enums.UserRoleDriver, enums.UserRoleOther,
	}
	for _, role := range office {
		granted := defaults[role]
		for _, code := range brokerPortalCodes() {
			assert.Falsef(t, middleware.HasPermission(granted, code),
				"office role %s must not resolve broker code %q", role, code)
		}
	}
}
