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
	RecordType   string    `gorm:"column:record_type"`
	RecordID     string    `gorm:"column:record_id"`
	CompanyID    string    `gorm:"column:company_id"`
	Key          string    `gorm:"column:key"`
	Label        string    `gorm:"column:label"`
	Value        string    `gorm:"column:value"` // sealed text when Private
	Private      bool      `gorm:"column:private"`
	SourceSystem string    `gorm:"column:source_system"`
	BatchID      string    `gorm:"column:migration_batch_id"`
	ImportedAt   time.Time `gorm:"column:imported_at"`
}

// TableName is the table every owning service keeps its imported details in.
func (Detail) TableName() string { return "imported_details" }
