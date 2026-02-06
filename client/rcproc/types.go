package rcproc

import (
	"context"
	"io"
	"time"
)

type Client interface {
	Process(ctx context.Context, fileUrl, authToken string) (*RCProcessingResponse, error)
	GetStatus(ctx context.Context, requestID string) (*RateConResponse, error)
	ProcessSync(ctx context.Context, fileReader io.Reader, filename, contentType string) (*RateConResponse, error)
}

type RCProcessingRequest struct {
	FileURL  string `json:"file_url"`
	Provider string `json:"provider"`
}

type RCProcessingResponse struct {
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
	EstimatedSeconds int    `json:"estimated_seconds"`
	Message          string `json:"message"`
}

type RCProcessingStatusResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	Message   string    `json:"message"`
	FileURL   string    `json:"file_url"`
	Filename  string    `json:"filename"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
}

// RateConResponse is the top-level response from your OCR/AI service
type RateConResponse struct {
	// Status check response fields
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	Message   string    `json:"message"`
	FileURL   string    `json:"file_url"`
	Filename  string    `json:"filename"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`

	// Extracted load data
	ExtractionStatus    string           `json:"extraction_status"`
	ConfidenceScore     float64          `json:"confidence_score"`
	LoadDetails         LoadDetailsDTO   `json:"load_details"`
	Equipment           EquipmentDTO     `json:"equipment"`
	Commodities         CommoditiesDTO   `json:"commodities"`
	Stops               []LoadStopDTO    `json:"stops"`
	BrokerContact       BrokerContactDTO `json:"broker_contact"`
	SpecialInstructions string           `json:"special_instructions"`
	PaymentTerms        PaymentTermsDTO  `json:"payment_terms"`
}

type LoadDetailsDTO struct {
	LoadID           string  `json:"load_id"`
	ProNumber        *string `json:"pro_number"`
	PoNumber         string  `json:"po_number"`
	ReferenceNumbers string  `json:"reference_numbers"`
	BolNumber        string  `json:"bol_number"`
	LoadPay          float64 `json:"load_pay"`
	Currency         string  `json:"currency"`
	TotalWeight      float64 `json:"total_weight"`
	WeightUnit       string  `json:"weight_unit"`
}

type EquipmentDTO struct {
	Type                  string   `json:"type"`
	Size                  string   `json:"size"`
	Requirements          string   `json:"requirements"`
	TemperatureControlled bool     `json:"temperature_controlled"`
	TemperatureMin        *float64 `json:"temperature_min"`
	TemperatureMax        *float64 `json:"temperature_max"`
	TemperatureUnit       string   `json:"temperature_unit"`
}

type CommoditiesDTO struct {
	Description  string  `json:"description"`
	PieceCount   int     `json:"piece_count"`
	HandlingUnit string  `json:"handling_unit"`
	Notes        *string `json:"notes"`
}

type LoadStopDTO struct {
	Type                string  `json:"type"` // pickup or dropoff
	StopNumber          int     `json:"stop_number"`
	FacilityName        string  `json:"facility_name"`
	Address             string  `json:"address"`
	City                string  `json:"city"`
	State               string  `json:"state"`
	Zip                 string  `json:"zip"`
	Country             string  `json:"country"`
	StartDate           string  `json:"start_date"` // YYYY-MM-DD
	StartTime           string  `json:"start_time"` // HH:mm
	EndDate             string  `json:"end_date"`
	EndTime             string  `json:"end_time"`
	AppointmentRequired bool    `json:"appointment_required"`
	PickupNumber        *string `json:"pickup_number,omitempty"`
	DeliveryNumber      *string `json:"delivery_number,omitempty"`
	Notes               string  `json:"notes"`
}

type BrokerContactDTO struct {
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Fax         string `json:"fax"`
}

type PaymentTermsDTO struct {
	QuickPayAvailable bool   `json:"quick_pay_available"`
	InvoiceEmail      string `json:"invoice_email"`
}

// ValidationError represents the structure for 422 Unprocessable Entity
type ValidationError struct {
	Location []any  `json:"loc"`
	Message  string `json:"msg"`
	Type     string `json:"type"`
}

type HTTPValidationError struct {
	Detail []ValidationError `json:"detail"`
}

// BadRequestError represents the structure for a standard 400 Bad Request
type BadRequestError struct {
	Detail string `json:"detail"`
}
