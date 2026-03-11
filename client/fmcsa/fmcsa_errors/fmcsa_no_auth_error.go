package fmcsa_errors

// CompanyNoAuthError holds the context for a failed MC check
type CompanyNoAuthError struct {
	Status int
}

func NewCompanyNoAuthError(status int) *CompanyNoAuthError {
	return &CompanyNoAuthError{Status: status}
}

// Error satisfies the error interface with a dynamic message
func (e *CompanyNoAuthError) Error() string {
	return "current Company Operating Authority Status is not valid for registration"
}
