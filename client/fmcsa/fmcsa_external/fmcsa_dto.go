package fmcsa_external

// SearchResponse matches the root JSON object
type SearchResponse struct {
	Content       []Content `json:"content"`
	RetrievalDate string    `json:"retrievalDate"`
}

type Content struct {
	Carrier Carrier `json:"carrier"`
	// We can ignore "_links" unless we need to make follow-up calls
}

type Carrier struct {
	LegalName        string  `json:"legalName"`
	DbaName          *string `json:"dbaName"` // Nullable
	DotNumber        int     `json:"dotNumber"`
	StatusCode       string  `json:"statusCode"`       // "A" (Active), "I" (Inactive)
	AllowedToOperate string  `json:"allowedToOperate"` // "Y" or "N"

	// Address Fields
	PhyStreet  string `json:"phyStreet"`
	PhyCity    string `json:"phyCity"`
	PhyState   string `json:"phyState"`
	PhyZipcode string `json:"phyZipcode"`
	PhyCountry string `json:"phyCountry"`

	// Stats
	TotalPowerUnits int `json:"totalPowerUnits"`
	TotalDrivers    int `json:"totalDrivers"`

	// Nested Objects
	CarrierOperation *CarrierOperation `json:"carrierOperation"`
	CensusTypeId     *CensusType       `json:"censusTypeId"`

	// Safety / Status
	OosDate *string `json:"oosDate"` // Nullable date string
}

type CarrierOperation struct {
	Code string `json:"carrierOperationCode"`
	Desc string `json:"carrierOperationDesc"` // Maps to 'carrier_operation'
}

type CensusType struct {
	Desc string `json:"censusTypeDesc"` // Maps to 'entity_type'
}
