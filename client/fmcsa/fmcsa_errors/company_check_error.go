package fmcsa_errors

import "fmt"

// CompanyCheckError holds the context for a failed MC check
type CompanyCheckError struct {
	Status    int
	DotNumber string
	Err       error // The underlying error (optional)
}

func NewCompanyCheckError(status int, dotNumber string, err error) *CompanyCheckError {
	return &CompanyCheckError{
		Status:    status,
		DotNumber: dotNumber,
		Err:       err,
	}
}

// Error satisfies the error interface with a dynamic message
func (e *CompanyCheckError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed check company status for DOT %s: %v", e.DotNumber, e.Err)
	}
	return fmt.Sprintf("failed check company status for DOT %s", e.DotNumber)
}

// Unwrap allows errors.Is and errors.As to see the underlying error
func (e *CompanyCheckError) Unwrap() error {
	return e.Err
}
