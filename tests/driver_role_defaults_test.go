package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2866: the built-in driver role gets the driver app's calls only, not the
// office module baseline.

// AC1: a new company's driver cannot open payroll, settings, the driver list,
// fleet — or any other office page.
func TestDEV2866_DriverHoldsNoOfficeAccess(t *testing.T) {
	driver := enums.DefaultRolePermissions()[enums.UserRoleDriver]
	require.NotEmpty(t, driver)

	for _, module := range enums.ModulePermissionCodes() {
		assert.NotContainsf(t, driver, module, "driver must not hold the whole %s module", module)
	}
	for _, code := range []string{
		"accounting.pay_statements.view", "accounting.pay_batches.view", // payroll
		"settings.company.view", "settings.office_users.view", "settings.office_roles.edit", // settings
		"drivers.drivers.view", "drivers.drivers.create", // driver list / driver create
		"fleet.trucks.view", "fleet.trailers.view", "fleet.maintenance.view", // fleet
		"customers.brokers.view", "teams.teams.view", "tasks.tasks.view", "fuel.plans.view",
		"dashboard.stats.view", "workspaces.boards.view",
	} {
		assert.Falsef(t, middleware.HasPermission(driver, code), "driver must not pass %s", code)
	}
}

// AC2: the driver keeps what the app calls — trips, stops, accept (no code),
// split, chat (no code), trip-file upload, company files, the app config.
func TestDEV2866_DriverKeepsTheAppCalls(t *testing.T) {
	driver := enums.DefaultRolePermissions()[enums.UserRoleDriver]
	for _, code := range []string{
		"shipments.trips.view",        // getTrips, getTrip, User.activeTrip
		"shipments.trip_stops.view",   // getTripStops
		"shipments.shipments.view",    // getShipment
		"shipments.trip_files.view",   // trip files
		"shipments.trip_files.create", // addTripFile, addTripStopFile
		"shipments.trip_files.edit",   // attachOrderFile
		"shipments.shipments.split",   // splitOrder
		"settings.files.view",         // getCompanyFiles, getDriverDocuments, getDriverShipmentFiles
		"settings.doc_types.view",     // getDocTypes (upload picker)
		"settings.driver_app.view",    // getDriverAppConfig
	} {
		assert.Truef(t, middleware.HasPermission(driver, code), "driver must pass %s", code)
	}
	for _, code := range driver {
		assert.Truef(t, enums.IsValidPermissionCode(code), "%s must be a catalog code", code)
	}
}

// Split stays allowed for a driver; cancel, edit and mark-paid do not.
func TestDEV2866_SplitIsItsOwnLeaf(t *testing.T) {
	driver := enums.DefaultRolePermissions()[enums.UserRoleDriver]
	assert.True(t, middleware.HasPermission(driver, string(enums.PermShipmentsShipmentsSplit)))
	for _, code := range []string{"shipments.shipments.edit", "shipments.shipments.create", "shipments.shipments.delete", "shipments.trips.edit"} {
		assert.Falsef(t, middleware.HasPermission(driver, code), "split must not carry %s", code)
	}
	assert.True(t, enums.IsValidPermissionCode(string(enums.PermShipmentsShipmentsSplit)), "split is grantable")
	assert.False(t, middleware.HasPermission([]string{"shipments.shipments.edit"}, "shipments.shipments.split"),
		"edit does not imply split — tms-loads keeps edit as an alternative on splitOrder")
}

// Edge: the app's unused update-truck and manual check-in calls are not granted.
func TestDEV2866_UnusedAppCallsStayClosed(t *testing.T) {
	driver := enums.DefaultRolePermissions()[enums.UserRoleDriver]
	for _, code := range []string{"shipments.trip_stops.edit", "fleet.trucks.edit"} {
		assert.Falsef(t, middleware.HasPermission(driver, code), "driver must not pass %s", code)
	}
}

// AC4: other roles stay as they are — every office role still holds the
// `shipments` module (so it passes split), and admin, dispatcher and accounting
// still pass their office pages.
func TestDEV2866_OfficeRolesUnchanged(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	for _, role := range []enums.UserRoleEnum{
		enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleAccounting, enums.UserRoleFleet,
		enums.UserRoleSafety, enums.UserRoleHr, enums.UserRoleAuditor, enums.UserRoleDispatcher,
		enums.UserRoleTrackAndTrace, enums.UserRoleOther,
	} {
		perms := defaults[role]
		for _, module := range []string{"shipments", "drivers", "accounting", "settings", "customers", "dashboard"} {
			assert.Containsf(t, perms, module, "%s keeps the %s module", role, module)
		}
		assert.Truef(t, middleware.HasPermission(perms, "shipments.shipments.split"), "%s passes split via the module", role)
	}
}
