package rcproc

import (
	"context"
	"io"
	"time"
)

type Client interface {
	Process(ctx context.Context, fileUrl string) (*RCProcessingResponse, error)
	GetStatus(ctx context.Context, requestID string) (*RCProcessingStatusResponse, error)
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
	RequestID     string           `json:"request_id"`
	Status        string           `json:"status"`
	Progress      int              `json:"progress"`
	Message       string           `json:"message"`
	CompanyID     string           `json:"company_id"`
	UserID        string           `json:"user_id"`
	Provider      string           `json:"provider"`
	FileURL       string           `json:"file_url"`
	Filename      string           `json:"filename"`
	FileSizeBytes int64            `json:"file_size_bytes"`
	CreatedAt     time.Time        `json:"created_at"`
	Data          *RateConResponse `json:"data"`
}

// RateConResponse is the top-level response from your OCR/AI service
type RateConResponse struct {
	// Extracted load data
	ExtractionStatus    string                 `json:"extraction_status"`
	ConfidenceScore     float64                `json:"confidence_score"`
	LoadDetails         LoadDetailsDTO         `json:"load_details"`
	Equipment           EquipmentDTO           `json:"equipment"`
	Commodities         []CommoditiesDTO       `json:"commodities"`
	Stops               []LoadStopDTO          `json:"stops"`
	BrokerContact       BrokerContactDTO       `json:"broker_contact"`
	SpecialInstructions string                 `json:"special_instructions"`
	PaymentTerms        PaymentTermsDTO        `json:"payment_terms"`
	CarrierRequirements CarrierRequirementsDTO `json:"carrier_requirements"`
}

type LoadDetailsDTO struct {
	LoadID              string   `json:"load_id"`
	ProNumber           *string  `json:"pro_number"`
	PoNumber            string   `json:"po_number"`
	ReferenceNumbers    []string `json:"reference_numbers"` // Changed to slice to handle []
	BolNumber           *string  `json:"bol_number"`
	ShipmentID          *string  `json:"shipment_id"`
	LoadPay             float64  `json:"load_pay"`
	Currency            string   `json:"currency"`
	DistanceMiles       *float64 `json:"distance_miles"`
	TotalWeight         float64  `json:"total_weight"`
	WeightUnit          string   `json:"weight_unit"`
	DetentionRate       *float64 `json:"detention_rate"`
	DetentionFreeTime   *int     `json:"detention_free_time"`
	LayoverRate         *float64 `json:"layover_rate"`
	TonuRate            *float64 `json:"tonu_rate"`
	LumperReimbursement *float64 `json:"lumper_reimbursement"`
	FuelSurcharge       *float64 `json:"fuel_surcharge"`
	FuelSurchargeType   *string  `json:"fuel_surcharge_type"`
}

type EquipmentDTO struct {
	Type                 string `json:"type"`
	TypeNormalized       string `json:"type_normalized"`
	Size                 string `json:"size"`
	LengthFeet           int    `json:"length_feet"`
	Requirements         string `json:"requirements"`
	Mode                 string `json:"mode"`
	TeamRequired         *bool  `json:"team_required"`
	Hazmat               bool   `json:"hazmat"`
	HazmatEndorsementReq bool   `json:"hazmat_endorsement_required"`
	TankerEndorsementReq bool   `json:"tanker_endorsement_required"`
	TwicCardRequired     *bool  `json:"twic_card_required"`

	// Nested Hazmat Details
	HazmatDetails HazmatDetailsDTO `json:"hazmat_details"`

	// Nested Temperature Requirements
	TemperatureRequirements TemperatureRequirementsDTO `json:"temperature_requirements"`

	FlatbedRequirements   *string  `json:"flatbed_requirements"`
	OversizeRequirements  *string  `json:"oversize_requirements"`
	TemperatureControlled bool     `json:"temperature_controlled"`
	TemperatureMin        *float64 `json:"temperature_min"`
	TemperatureMax        *float64 `json:"temperature_max"`
	TemperatureUnit       string   `json:"temperature_unit"`
	LiftGateRequired      *bool    `json:"lift_gate_required"`
	PalletJackRequired    *bool    `json:"pallet_jack_required"`
	LoadBarsRequired      *bool    `json:"load_bars_required"`
	ETrackRequired        *bool    `json:"e_track_required"`
	AirRideRequired       *bool    `json:"air_ride_required"`
	VentedTrailer         *bool    `json:"vented_trailer"`
	FoodGradeTrailer      bool     `json:"food_grade_trailer"`
}

type HazmatDetailsDTO struct {
	HazmatClass           *string  `json:"hazmat_class"`
	UnNumber              *string  `json:"un_number"`
	ProperShippingName    string   `json:"proper_shipping_name"`
	PackingGroup          *string  `json:"packing_group"`
	PlacardRequired       *bool    `json:"placard_required"`
	PlacardType           *string  `json:"placard_type"`
	EmergencyContactName  *string  `json:"emergency_contact_name"`
	EmergencyContactPhone *string  `json:"emergency_contact_phone"`
	ErgGuideNumber        *string  `json:"erg_guide_number"`
	ReportableQuantity    *bool    `json:"reportable_quantity"`
	MarinePollutant       *bool    `json:"marine_pollutant"`
	InhalationHazard      *bool    `json:"inhalation_hazard"`
	SpecialPermits        []string `json:"special_permits"` // Note: JSON shows an array []
}

type TemperatureRequirementsDTO struct {
	TemperatureMin              float64  `json:"temperature_min"`
	TemperatureMax              float64  `json:"temperature_max"`
	TemperatureSetpoint         float64  `json:"temperature_setpoint"`
	TemperatureUnit             string   `json:"temperature_unit"`
	ContinuousMode              *bool    `json:"continuous_mode"`
	CycleMode                   *bool    `json:"cycle_mode"`
	PrecoolRequired             *bool    `json:"precool_required"`
	PrecoolTemperature          *float64 `json:"precool_temperature"`
	PulpTemperatureRequired     *bool    `json:"pulp_temperature_required"`
	TemperatureRecorderRequired *bool    `json:"temperature_recorder_required"`
	FsmaCompliant               *bool    `json:"fsma_compliant"`
}

type CommoditiesDTO struct {
	Description        string   `json:"description"`
	PieceCount         int      `json:"piece_count"`
	PalletCount        *int     `json:"pallet_count"`
	HandlingUnit       string   `json:"handling_unit"`
	Weight             *float64 `json:"weight"`
	WeightUnit         *string  `json:"weight_unit"`
	Dimensions         *string  `json:"dimensions"` // Or a nested struct if the API expands this
	Hazmat             bool     `json:"hazmat"`
	HazmatClass        *string  `json:"hazmat_class"`
	UnNumber           *string  `json:"un_number"`
	PackingGroup       *string  `json:"packing_group"`
	ProperShippingName string   `json:"proper_shipping_name"`
	NmfcCode           *string  `json:"nmfc_code"`
	FreightClass       *string  `json:"freight_class"`
	Value              *float64 `json:"value"`
	Stackable          *bool    `json:"stackable"`
	Notes              *string  `json:"notes"`
}

type LoadStopDTO struct {
	Type                string  `json:"type"` // pickup or dropoff
	StopNumber          int32   `json:"stop_number"`
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
	ContactName         *string `json:"contact_name,omitempty"`
	ContactPhone        *string `json:"contact_phone,omitempty"`
	Notes               string  `json:"notes"`
}

type BrokerContactDTO struct {
	CompanyName     string  `json:"company_name"`
	ContactName     string  `json:"contact_name"`
	Phone           string  `json:"phone"`
	Email           string  `json:"email"`
	Fax             string  `json:"fax"`
	McNumber        *string `json:"mc_number"`
	DotNumber       *string `json:"dot_number"`
	Address         string  `json:"address"`
	AfterHoursPhone *string `json:"after_hours_phone"`
}

type PaymentTermsDTO struct {
	Terms              string   `json:"terms"`
	DaysToPay          int      `json:"days_to_pay"`
	QuickPayAvailable  bool     `json:"quick_pay_available"`
	QuickPayPercentage float64  `json:"quick_pay_percentage"`
	QuickPayDays       int      `json:"quick_pay_days"`
	FactoringAllowed   *bool    `json:"factoring_allowed"`
	InvoiceEmail       string   `json:"invoice_email"`
	PaymentMethod      *string  `json:"payment_method"`
	RequiredDocuments  []string `json:"required_documents"`
}

type CarrierRequirementsDTO struct {
	MinimumInsurance        *float64 `json:"minimum_insurance"`
	HazmatInsuranceRequired *bool    `json:"hazmat_insurance_required"`
	AutoLiabilityMinimum    *float64 `json:"auto_liability_minimum"`
	HazmatEndorsement       bool     `json:"hazmat_endorsement"`
	TankerEndorsement       bool     `json:"tanker_endorsement"`
	TwicCard                *bool    `json:"twic_card"`
	PassportRequired        *bool    `json:"passport_required"`
	FastCardRequired        *bool    `json:"fast_card_required"`
	EldRequired             *bool    `json:"eld_required"`
	GpsTrackingRequired     *bool    `json:"gps_tracking_required"`
	DashcamRequired         *bool    `json:"dashcam_required"`
	MinimumSafetyRating     *string  `json:"minimum_safety_rating"`
	CsaScoreRequirement     *string  `json:"csa_score_requirement"`
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
