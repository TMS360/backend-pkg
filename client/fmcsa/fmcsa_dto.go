package fmcsa

import (
	"log/slog"
	"strings"
)

// SearchParams encapsulates query arguments for clean API usage.
type SearchParams struct {
	Query      string
	Limit      int
	Offset     int
	ActiveOnly bool
}

type SearchResponse struct {
	Query         string    `json:"query"`
	SearchType    string    `json:"search_type"`
	Count         int       `json:"count"`
	Results       []*Result `json:"results"`
	DataAvailable bool      `json:"data_available"`
}

type Result struct {
	DotNumber        int     `json:"dot_number"`
	LegalName        string  `json:"legal_name"`
	DbaName          string  `json:"dba_name"`
	EntityType       string  `json:"entity_type"`
	OperatingStatus  *string `json:"operating_status"`
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

	// --- Fields added for Search API compatibility ---
	StatusCode *string  `json:"status_code,omitempty"`
	Score      *float64 `json:"score,omitempty"`

	// --- Fields added for Detail API compatibility ---
	AllowedToOperate  *bool   `json:"allowed_to_operate,omitempty"`
	OutOfServiceDate  *string `json:"out_of_service_date,omitempty"`
	LiveDataAvailable *bool   `json:"live_data_available,omitempty"`
}

func (result *Result) IsValid() bool {
	if result.OperatingStatus == nil || strings.TrimSpace(strings.ToUpper(*result.OperatingStatus)) == "NOT AUTHORIZED" {
		slog.Error("FMCSA result is not authorized", "DOT", result.DotNumber, "MC", result.McNumber)
		return false
	}

	if result.AllowedToOperate != nil && *result.AllowedToOperate == false {
		slog.Error("FMCSA result is not allowed to operate", "DOT", result.DotNumber, "MC", result.McNumber)
		return false
	}

	return true
}

// CheckIsCarrier strictly verifies if the company operates as a carrier.
func (result *Result) CheckIsCarrier() bool {
	if result.IsCarrier {
		return true
	}
	return strings.Contains(strings.ToLower(result.EntityType), "carrier")
}

// CheckIsBroker strictly verifies if the company operates as a broker.
func (result *Result) CheckIsBroker() bool {
	if result.IsBroker {
		return true
	}
	return strings.Contains(strings.ToLower(result.EntityType), "broker")
}

// CheckIs strictly verifies if the company operates as the specified entity type (e.g., "carrier", "broker").
func (result *Result) CheckIs(entityType string) bool {
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	switch entityType {
	case "carrier":
		return result.CheckIsCarrier()
	case "broker":
		return result.CheckIsBroker()
	default:
		slog.Warn("Unknown entity type for CheckIs", "entity_type", entityType)
		return false
	}
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
