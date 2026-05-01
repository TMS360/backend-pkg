package enums

// SystemDocTypeCode is the stable, machine-readable identifier for a system
// document_types row (company_id IS NULL, is_system = true). It mirrors the
// row's `unique_name` column and is the value business logic across services
// should compare against — never the human-readable `name`.
type SystemDocTypeCode string

const (
	// GENERAL
	SystemDocCertificateOfLiabilityInsurance          SystemDocTypeCode = "certificate_of_liability_insurance"
	SystemDocCarbCertificate                          SystemDocTypeCode = "carb_certificate"
	SystemDocCTPermit                                 SystemDocTypeCode = "ct_permit"
	SystemDocDriverPolicyAndSafetyManual              SystemDocTypeCode = "driver_policy_and_safety_manual"
	SystemDocELDManual                                SystemDocTypeCode = "eld_manual"
	SystemDocIFTALicense                              SystemDocTypeCode = "ifta_license"
	SystemDocKYPermit                                 SystemDocTypeCode = "ky_permit"
	SystemDocMCAuthority                              SystemDocTypeCode = "mc_authority"
	SystemDocPSSpottedLanternflyPermit                SystemDocTypeCode = "ps_spotted_lanternfly_permit"
	SystemDocPSSpottedLanternflyQuarantineRegulations SystemDocTypeCode = "ps_spotted_lanternfly_quarantine_regulations"

	// TRUCK
	SystemDocNMPermit              SystemDocTypeCode = "nm_permit"
	SystemDocNYPermit              SystemDocTypeCode = "ny_permit"
	SystemDocORPermit              SystemDocTypeCode = "or_permit"
	SystemDocTruckAnnualInspection SystemDocTypeCode = "truck_annual_inspection"
	SystemDocTruckRegistration     SystemDocTypeCode = "truck_registration"
	SystemDocVehicleLeaseAgreement SystemDocTypeCode = "vehicle_lease_agreement"

	// TRAILER
	SystemDocTrailerAnnualInspection SystemDocTypeCode = "trailer_annual_inspection"
	SystemDocTrailerRegistration     SystemDocTypeCode = "trailer_registration"

	// SHIPMENT
	SystemDocRC      SystemDocTypeCode = "rc"
	SystemDocBOL     SystemDocTypeCode = "bol"
	SystemDocPOD     SystemDocTypeCode = "pod"
	SystemDocInvoice SystemDocTypeCode = "invoice"
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
		SystemDocRC,
		SystemDocBOL,
		SystemDocPOD,
		SystemDocInvoice:
		return true
	default:
		return false
	}
}
