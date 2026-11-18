package fieldcatalog

// Catalog is the first version of the shared field whitelist. Every entry was
// verified against the owning service's live GraphQL schema on 2026-08-03 — a
// requirements list is never transcribed on trust (DEV-1360). Paths, value
// types and permissions were confirmed in the owning schemas; the permission is
// the query-level @hasPerm that gates the owning read.
//
// Adding a field here is a reviewed code change with a test. There is no runtime
// path that adds a bindable field.
//
// Record types are grouped by owning service. TRUCK/TRAILER/TRIP are served by
// tms-loads, a proven board-data federation hop, and are bindable today.
// DRIVER_CREW is owned by tms-teams, which is NOT a board-data hop yet, so its
// (verified) fields are catalogued with Held=true — reachable by the reporting
// engine but withheld from board columns until the hop is proven (see the Held
// doc on Entry). Compliance status is a verified tms-files enum (CrewStatus-style
// STATUS value) but has no bindable record.field path on any covered record type
// today, so it is intentionally NOT invented here — it will be added when a
// concrete owning field (e.g. an asset's complianceStatus) is exposed.
var Catalog = []Entry{
	// === TRUCK — tms-loads getTrucks, gated fleet.trucks.view ===
	{RecordType: "TRUCK", Path: "truck.id", Label: "ID", ValueType: ValueID, Permission: "fleet.trucks.view"},
	{RecordType: "TRUCK", Path: "truck.number", Label: "Truck #", ValueType: ValueString, Permission: "fleet.trucks.view", Sortable: true},
	{RecordType: "TRUCK", Path: "truck.primaryDriverName", Label: "Primary driver", ValueType: ValueString, Permission: "fleet.trucks.view"},
	{RecordType: "TRUCK", Path: "truck.createdAt", Label: "Created", ValueType: ValueDate, Permission: "fleet.trucks.view", Sortable: true},

	// === TRAILER — tms-loads getTrailers, gated fleet.trailers.view ===
	{RecordType: "TRAILER", Path: "trailer.number", Label: "Trailer #", ValueType: ValueString, Permission: "fleet.trailers.view", Sortable: true},
	{RecordType: "TRAILER", Path: "trailer.primaryDriverName", Label: "Primary driver", ValueType: ValueString, Permission: "fleet.trailers.view"},
	{RecordType: "TRAILER", Path: "trailer.isCompanyTrailer", Label: "Company trailer", ValueType: ValueBool, Permission: "fleet.trailers.view"},

	// === TRIP — tms-loads getTrips, gated shipments.trips.view ===
	// trip.grossRate is editable in tms-loads via updateTripMiles (gated by the
	// separate trip-financials write permission); left read-only here (no
	// WriteBackKey) until the write-back iteration models a write permission.
	{RecordType: "TRIP", Path: "trip.tripNumber", Label: "Trip #", ValueType: ValueString, Permission: "shipments.trips.view", Sortable: true},
	{RecordType: "TRIP", Path: "trip.status", Label: "Status", ValueType: ValueStatus, Permission: "shipments.trips.view", Sortable: true},
	{RecordType: "TRIP", Path: "trip.loadedMiles", Label: "Loaded miles", ValueType: ValueNumber, Permission: "shipments.trips.view"},
	{RecordType: "TRIP", Path: "trip.emptyMiles", Label: "Empty miles", ValueType: ValueNumber, Permission: "shipments.trips.view", Sortable: true},
	{RecordType: "TRIP", Path: "trip.grossRate", Label: "Gross rate", ValueType: ValueNumber, Permission: "shipments.trips.view"},

	// === DRIVER_CREW — tms-teams crew queries, gated teams.crews.view ===
	// Verified present in teams.graphqls (DriverCrewDetails), reachable via the
	// IMPLEMENTED getDriverCrewHistory / getAllCrews resolvers (getCrewByDispatcher
	// and getReassignment are the "not implemented" stubs — do NOT source from
	// them). Held=true: tms-teams is not a board-data federation hop yet, so these
	// are catalogued for the reporting engine but withheld from board columns.
	{RecordType: "DRIVER_CREW", Path: "driverCrew.primaryDriver.name", Label: "Primary driver", ValueType: ValueString, Permission: "teams.crews.view", Held: true},
	{RecordType: "DRIVER_CREW", Path: "driverCrew.secondaryDriver.name", Label: "Secondary driver", ValueType: ValueString, Permission: "teams.crews.view", Held: true},
	{RecordType: "DRIVER_CREW", Path: "driverCrew.truck.number", Label: "Truck #", ValueType: ValueString, Permission: "teams.crews.view", Held: true},
	{RecordType: "DRIVER_CREW", Path: "driverCrew.trailer.number", Label: "Trailer #", ValueType: ValueString, Permission: "teams.crews.view", Held: true},
	{RecordType: "DRIVER_CREW", Path: "driverCrew.status", Label: "Crew status", ValueType: ValueStatus, Permission: "teams.crews.view", Held: true},
}
