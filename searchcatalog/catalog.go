package searchcatalog

// Catalog is the office global-search contract (DEV-2044, epic DEV-1957).
//
// Every field and relation below was read off the owning service's domain
// model on 2026-09-03 — a field list is never transcribed on trust:
//
//	LOAD/TRIP/TRUCK/TRAILER  tms-loads   internal/domain/{shipment,trip,truck,trailer,shipment_leg}.go
//	DRIVER/OFFICE_USER       tms-auth    internal/domain/user.go
//	CUSTOMER                 tms-mediator internal/domain/{courier_customer,customer_contact,customer_address}.go
//	CREW                     tms-teams   internal/domain/{driver_crew,local_driver}.go
//	TASK                     backend-tasks internal/domain/task.go
//	FILE                     tms-files   internal/domain/{file,file_link,document_type}.go
//	INVOICE/PAY_STATEMENT    backend-accounting internal/domain/{invoice,statement}.go
//
// Order matters: it is the order the groups come back in, so the entities an
// office user asks for most often come first.
//
// Money is deliberately absent. No rate, pay, balance or total is searchable
// or returned on a hit — see the epic ("no pay, rate, or balance in the row").
var Catalog = []Entity{
	// =====================================================================
	// LOAD — tms-loads, shipments table. The load is the record the office
	// searches for by every proxy it has: its own numbers, the crew hauling
	// it, the equipment under it, the broker paying for it, the city it
	// picks up in.
	// =====================================================================
	{
		Code: EntityLoad, Label: "Loads",
		Service: ServiceLoads, Permission: "shipments.shipments.view",
		Fields: []Field{
			{Path: "load.shipmentNumber", Label: "Load #", Kind: KindNumber},
			{Path: "load.loadId", Label: "Load ID", Kind: KindNumber},
			{Path: "load.referenceNumbers", Label: "Reference #", Kind: KindNumber},
			{Path: "load.proNumber", Label: "PRO #", Kind: KindNumber},
			{Path: "load.poNumber", Label: "PO #", Kind: KindNumber},
			{Path: "load.bolNumber", Label: "BOL #", Kind: KindNumber},
			{Path: "load.status", Label: "Status", Kind: KindStatus},
		},
		Relations: []Relation{
			{Path: "load.trip.tripNumber", Label: "Trip #", Kind: KindNumber, Target: EntityTrip},
			{Path: "load.trip.truck.number", Label: "Truck #", Kind: KindNumber, Target: EntityTruck},
			{Path: "load.trip.truck.vin", Label: "Truck VIN", Kind: KindCode, Target: EntityTruck},
			{Path: "load.trip.truck.licensePlate", Label: "Truck plate", Kind: KindCode, Target: EntityTruck},
			{Path: "load.trip.trailer.number", Label: "Trailer #", Kind: KindNumber, Target: EntityTrailer},
			{Path: "load.trip.trailer.vin", Label: "Trailer VIN", Kind: KindCode, Target: EntityTrailer},
			{Path: "load.trip.trailer.plateNumber", Label: "Trailer plate", Kind: KindCode, Target: EntityTrailer},
			// Crew and office people live in tms-auth, brokers in tms-mediator:
			// matching ids are resolved over gRPC before the local filter runs.
			{Path: "load.trip.mainDriver.name", Label: "Driver", Kind: KindText, Target: EntityDriver, Remote: true},
			{Path: "load.trip.mainDriver.phone", Label: "Driver phone", Kind: KindPhone, Target: EntityDriver, Remote: true},
			{Path: "load.trip.secondaryDriver.name", Label: "Co-driver", Kind: KindText, Target: EntityDriver, Remote: true},
			{Path: "load.trip.dispatcher.name", Label: "Dispatcher", Kind: KindText, Target: EntityOfficeUser, Remote: true},
			{Path: "load.customer.companyName", Label: "Customer", Kind: KindText, Target: EntityCustomer, Remote: true},
			{Path: "load.customer.mcNumber", Label: "Customer MC", Kind: KindNumber, Target: EntityCustomer, Remote: true},
			{Path: "load.customer.usdot", Label: "Customer USDOT", Kind: KindNumber, Target: EntityCustomer, Remote: true},
			{Path: "load.billTo.companyName", Label: "Bill to", Kind: KindText, Target: EntityCustomer, Remote: true},
			// Legs are same-database rows, not an entity of their own.
			{Path: "load.pickup.facilityName", Label: "Pickup facility", Kind: KindText},
			{Path: "load.pickup.location", Label: "Pickup", Kind: KindText},
			{Path: "load.pickup.city", Label: "Pickup city", Kind: KindText},
			{Path: "load.pickup.state", Label: "Pickup state", Kind: KindCode},
			{Path: "load.pickup.zip", Label: "Pickup ZIP", Kind: KindNumber},
			{Path: "load.delivery.facilityName", Label: "Delivery facility", Kind: KindText},
			{Path: "load.delivery.city", Label: "Delivery city", Kind: KindText},
			{Path: "load.delivery.state", Label: "Delivery state", Kind: KindCode},
		},
	},

	// =====================================================================
	// DRIVER — tms-auth users. Master data: tms-teams keeps a mirror
	// (local_drivers) for crew work, but a driver's name, phone and licence
	// are owned here, so the group is answered here.
	//
	// Inactive, deactivated and terminated drivers are searchable on
	// purpose: the office needs to reach the record of the person they are
	// on the phone with. The hit carries a `why` instead of vanishing.
	// =====================================================================
	{
		Code: EntityDriver, Label: "Drivers",
		Service: ServiceAuth, Permission: "drivers.drivers.view",
		Fields: []Field{
			{Path: "driver.name", Label: "Driver", Kind: KindText},
			{Path: "driver.phone", Label: "Phone", Kind: KindPhone},
			{Path: "driver.phoneSecond", Label: "Second phone", Kind: KindPhone},
			{Path: "driver.email", Label: "Email", Kind: KindEmail},
			{Path: "driver.licenseNumber", Label: "Licence #", Kind: KindCode},
			{Path: "driver.samsaraDriverId", Label: "Samsara ID", Kind: KindCode},
			{Path: "driver.driverCompanyName", Label: "Driver company", Kind: KindText},
		},
		// No relations. A driver could in principle be searched by their crew
		// partner or their truck number, but tms-auth has no client to
		// tms-teams or tms-loads, and inverting that dependency to add one
		// buys nothing the office cannot already get: the TRUCK group answers
		// "1043" and its row names the driver on it.
		Relations: nil,
	},

	// =====================================================================
	// TRUCK — tms-loads. A parked or out-of-service truck stays searchable
	// (epic: "parked trucks still show, with a short why").
	// =====================================================================
	{
		Code: EntityTruck, Label: "Trucks",
		Service: ServiceLoads, Permission: "fleet.trucks.view",
		Fields: []Field{
			{Path: "truck.number", Label: "Truck #", Kind: KindNumber},
			{Path: "truck.vin", Label: "VIN", Kind: KindCode},
			{Path: "truck.licensePlate", Label: "Plate", Kind: KindCode},
			{Path: "truck.make", Label: "Make", Kind: KindText},
			{Path: "truck.model", Label: "Model", Kind: KindText},
			{Path: "truck.year", Label: "Year", Kind: KindNumber},
			{Path: "truck.location", Label: "Location", Kind: KindText},
			{Path: "truck.lessorName", Label: "Lessor", Kind: KindText},
		},
		Relations: []Relation{
			{Path: "truck.driver.name", Label: "Driver", Kind: KindText, Target: EntityDriver, Remote: true},
			{Path: "truck.trip.load.shipmentNumber", Label: "Load #", Kind: KindNumber, Target: EntityLoad},
		},
	},

	// =====================================================================
	// TRAILER — tms-loads.
	// =====================================================================
	{
		Code: EntityTrailer, Label: "Trailers",
		Service: ServiceLoads, Permission: "fleet.trailers.view",
		Fields: []Field{
			{Path: "trailer.number", Label: "Trailer #", Kind: KindNumber},
			{Path: "trailer.vin", Label: "VIN", Kind: KindCode},
			{Path: "trailer.plateNumber", Label: "Plate", Kind: KindCode},
			{Path: "trailer.location", Label: "Location", Kind: KindText},
		},
		Relations: []Relation{
			{Path: "trailer.ownerCustomer.companyName", Label: "Trailer owner", Kind: KindText, Target: EntityCustomer, Remote: true},
			{Path: "trailer.trip.load.shipmentNumber", Label: "Load #", Kind: KindNumber, Target: EntityLoad},
		},
	},

	// =====================================================================
	// CUSTOMER — tms-mediator courier_customers (brokers and shippers).
	// company_name already carries a pg_trgm GIN index.
	// =====================================================================
	{
		Code: EntityCustomer, Label: "Customers",
		Service: ServiceMediator, Permission: "customers.brokers.view",
		Fields: []Field{
			{Path: "customer.companyName", Label: "Customer", Kind: KindText},
			{Path: "customer.legalName", Label: "Legal name", Kind: KindText},
			{Path: "customer.dbaName", Label: "DBA", Kind: KindText},
			{Path: "customer.mcNumber", Label: "MC #", Kind: KindNumber},
			{Path: "customer.usdot", Label: "USDOT", Kind: KindNumber},
			{Path: "customer.ffNumber", Label: "FF #", Kind: KindNumber},
			{Path: "customer.dunsNumber", Label: "DUNS", Kind: KindNumber},
			{Path: "customer.email", Label: "Email", Kind: KindEmail},
			{Path: "customer.phone", Label: "Phone", Kind: KindPhone},
			{Path: "customer.physicalAddress", Label: "Address", Kind: KindText},
			{Path: "customer.phyCity", Label: "City", Kind: KindText},
			{Path: "customer.phyState", Label: "State", Kind: KindCode},
			{Path: "customer.phyZip", Label: "ZIP", Kind: KindNumber},
		},
		Relations: []Relation{
			{Path: "customer.contact.name", Label: "Contact", Kind: KindText},
			{Path: "customer.contact.email", Label: "Contact email", Kind: KindEmail},
			{Path: "customer.contact.phone", Label: "Contact phone", Kind: KindPhone},
			{Path: "customer.address.city", Label: "Address city", Kind: KindText},
			{Path: "customer.address.state", Label: "Address state", Kind: KindCode},
			{Path: "customer.address.zip", Label: "Address ZIP", Kind: KindNumber},
		},
	},

	// =====================================================================
	// TRIP — tms-loads. Separate from LOAD because dispatch quotes a trip
	// number on the phone as often as a load number, and a split load has
	// several trips under one shipment.
	// =====================================================================
	{
		Code: EntityTrip, Label: "Trips",
		Service: ServiceLoads, Permission: "shipments.trips.view",
		Fields: []Field{
			{Path: "trip.tripNumber", Label: "Trip #", Kind: KindNumber},
			{Path: "trip.status", Label: "Status", Kind: KindStatus},
		},
		Relations: []Relation{
			{Path: "trip.load.shipmentNumber", Label: "Load #", Kind: KindNumber, Target: EntityLoad},
			{Path: "trip.load.loadId", Label: "Load ID", Kind: KindNumber, Target: EntityLoad},
			{Path: "trip.truck.number", Label: "Truck #", Kind: KindNumber, Target: EntityTruck},
			{Path: "trip.trailer.number", Label: "Trailer #", Kind: KindNumber, Target: EntityTrailer},
			{Path: "trip.mainDriver.name", Label: "Driver", Kind: KindText, Target: EntityDriver, Remote: true},
			{Path: "trip.dispatcher.name", Label: "Dispatcher", Kind: KindText, Target: EntityOfficeUser, Remote: true},
		},
	},

	// =====================================================================
	// OFFICE_USER — tms-auth. Outside the epic's v1 five groups but inside
	// this ticket: dispatch looks up colleagues by name and extension too.
	// =====================================================================
	{
		Code: EntityOfficeUser, Label: "Office users",
		Service: ServiceAuth, Permission: "settings.office_users.view",
		Fields: []Field{
			{Path: "officeUser.name", Label: "Name", Kind: KindText},
			{Path: "officeUser.email", Label: "Email", Kind: KindEmail},
			{Path: "officeUser.phone", Label: "Phone", Kind: KindPhone},
		},
		Relations: []Relation{
			{Path: "officeUser.role.label", Label: "Role", Kind: KindText},
		},
	},

	// =====================================================================
	// CREW — tms-teams. A crew has no text of its own: it is found through
	// its drivers and its equipment, which is exactly why dispatch searches
	// for it.
	// =====================================================================
	{
		Code: EntityCrew, Label: "Crews",
		Service: ServiceTeams, Permission: "teams.crews.view",
		Fields:  nil,
		Relations: []Relation{
			{Path: "crew.primaryDriver.name", Label: "Primary driver", Kind: KindText, Target: EntityDriver},
			{Path: "crew.secondaryDriver.name", Label: "Secondary driver", Kind: KindText, Target: EntityDriver},
			{Path: "crew.truck.number", Label: "Truck #", Kind: KindNumber, Target: EntityTruck, Remote: true},
			{Path: "crew.trailer.number", Label: "Trailer #", Kind: KindNumber, Target: EntityTrailer, Remote: true},
		},
	},

	// =====================================================================
	// TASK — backend-tasks.
	// =====================================================================
	{
		Code: EntityTask, Label: "Tasks",
		Service: ServiceTasks, Permission: "tasks.tasks.view",
		Fields: []Field{
			{Path: "task.title", Label: "Task", Kind: KindText},
			{Path: "task.description", Label: "Description", Kind: KindText},
			{Path: "task.status", Label: "Status", Kind: KindStatus},
		},
		// No relations: backend-tasks holds an assignee id and has no client to
		// tms-auth, so "tasks assigned to Ann" is not answerable here today.
		// The task's own title and description are what the office searches.
		Relations: nil,
	},

	// =====================================================================
	// FILE — tms-files. Filename plus the document type it was filed under.
	// =====================================================================
	{
		Code: EntityFile, Label: "Files",
		Service: ServiceFiles, Permission: "settings.files.view",
		Fields: []Field{
			{Path: "file.filename", Label: "File", Kind: KindText},
		},
		Relations: []Relation{
			{Path: "file.documentType.name", Label: "Document type", Kind: KindText},
		},
	},

	// =====================================================================
	// INVOICE — backend-accounting. Identifiers only: no amount, subtotal or
	// total is searchable or shown.
	// =====================================================================
	{
		Code: EntityInvoice, Label: "Invoices",
		Service: ServiceAccounting, Permission: "accounting.invoices.view",
		Fields: []Field{
			{Path: "invoice.invoiceNumber", Label: "Invoice #", Kind: KindNumber},
			{Path: "invoice.status", Label: "Status", Kind: KindStatus},
		},
		Relations: []Relation{
			{Path: "invoice.customer.companyName", Label: "Customer", Kind: KindText, Target: EntityCustomer, Remote: true},
			{Path: "invoice.load.shipmentNumber", Label: "Load #", Kind: KindNumber, Target: EntityLoad, Remote: true},
		},
	},

	// =====================================================================
	// PAY_STATEMENT — backend-accounting. Statement number and driver only;
	// the money on a statement never reaches a search hit.
	// =====================================================================
	{
		Code: EntityPayStatement, Label: "Pay statements",
		Service: ServiceAccounting, Permission: "accounting.pay_statements.view",
		Fields: []Field{
			{Path: "payStatement.number", Label: "Statement #", Kind: KindNumber},
			{Path: "payStatement.status", Label: "Status", Kind: KindStatus},
		},
		Relations: []Relation{
			{Path: "payStatement.driver.name", Label: "Driver", Kind: KindText, Target: EntityDriver, Remote: true},
		},
	},
}
