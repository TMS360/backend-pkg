package saferapi

import "encoding/json"

// SaferCompanyDTO matches the typical JSON response from FMCSA/DOT APIs
type SaferCompanyDTO struct {
	EntityType              *string          `json:"entity_type"`
	LegalName               *string          `json:"legal_name"`
	Phone                   *string          `json:"phone"`
	DbaName                 *string          `json:"dba_name"`
	DunsNumber              *string          `json:"duns_number"`
	PhysicalAddress         *string          `json:"physical_address"`
	MailingAddress          *string          `json:"mailing_address"`
	Usdot                   *string          `json:"usdot"`
	McMxFfNumbers           *string          `json:"mc_mx_ff_numbers"`
	PowerUnits              *int32           `json:"power_units"`
	Drivers                 *int32           `json:"drivers"`
	Mcs150FormDate          *string          `json:"mcs_150_form_date"`
	OutOfServiceDate        *string          `json:"out_of_service_date"`
	LatestUpdate            *string          `json:"latest_update"`
	Mcs150MileageYear       *json.RawMessage `json:"mcs_150_mileage_year"`
	OperationClassification []string         `json:"operation_classification"`
	CarrierOperation        []string         `json:"carrier_operation"`
	CargoCarried            []string         `json:"cargo_carried"`
	Url                     *string          `json:"url"`
}
