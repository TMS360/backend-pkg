package validation

import (
	"context"
	"sync"
)

type validationContextKey struct{}

type ValidationContext struct {
	mu              sync.Mutex
	errors          []ValidationFieldError
	continueOnError bool
}

type ValidationFieldError struct {
	Field      string      `json:"field"`
	Rule       string      `json:"rule"`
	Value      interface{} `json:"value,omitempty"`
	Message    string      `json:"message"`
	Constraint string      `json:"constraint"`
	InputType  string      `json:"inputType,omitempty"`
}

func NewValidationContext(continueOnError bool) *ValidationContext {
	return &ValidationContext{
		errors:          make([]ValidationFieldError, 0),
		continueOnError: continueOnError,
	}
}

func (vc *ValidationContext) AddError(err ValidationFieldError) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.errors = append(vc.errors, err)
}

func (vc *ValidationContext) GetErrors() []ValidationFieldError {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return vc.errors
}

func (vc *ValidationContext) HasErrors() bool {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return len(vc.errors) > 0
}

func (vc *ValidationContext) ShouldContinue() bool {
	return vc.continueOnError
}

func WithValidationContext(ctx context.Context, continueOnError bool) context.Context {
	return context.WithValue(ctx, validationContextKey{}, NewValidationContext(continueOnError))
}

func GetValidationContext(ctx context.Context) *ValidationContext {
	if vc, ok := ctx.Value(validationContextKey{}).(*ValidationContext); ok {
		return vc
	}
	return nil
}
