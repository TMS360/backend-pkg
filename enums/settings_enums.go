package enums

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

type CompanySettingsGeneralKey string

const (
	CompanySettingsGeneralKeyLogo                     CompanySettingsGeneralKey = "logo"
	CompanySettingsGeneralKeyHazmatEnabled            CompanySettingsGeneralKey = "hazmat_enabled"
	CompanySettingsGeneralKeyReeferEnabled            CompanySettingsGeneralKey = "reefer_enabled"
	CompanySettingsGeneralKeyBrokerHasVerifyShipments CompanySettingsGeneralKey = "broker_has_verify_shipments"
)

var AllCompanySettingsGeneralKey = []CompanySettingsGeneralKey{
	CompanySettingsGeneralKeyLogo,
	CompanySettingsGeneralKeyHazmatEnabled,
	CompanySettingsGeneralKeyReeferEnabled,
	CompanySettingsGeneralKeyBrokerHasVerifyShipments,
}

func (e CompanySettingsGeneralKey) IsValid() bool {
	switch e {
	case CompanySettingsGeneralKeyLogo, CompanySettingsGeneralKeyHazmatEnabled, CompanySettingsGeneralKeyReeferEnabled, CompanySettingsGeneralKeyBrokerHasVerifyShipments:
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
	CompanySettingsIntegrationKeyHereAPIKey    CompanySettingsIntegrationKey = "here_api_key"
	CompanySettingsIntegrationKeySamsaraAPIKey CompanySettingsIntegrationKey = "samsara_api_key"
)

var AllCompanySettingsIntegrationKey = []CompanySettingsIntegrationKey{
	CompanySettingsIntegrationKeyHereAPIKey,
	CompanySettingsIntegrationKeySamsaraAPIKey,
}

func (e CompanySettingsIntegrationKey) IsValid() bool {
	switch e {
	case CompanySettingsIntegrationKeyHereAPIKey, CompanySettingsIntegrationKeySamsaraAPIKey:
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
