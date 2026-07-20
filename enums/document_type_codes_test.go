package enums

import "testing"

// Every declared SystemDocTypeCode constant must be recognized by IsValid —
// a constant missing from the switch (as SystemDocShipmentOther was until
// DEV-1141) silently fails validation for a perfectly legitimate code.
func TestSystemDocTypeCode_IsValid_CoversAllConstants(t *testing.T) {
	all := []SystemDocTypeCode{
		SystemDocCertificateOfLiabilityInsurance,
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
		SystemDocInvoice,
		SystemDocShipmentOther,
		SystemPayStatement,
	}
	for _, c := range all {
		if !c.IsValid() {
			t.Fatalf("declared constant %q must be valid", c)
		}
	}
	if SystemDocTypeCode("NOT_A_CODE").IsValid() {
		t.Fatal("unknown code must be invalid")
	}
	if SystemDocRC.String() != "RC" {
		t.Fatalf("SystemDocRC must be the canonical \"RC\", got %q", SystemDocRC)
	}
}
