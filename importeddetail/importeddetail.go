// Package importeddetail holds the values a migrated record brought from the
// old system that TMS360 has no field for (DEV-3506, BL §21.21). Every
// service that owns imported records (auth drivers, load trucks / trailers /
// Truck Owners, accounting vendors, mediator-cb customers) keeps them in its
// own imported_details table and reads them through this package.
//
// Private values (SSN, bank, birth date, emergency contact, tax details) are
// sealed with the service's existing keycrypt secret under their own tag, so a
// sealed imported detail never opens as an integration key and the other way
// round. They come back masked; the full value needs private_data_view.
package importeddetail

import "time"

// Record types. They are part of the sealing AAD, so the writer (the migration
// commit) and every reader must use these exact strings.
const (
	RecordDriver     = "driver"
	RecordTruck      = "truck"
	RecordTrailer    = "trailer"
	RecordTruckOwner = "truck_owner"
	RecordVendor     = "vendor"
	RecordCustomer   = "customer"
)

// Detail is one imported value on one record.
type Detail struct {
	RecordType   string
	RecordID     string
	CompanyID    string
	Key          string
	Label        string
	Value        string // sealed text when Private
	Private      bool
	SourceSystem string
	BatchID      string
	ImportedAt   time.Time
}
