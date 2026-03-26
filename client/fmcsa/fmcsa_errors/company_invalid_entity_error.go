package fmcsa_errors

import (
	"fmt"
	"strings"
)

// CompanyInvalidEntityError holds the context for a failed MC check
type CompanyInvalidEntityError struct {
	Status       int
	Entity       string
	ActualEntity string
}

func NewCompanyInvalidEntityError(status int, entity, actualEntity string) *CompanyInvalidEntityError {
	return &CompanyInvalidEntityError{
		Status:       status,
		Entity:       entity,
		ActualEntity: actualEntity,
	}
}

// Error satisfies the error interface with a dynamic message
func (e *CompanyInvalidEntityError) Error() string {
	return fmt.Sprintf("entity must be %s: %s", strings.ToUpper(e.Entity), e.ActualEntity)
}

// Unwrap allows errors.Is and errors.As to see the underlying error
func (e *CompanyInvalidEntityError) Unwrap() error {
	return nil
}
