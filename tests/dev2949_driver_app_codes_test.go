package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/stretchr/testify/assert"
)

// DEV-2949: a new company's driver gets only the six codes the driver app
// calls — no load list, no load export, no office file edit.

// The one list tms-auth's 20261012120000 back-fill writes too.
var dev2949DriverCodes = []string{
	"shipments.trips.view",
	"shipments.trip_stops.view",
	"shipments.trip_files.create",
	"shipments.shipments.split",
	"settings.files.view",
	"settings.driver_app.view",
}

func TestDEV2949_DriverDefaultIsTheSixAppCodes(t *testing.T) {
	assert.ElementsMatch(t, dev2949DriverCodes, enums.DefaultRolePermissions()[enums.UserRoleDriver])
	assert.ElementsMatch(t, dev2949DriverCodes, enums.DriverRolePermissions())
}

// AC1: payroll, settings, the driver list, fleet and the load list are refused.
func TestDEV2949_NewDriverCannotOpenOfficePagesOrTheLoadList(t *testing.T) {
	driver := enums.DefaultRolePermissions()[enums.UserRoleDriver]
	for _, code := range []string{
		"accounting.pay_statements.view", // payroll
		"settings.company.view",          // settings
		"drivers.drivers.view",           // driver list
		"fleet.trucks.view",              // fleet
		"shipments.shipments.view",       // load list (getShipments) and load export
		"shipments.trip_files.view",      // office file list
		"shipments.trip_files.edit",      // office file attach / edit
		"settings.doc_types.view",        // not called by the app
		"shipments.rc_files.view",        // rate-confirmation files: out of scope
	} {
		assert.Falsef(t, middleware.HasPermission(driver, code), "driver must not pass %s", code)
	}
}

// AC2: open a trip, split a load, upload a file on the trip. Accept needs no code.
func TestDEV2949_NewDriverKeepsTripSplitAndUpload(t *testing.T) {
	driver := enums.DefaultRolePermissions()[enums.UserRoleDriver]
	for _, code := range []string{
		"shipments.trips.view", "shipments.trip_stops.view",
		"shipments.shipments.split", "shipments.trip_files.create",
	} {
		assert.Truef(t, middleware.HasPermission(driver, code), "driver must pass %s", code)
	}
	for _, code := range []string{"shipments.shipments.edit", "shipments.shipments.delete"} {
		assert.Falsef(t, middleware.HasPermission(driver, code), "cancel / edit / mark-paid stay closed: %s", code)
	}
}

// Edge: admin, dispatcher and accounting keep the access they have today.
func TestDEV2949_OfficeRolesKeepLoadListAndFiles(t *testing.T) {
	defaults := enums.DefaultRolePermissions()
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleDispatcher, enums.UserRoleAccounting} {
		for _, code := range []string{"shipments.shipments.view", "shipments.trip_files.edit", "shipments.shipments.split"} {
			assert.Truef(t, middleware.HasPermission(defaults[role], code), "%s keeps %s", role, code)
		}
	}
}
