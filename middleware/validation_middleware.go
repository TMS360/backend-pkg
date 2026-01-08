package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/validation"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// ValidationMiddleware is a GraphQL middleware that collects validation errors
func ValidationMiddleware() graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (res interface{}, err error) {
		// Check if this is a mutation operation
		fieldContext := graphql.GetFieldContext(ctx)
		if fieldContext == nil {
			return next(ctx)
		}

		// Only apply validation collection to mutations
		operationContext := graphql.GetOperationContext(ctx)
		if operationContext == nil || operationContext.Operation == nil {
			return next(ctx)
		}

		// Check if this is a mutation
		if operationContext.Operation.Operation != "mutation" {
			return next(ctx)
		}

		// Check if this is a top-level mutation field
		if len(fieldContext.Path()) != 1 {
			return next(ctx)
		}

		// Add validation context to collect all errors
		ctx = validation.WithValidationContext(ctx, true)

		// Execute the resolver
		res, err = next(ctx)

		// Check if we collected any validation errors
		validationCtx := validation.GetValidationContext(ctx)
		if validationCtx != nil && validationCtx.HasErrors() {
			errors := validationCtx.GetErrors()

			// Group errors by field
			fieldErrors := make(map[string][]map[string]interface{})
			for _, err := range errors {
				errMap := map[string]interface{}{
					"field":      err.Field,
					"rule":       err.Rule,
					"value":      err.Value,
					"message":    err.Message,
					"constraint": err.Constraint,
				}
				fieldErrors[err.Field] = append(fieldErrors[err.Field], errMap)
			}

			// Create a structured error response
			allErrors := []map[string]interface{}{}
			for _, errs := range fieldErrors {
				for _, e := range errs {
					allErrors = append(allErrors, e)
				}
			}

			operationName := fieldContext.Field.Name
			return nil, &gqlerror.Error{
				Message: fmt.Sprintf("Validation failed for %s", operationName),
				Path:    graphql.GetPath(ctx),
				Extensions: map[string]interface{}{
					"code":      "VALIDATION_ERROR",
					"operation": operationName,
					"errors":    allErrors,
					"service":   "load_app",
				},
			}
		}

		return res, err
	}
}

// ExtractInputType extracts the input type name from field arguments
func ExtractInputType(ctx context.Context) string {
	fieldContext := graphql.GetFieldContext(ctx)
	if fieldContext == nil || len(fieldContext.Args) == 0 {
		return ""
	}

	for argName, argValue := range fieldContext.Args {
		if strings.Contains(strings.ToLower(argName), "input") && argValue != nil {
			typeName := fmt.Sprintf("%T", argValue)
			parts := strings.Split(typeName, ".")
			if len(parts) > 0 {
				return strings.TrimPrefix(parts[len(parts)-1], "*")
			}
		}
	}
	return ""
}
