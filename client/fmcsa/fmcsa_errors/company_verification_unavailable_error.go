package fmcsa_errors

// CompanyVerificationUnavailableError signals that FMCSA live verification could
// not be completed (e.g. the QCMobile upstream returned 5xx or timed out), so the
// company's operating authority is UNKNOWN — not confirmed invalid. Callers must
// surface this as a retry-later condition, never as "not authorized": treating an
// unverifiable company as unauthorized falsely blocks legitimate, active brokers
// whenever the FMCSA upstream is degraded.
type CompanyVerificationUnavailableError struct {
	Status int
}

func NewCompanyVerificationUnavailableError(status int) *CompanyVerificationUnavailableError {
	return &CompanyVerificationUnavailableError{Status: status}
}

// Error satisfies the error interface with a user-facing, retry-later message.
func (e *CompanyVerificationUnavailableError) Error() string {
	return "FMCSA verification is temporarily unavailable, please try again shortly"
}
