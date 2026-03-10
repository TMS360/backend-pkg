package fmcsa_errors

import "fmt"

// DOTVerificationError holds the context for a failed MC check
type DOTVerificationError struct {
	Status    int
	DOTNumber string
	Err       error // The underlying error (optional)
}

func NewDOTVerificationError(status int, dotNumber string, err error) *DOTVerificationError {
	return &DOTVerificationError{
		Status:    status,
		DOTNumber: dotNumber,
		Err:       err,
	}
}

// Error satisfies the error interface with a dynamic message
func (e *DOTVerificationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to verify DOT %s: %v", e.DOTNumber, e.Err)
	}
	return fmt.Sprintf("failed to verify DOT %s", e.DOTNumber)
}

// Unwrap allows errors.Is and errors.As to see the underlying error
func (e *DOTVerificationError) Unwrap() error {
	return e.Err
}
