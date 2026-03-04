package fmcsa

// SearchParams encapsulates query arguments for clean API usage.
type SearchParams struct {
	Query      string
	Limit      int
	Offset     int
	ActiveOnly bool
}

type SearchResponse struct {
	Query         string   `json:"query"`
	SearchType    string   `json:"search_type"`
	Count         int      `json:"count"`
	Results       []Result `json:"results"`
	DataAvailable bool     `json:"data_available"`
}

type Result struct {
	DotNumber        int     `json:"dot_number"`
	LegalName        string  `json:"legal_name"`
	DbaName          string  `json:"dba_name"`
	EntityType       string  `json:"entity_type"`
	OperatingStatus  string  `json:"operating_status"`
	StatusCode       string  `json:"status_code"`
	Phone            string  `json:"phone"`
	PhyStreet        string  `json:"phy_street"`
	PhyCity          string  `json:"phy_city"`
	PhyState         string  `json:"phy_state"`
	PhyZip           string  `json:"phy_zip"`
	PhyCountry       string  `json:"phy_country"`
	MailStreet       string  `json:"mail_street"`
	MailCity         string  `json:"mail_city"`
	MailState        string  `json:"mail_state"`
	MailZip          string  `json:"mail_zip"`
	MailCountry      string  `json:"mail_country"`
	CarrierOperation string  `json:"carrier_operation"`
	Classdef         string  `json:"classdef"`
	PowerUnits       int     `json:"power_units"`
	TotalDrivers     int     `json:"total_drivers"`
	IsCarrier        bool    `json:"is_carrier"`
	IsBroker         bool    `json:"is_broker"`
	McNumber         string  `json:"mc_number"`
	FfNumber         string  `json:"ff_number"`
	Score            float64 `json:"score"`
}

type HTTPValidationError struct {
	Detail []ValidationErrorDetail `json:"detail"`
}

type ValidationErrorDetail struct {
	Loc   []any          `json:"loc"`
	Msg   string         `json:"msg"`
	Type  string         `json:"type"`
	Input any            `json:"input,omitempty"`
	Ctx   map[string]any `json:"ctx,omitempty"`
}
