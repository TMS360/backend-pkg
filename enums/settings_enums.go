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
)

var AllCompanySettingsGeneralKey = []CompanySettingsGeneralKey{
	CompanySettingsGeneralKeyLogo,
	CompanySettingsGeneralKeyHazmatEnabled,
	CompanySettingsGeneralKeyReeferEnabled,
	CompanySettingsGeneralKeyBrokerHasVerifyShipments,
	CompanySettingsGeneralKeyTripAssignmentBufferHours,
	CompanySettingsGeneralKeySamsaraAssetTrackingEnabled,
	CompanySettingsGeneralKeyUseHereInRisk,
}

func (e CompanySettingsGeneralKey) IsValid() bool {
	switch e {
	case CompanySettingsGeneralKeyLogo, CompanySettingsGeneralKeyHazmatEnabled, CompanySettingsGeneralKeyReeferEnabled, CompanySettingsGeneralKeyBrokerHasVerifyShipments, CompanySettingsGeneralKeyTripAssignmentBufferHours, CompanySettingsGeneralKeySamsaraAssetTrackingEnabled, CompanySettingsGeneralKeyUseHereInRisk:
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
)

var AllCompanySettingsIntegrationKey = []CompanySettingsIntegrationKey{
	CompanySettingsIntegrationKeyHereAPIKey,
	CompanySettingsIntegrationKeySamsaraAPIKey,
	CompanySettingsIntegrationKeyRelayAPIKey,
	CompanySettingsIntegrationKeyUSPSCredentials,
}

func (e CompanySettingsIntegrationKey) IsValid() bool {
	switch e {
	case CompanySettingsIntegrationKeyHereAPIKey,
		CompanySettingsIntegrationKeySamsaraAPIKey,
		CompanySettingsIntegrationKeyRelayAPIKey,
		CompanySettingsIntegrationKeyUSPSCredentials:
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
