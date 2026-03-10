package fmcsa_errors

import "fmt"

// MCVerificationError holds the context for a failed MC check
type MCVerificationError struct {
	Status   int
	MCNumber string
	Err      error // The underlying error (optional)
}

func NewMCVerificationError(status int, mcNumber string, err error) *MCVerificationError {
	return &MCVerificationError{
		Status:   status,
		MCNumber: mcNumber,
		Err:      err,
	}
}

// Error satisfies the error interface with a dynamic message
func (e *MCVerificationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to verify MC %s: %v", e.MCNumber, e.Err)
	}
	return fmt.Sprintf("failed to verify MC %s", e.MCNumber)
}

// Unwrap allows errors.Is and errors.As to see the underlying error
func (e *MCVerificationError) Unwrap() error {
	return e.Err
}
