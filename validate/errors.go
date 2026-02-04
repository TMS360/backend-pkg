package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type contextKey string

const validationErrorsKey contextKey = "validationErrors"

type FieldValidationError struct {
	Field    string   `json:"field"`
	Value    any      `json:"value,omitempty"`
	Rules    []string `json:"rules"`
	Messages []string `json:"messages"`
}

type ValidationErrors struct {
	mu       sync.RWMutex
	hasErrs  atomic.Bool             // lock-free flag for fast HasErrors() on the hot path
	Errors   []*FieldValidationError `json:"errors"`
	errorMap map[string]*FieldValidationError
}

func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{
		Errors:   make([]*FieldValidationError, 0),
		errorMap: make(map[string]*FieldValidationError),
	}
}

func (ve *ValidationErrors) Add(fieldError *FieldValidationError) {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	if existing, ok := ve.errorMap[fieldError.Field]; ok {
		existing.Rules = append(existing.Rules, fieldError.Rules...)
		existing.Messages = append(existing.Messages, fieldError.Messages...)
		existing.Value = fieldError.Value
	} else {
		ve.errorMap[fieldError.Field] = fieldError
		ve.Errors = append(ve.Errors, fieldError)
	}
	ve.hasErrs.Store(true)
}

func (ve *ValidationErrors) HasErrors() bool {
	return ve.hasErrs.Load()
}

func (ve *ValidationErrors) Error() string {
	ve.mu.RLock()
	defer ve.mu.RUnlock()

	if len(ve.Errors) == 0 {
		return ""
	}

	if len(ve.Errors) == 1 {
		return fmt.Sprintf("validation failed for field: %s", ve.Errors[0].Field)
	}
	return fmt.Sprintf("validation failed for %d fields", len(ve.Errors))
}

func (ve *ValidationErrors) ToArray() []*FieldValidationError {
	ve.mu.RLock()
	defer ve.mu.RUnlock()

	result := make([]*FieldValidationError, len(ve.Errors))
	copy(result, ve.Errors)
	return result
}

func WithValidationError(ctx context.Context, fieldError *FieldValidationError) context.Context {
	var errors *ValidationErrors

	if val := ctx.Value(validationErrorsKey); val != nil {
		errors = val.(*ValidationErrors)
	} else {
		errors = NewValidationErrors()
		ctx = context.WithValue(ctx, validationErrorsKey, errors)
	}

	errors.Add(fieldError)
	return ctx
}

func GetValidationErrors(ctx context.Context) *ValidationErrors {
	if val := ctx.Value(validationErrorsKey); val != nil {
		return val.(*ValidationErrors)
	}
	return nil
}

func ErrorPresenter() graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		if validationErrors := GetValidationErrors(ctx); validationErrors != nil && validationErrors.HasErrors() {
			return &gqlerror.Error{
				Message: "Validation failed",
				Extensions: map[string]interface{}{
					"code":             "VALIDATION_ERROR",
					"validationErrors": validationErrors.ToArray(),
				},
			}
		}

		if gqlErr, ok := err.(*gqlerror.Error); ok {
			return gqlErr
		}

		return graphql.DefaultErrorPresenter(ctx, err)
	}
}

func Middleware() graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		result, err := next(ctx)

		// Only check on resolver fields (root mutations/queries) where input
		// validation actually runs.  Skip trivial field accessors (Truck.id,
		// Truck.name, …) to avoid per-field context lookup + atomic load on
		// every single response field.
		if fc := graphql.GetFieldContext(ctx); fc != nil && fc.IsResolver {
			if ve := GetValidationErrors(ctx); ve != nil && ve.HasErrors() {
				return nil, ve
			}
		}

		return result, err
	}
}

func OperationMiddleware() graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		ctx = context.WithValue(ctx, validationErrorsKey, NewValidationErrors())

		return next(ctx)
	}
}

func (f *FieldValidationError) MarshalJSON() ([]byte, error) {
	type Alias FieldValidationError

	if f.Value == nil {
		return json.Marshal(&struct {
			Field    string   `json:"field"`
			Rules    []string `json:"rules"`
			Messages []string `json:"messages"`
		}{
			Field:    f.Field,
			Rules:    f.Rules,
			Messages: f.Messages,
		})
	}

	return json.Marshal((*Alias)(f))
}
