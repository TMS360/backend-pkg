package enums

// SystemDocTypeCode is the stable, machine-readable identifier for a system
// document_types row (company_id IS NULL, is_system = true). It mirrors the
// row's `unique_name` column and is the value business logic across services
// should compare against — never the human-readable `name`.
type SystemDocTypeCode string

const (
	// GENERAL
	SystemDocCertificateOfLiabilityInsurance          SystemDocTypeCode = "CERTIFICATE_OF_LIABILITY_INSURANCE"
	SystemDocCarbCertificate                          SystemDocTypeCode = "CARB_CERTIFICATE"
	SystemDocCTPermit                                 SystemDocTypeCode = "CT_PERMIT"
	SystemDocDriverPolicyAndSafetyManual              SystemDocTypeCode = "DRIVER_POLICY_AND_SAFETY_MANUAL"
	SystemDocELDManual                                SystemDocTypeCode = "ELD_MANUAL"
	SystemDocIFTALicense                              SystemDocTypeCode = "IFTA_LICENSE"
	SystemDocKYPermit                                 SystemDocTypeCode = "KY_PERMIT"
	SystemDocMCAuthority                              SystemDocTypeCode = "MC_AUTHORITY"
	SystemDocPSSpottedLanternflyPermit                SystemDocTypeCode = "PS_SPOTTED_LANTERNFLY_PERMIT"
	SystemDocPSSpottedLanternflyQuarantineRegulations SystemDocTypeCode = "PS_SPOTTED_LANTERNFLY_QUARANTINE_REGULATIONS"

	// TRUCK
	SystemDocNMPermit              SystemDocTypeCode = "NM_PERMIT"
	SystemDocNYPermit              SystemDocTypeCode = "NY_PERMIT"
	SystemDocORPermit              SystemDocTypeCode = "OR_PERMIT"
	SystemDocTruckAnnualInspection SystemDocTypeCode = "TRUCK_ANNUAL_INSPECTION"
	SystemDocTruckRegistration     SystemDocTypeCode = "TRUCK_REGISTRATION"
	SystemDocVehicleLeaseAgreement SystemDocTypeCode = "VEHICLE_LEASE_AGREEMENT"

	// TRAILER
	SystemDocTrailerAnnualInspection SystemDocTypeCode = "TRAILER_ANNUAL_INSPECTION"
	SystemDocTrailerRegistration     SystemDocTypeCode = "TRAILER_REGISTRATION"
	SystemDocTrailerInsurance        SystemDocTypeCode = "TRAILER_INSURANCE"
	SystemDocTrailerTitle            SystemDocTypeCode = "TRAILER_TITLE"

	// SHIPMENT
	SystemDocRC            SystemDocTypeCode = "RC"
	SystemDocBOL           SystemDocTypeCode = "BOL"
	SystemDocPOD           SystemDocTypeCode = "POD"
	SystemDocInvoice       SystemDocTypeCode = "INVOICE"
	SystemDocShipmentOther SystemDocTypeCode = "SHIPMENT_OTHER"

	// DRIVER — the FMCSA driver qualification file (49 CFR 391.51) plus payroll.
	SystemPayStatement                SystemDocTypeCode = "PAY_STATEMENT"
	SystemDocDriverLicense            SystemDocTypeCode = "DRIVER_LICENSE"
	SystemDocDriverMedicalCard        SystemDocTypeCode = "DRIVER_MEDICAL_CARD"
	SystemDocDriverMVR                SystemDocTypeCode = "DRIVER_MVR"
	SystemDocDriverRoadTest           SystemDocTypeCode = "DRIVER_ROAD_TEST"
	SystemDocDriverApplication        SystemDocTypeCode = "DRIVER_APPLICATION"
	SystemDocDriverPreviousEmployment SystemDocTypeCode = "DRIVER_PREVIOUS_EMPLOYMENT"
)

func (c SystemDocTypeCode) String() string { return string(c) }

func (c SystemDocTypeCode) IsValid() bool {
	switch c {
	case SystemDocCertificateOfLiabilityInsurance,
		SystemDocCarbCertificate,
		SystemDocCTPermit,
		SystemDocDriverPolicyAndSafetyManual,
		SystemDocELDManual,
		SystemDocIFTALicense,
		SystemDocKYPermit,
		SystemDocMCAuthority,
		SystemDocPSSpottedLanternflyPermit,
		SystemDocPSSpottedLanternflyQuarantineRegulations,
		SystemDocNMPermit,
		SystemDocNYPermit,
		SystemDocORPermit,
		SystemDocTruckAnnualInspection,
		SystemDocTruckRegistration,
		SystemDocVehicleLeaseAgreement,
		SystemDocTrailerAnnualInspection,
		SystemDocTrailerRegistration,
		SystemDocTrailerInsurance,
		SystemDocTrailerTitle,
		SystemDocRC,
		SystemDocBOL,
		SystemDocPOD,
		SystemDocInvoice,
		SystemDocShipmentOther,
		SystemPayStatement,
		SystemDocDriverLicense,
		SystemDocDriverMedicalCard,
		SystemDocDriverMVR,
		SystemDocDriverRoadTest,
		SystemDocDriverApplication,
		SystemDocDriverPreviousEmployment:
		return true
	default:
		return false
	}
}
