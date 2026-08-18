package enums

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

type CompanySettingsGeneralKey string

const (
	CompanySettingsGeneralKeyLogo                      CompanySettingsGeneralKey = "logo"
	CompanySettingsGeneralKeyTimezone                  CompanySettingsGeneralKey = "timezone"
	CompanySettingsGeneralKeyHazmatEnabled             CompanySettingsGeneralKey = "hazmat_enabled"
	CompanySettingsGeneralKeyReeferEnabled             CompanySettingsGeneralKey = "reefer_enabled"
	CompanySettingsGeneralKeyBrokerHasVerifyShipments  CompanySettingsGeneralKey = "broker_has_verify_shipments"
	CompanySettingsGeneralKeyTripAssignmentBufferHours CompanySettingsGeneralKey = "trip_assignment_buffer_hours"
	// CompanySettingsGeneralKeySamsaraAssetTrackingEnabled decides where recorded
	// mileage comes from: Samsara GPS actual when on (default), HERE road-distance
	// estimate when off. Default-on preserves current behaviour for existing tenants.
	CompanySettingsGeneralKeySamsaraAssetTrackingEnabled CompanySettingsGeneralKey = "samsara_asset_tracking_enabled"
	// CompanySettingsGeneralKeyUseHereInRisk decides whether the trip risk worker
	// may make the automatic (paid) HERE routing call when a trip looks high-risk.
	// Default OFF: risk scoring still works off the free required-speed estimate and
	// the ETA stays a placeholder until an admin opts in. Per-company.
	CompanySettingsGeneralKeyUseHereInRisk CompanySettingsGeneralKey = "use_here_in_risk"
	// CompanySettingsGeneralKeyEmptyMilesWorkflow decides when a trip's empty
	// (deadhead) miles get written. "auto" keeps the historical behaviour — every
	// lifecycle step that can change the deadhead origin recomputes and persists
	// the number. "deferred" leaves empty miles NULL until dispatch has checked the
	// origin and explicitly calculated, so nothing invents a mileage the driver
	// then gets paid on (DEV-1573/DEV-1577). Missing row reads as "auto": tenants
	// that predate the setting must not change behaviour on deploy.
	CompanySettingsGeneralKeyEmptyMilesWorkflow CompanySettingsGeneralKey = "empty_miles_workflow"
)

var AllCompanySettingsGeneralKey = []CompanySettingsGeneralKey{
	CompanySettingsGeneralKeyLogo,
	CompanySettingsGeneralKeyTimezone,
	CompanySettingsGeneralKeyHazmatEnabled,
	CompanySettingsGeneralKeyReeferEnabled,
	CompanySettingsGeneralKeyBrokerHasVerifyShipments,
	CompanySettingsGeneralKeyTripAssignmentBufferHours,
	CompanySettingsGeneralKeySamsaraAssetTrackingEnabled,
	CompanySettingsGeneralKeyUseHereInRisk,
	CompanySettingsGeneralKeyEmptyMilesWorkflow,
}

func (e CompanySettingsGeneralKey) IsValid() bool {
	switch e {
	case CompanySettingsGeneralKeyLogo, CompanySettingsGeneralKeyTimezone, CompanySettingsGeneralKeyHazmatEnabled, CompanySettingsGeneralKeyReeferEnabled, CompanySettingsGeneralKeyBrokerHasVerifyShipments, CompanySettingsGeneralKeyTripAssignmentBufferHours, CompanySettingsGeneralKeySamsaraAssetTrackingEnabled, CompanySettingsGeneralKeyUseHereInRisk, CompanySettingsGeneralKeyEmptyMilesWorkflow:
		return true
	}
	return false
}

func (e CompanySettingsGeneralKey) String() string {
	return string(e)
}

func (e *CompanySettingsGeneralKey) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = CompanySettingsGeneralKey(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid CompanySettingsGeneralKey", str)
	}
	return nil
}

func (e CompanySettingsGeneralKey) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

func (e *CompanySettingsGeneralKey) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	return e.UnmarshalGQL(s)
}

func (e CompanySettingsGeneralKey) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	e.MarshalGQL(&buf)
	return buf.Bytes(), nil
}

type CompanySettingsIntegrationKey string

const (
	CompanySettingsIntegrationKeyHereAPIKey      CompanySettingsIntegrationKey = "here_api_key"
	CompanySettingsIntegrationKeySamsaraAPIKey   CompanySettingsIntegrationKey = "samsara_api_key"
	CompanySettingsIntegrationKeyRelayAPIKey     CompanySettingsIntegrationKey = "relay_api_key"
	CompanySettingsIntegrationKeyUSPSCredentials CompanySettingsIntegrationKey = "usps_credentials"
	// CompanySettingsIntegrationKeyGoogleMapsAPIKey is the tenant's Google Maps
	// Platform key, used only as a fallback after a classified HERE failure.
	CompanySettingsIntegrationKeyGoogleMapsAPIKey CompanySettingsIntegrationKey = "google_maps_api_key"
	// CompanySettingsIntegrationKeyGoogleSheetsAPIKey is the tenant's Google API
	// key for the Sheets connector (DEV-1723): a board reads a spreadsheet range
	// through it. Deliberately SEPARATE from the Maps key — the two are different
	// integrations with different lifecycles, and sharing one credential would mean
	// revoking a spreadsheet also breaks geocoding. The Sheets API reads only
	// link-shared spreadsheets with an API key; a private sheet needs a service
	// account, which is a different credential shape and a later ticket.
	CompanySettingsIntegrationKeyGoogleSheetsAPIKey CompanySettingsIntegrationKey = "google_sheets_api_key"
	// CompanySettingsIntegrationKeyRingCentralCredentials is the tenant's own
	// RingCentral phone system (DEV-1751). Multi-field like usps_credentials: the
	// value is a JSON object (client id/secret + the JWT credential, plus an
	// optional sandbox host) read back through provider.JSONClientProvider, not a
	// bare API key. One row per company — a RingCentral account is never shared
	// across tenants.
	CompanySettingsIntegrationKeyRingCentralCredentials CompanySettingsIntegrationKey = "ringcentral_credentials"
)

var AllCompanySettingsIntegrationKey = []CompanySettingsIntegrationKey{
	CompanySettingsIntegrationKeyHereAPIKey,
	CompanySettingsIntegrationKeySamsaraAPIKey,
	CompanySettingsIntegrationKeyRelayAPIKey,
	CompanySettingsIntegrationKeyUSPSCredentials,
	CompanySettingsIntegrationKeyGoogleMapsAPIKey,
	CompanySettingsIntegrationKeyGoogleSheetsAPIKey,
	CompanySettingsIntegrationKeyRingCentralCredentials,
}

func (e CompanySettingsIntegrationKey) IsValid() bool {
	switch e {
	case CompanySettingsIntegrationKeyHereAPIKey,
		CompanySettingsIntegrationKeySamsaraAPIKey,
		CompanySettingsIntegrationKeyRelayAPIKey,
		CompanySettingsIntegrationKeyUSPSCredentials,
		CompanySettingsIntegrationKeyGoogleMapsAPIKey,
		CompanySettingsIntegrationKeyGoogleSheetsAPIKey,
		CompanySettingsIntegrationKeyRingCentralCredentials:
		return true
	}
	return false
}

func (e CompanySettingsIntegrationKey) String() string {
	return string(e)
}

func (e *CompanySettingsIntegrationKey) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = CompanySettingsIntegrationKey(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid CompanySettingsIntegrationKey", str)
	}
	return nil
}

func (e CompanySettingsIntegrationKey) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

func (e *CompanySettingsIntegrationKey) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	return e.UnmarshalGQL(s)
}

func (e CompanySettingsIntegrationKey) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	e.MarshalGQL(&buf)
	return buf.Bytes(), nil
}

type CompanySettingsScoringKey string

const (
	CompanySettingsScoringKeyScoringSamsaraWeight  CompanySettingsScoringKey = "scoring_samsara_weight"
	CompanySettingsScoringKeyScoringInternalWeight CompanySettingsScoringKey = "scoring_internal_weight"
)

var AllCompanySettingsScoringKey = []CompanySettingsScoringKey{
	CompanySettingsScoringKeyScoringSamsaraWeight,
	CompanySettingsScoringKeyScoringInternalWeight,
}

func (e CompanySettingsScoringKey) IsValid() bool {
	switch e {
	case CompanySettingsScoringKeyScoringSamsaraWeight, CompanySettingsScoringKeyScoringInternalWeight:
		return true
	}
	return false
}

func (e CompanySettingsScoringKey) String() string {
	return string(e)
}

func (e *CompanySettingsScoringKey) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = CompanySettingsScoringKey(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid CompanySettingsScoringKey", str)
	}
	return nil
}

func (e CompanySettingsScoringKey) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

func (e *CompanySettingsScoringKey) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	return e.UnmarshalGQL(s)
}

func (e CompanySettingsScoringKey) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	e.MarshalGQL(&buf)
	return buf.Bytes(), nil
}

type CompanySettingsSplitTripKey string

const (
	// CompanySettingsSplitTripKeyMaxRecoveryRadiusMiles is the soft radius (miles)
	// used to score drivers when searching for a recovery driver during a split.
	// Default 250 when unset.
	CompanySettingsSplitTripKeyMaxRecoveryRadiusMiles CompanySettingsSplitTripKey = "max_recovery_radius_miles"
	// CompanySettingsSplitTripKeySplitMarginWarningThreshold warns when the total
	// driver pay exceeds load pay by more than this margin. Default 0 when unset.
	CompanySettingsSplitTripKeySplitMarginWarningThreshold CompanySettingsSplitTripKey = "split_margin_warning_threshold"
	// CompanySettingsSplitTripKeyDeadheadRatePerMile overrides the per-mile deadhead
	// rate. Unset (null) means use the driver's own rate.
	CompanySettingsSplitTripKeyDeadheadRatePerMile CompanySettingsSplitTripKey = "deadhead_rate_per_mile"
)

var AllCompanySettingsSplitTripKey = []CompanySettingsSplitTripKey{
	CompanySettingsSplitTripKeyMaxRecoveryRadiusMiles,
	CompanySettingsSplitTripKeySplitMarginWarningThreshold,
	CompanySettingsSplitTripKeyDeadheadRatePerMile,
}

func (e CompanySettingsSplitTripKey) IsValid() bool {
	switch e {
	case CompanySettingsSplitTripKeyMaxRecoveryRadiusMiles,
		CompanySettingsSplitTripKeySplitMarginWarningThreshold,
		CompanySettingsSplitTripKeyDeadheadRatePerMile:
		return true
	}
	return false
}

func (e CompanySettingsSplitTripKey) String() string {
	return string(e)
}

func (e *CompanySettingsSplitTripKey) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = CompanySettingsSplitTripKey(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid CompanySettingsSplitTripKey", str)
	}
	return nil
}

func (e CompanySettingsSplitTripKey) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

func (e *CompanySettingsSplitTripKey) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	return e.UnmarshalGQL(s)
}

func (e CompanySettingsSplitTripKey) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	e.MarshalGQL(&buf)
	return buf.Bytes(), nil
}
