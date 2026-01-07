package tmsgraphql

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	_, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}

	return next(ctx)
}

// TODO: implement hasRole directive
func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, role string) (interface{}, error) {
	return next(ctx)
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if actor.Claims == nil {
		return nil, consts.ErrUnauthorized
	}

	for _, r := range actor.Claims.Roles {
		if r == role {
			return next(ctx)
		}
	}

	return nil, fmt.Errorf("access denied: missing role '%s'", role)
}

// TODO: implement hasPerm directive
func HasPermDirective(ctx context.Context, obj interface{}, next graphql.Resolver, perm string) (interface{}, error) {
	return next(ctx)
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if actor.Claims == nil {
		return nil, consts.ErrUnauthorized
	}

	for _, r := range actor.Claims.Permissions {
		if r == perm {
			return next(ctx)
		}
	}
	return next(ctx)
}

func ValidateDirective(v *validator.Validate) func(context.Context, interface{}, graphql.Resolver, string) (interface{}, error) {
	return func(ctx context.Context, obj interface{}, next graphql.Resolver, constraint string) (interface{}, error) {
		val, err := next(ctx)
		if err != nil {
			return nil, err
		}
		err = v.Var(val, constraint)
		if err != nil {
			return nil, fmt.Errorf("validation failed: %s", err.Error())
		}
		return val, nil
	}
}

type ValidationMessageStore interface {
	GetMessage(inputType, field, rule string) (string, bool)
}

func ValidateWithMessagesDirective(v *validator.Validate, messageStore ValidationMessageStore) func(context.Context, interface{}, graphql.Resolver, string) (interface{}, error) {
	return func(ctx context.Context, obj interface{}, next graphql.Resolver, constraint string) (interface{}, error) {
		val, err := next(ctx)
		if err != nil {
			return nil, err
		}

		err = v.Var(val, constraint)
		if err != nil {
			fieldInfo := extractFieldInfo(ctx)

			validationErrors := processValidationErrors(err, val, constraint, fieldInfo, messageStore)

			return nil, &gqlerror.Error{
				Message: fmt.Sprintf("Validation failed for field '%s'", fieldInfo.FieldName),
				Extensions: map[string]interface{}{
					"code":       "VALIDATION_ERROR",
					"field":      fieldInfo.FieldName,
					"inputType":  fieldInfo.InputType,
					"constraint": constraint,
					"errors":     validationErrors,
				},
			}
		}

		return val, nil
	}
}

func ValidateMessageDirective() func(context.Context, interface{}, graphql.Resolver, string, string) (interface{}, error) {
	return func(ctx context.Context, obj interface{}, next graphql.Resolver, rule string, message string) (interface{}, error) {
		return next(ctx)
	}
}

type FieldInfo struct {
	FieldName string
	InputType string
}

func extractFieldInfo(ctx context.Context) FieldInfo {
	info := FieldInfo{}

	fieldContext := graphql.GetFieldContext(ctx)
	if fieldContext == nil {
		return info
	}

	if fieldContext.Field.Field != nil {
		info.FieldName = fieldContext.Field.Field.Name
	}

	if path := fieldContext.Path(); path != nil {
		if key, ok := path.Key.(string); ok {
			info.FieldName = key
		}
	}

	if len(fieldContext.Args) > 0 {
		for argName, argValue := range fieldContext.Args {
			if strings.Contains(argName, "input") || strings.Contains(argName, "Input") {
				// Это наш input аргумент
				// Пытаемся получить тип из рефлексии
				if argValue != nil {
					typeName := fmt.Sprintf("%T", argValue)
					// Извлекаем имя типа (например, из "model.CreateTruckInput" получаем "CreateTruckInput")
					parts := strings.Split(typeName, ".")
					if len(parts) > 0 {
						lastPart := parts[len(parts)-1]
						// Убираем указатель если есть
						lastPart = strings.TrimPrefix(lastPart, "*")
						info.InputType = lastPart
					}
				}
				break
			}
		}
	}

	if info.InputType == "" && fieldContext.Field.ObjectDefinition != nil {
		defName := fieldContext.Field.ObjectDefinition.Name
		if strings.Contains(defName, "Input") {
			info.InputType = defName
		}
	}

	return info
}

type ValidationError struct {
	Field      string      `json:"field"`
	Rule       string      `json:"rule"`
	Value      interface{} `json:"value,omitempty"`
	Message    string      `json:"message"`
	Constraint string      `json:"constraint"`
}

func processValidationErrors(err error, value interface{}, constraint string, fieldInfo FieldInfo, messageStore ValidationMessageStore) []ValidationError {
	validationErrors := make([]ValidationError, 0)

	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return []ValidationError{{
			Field:      fieldInfo.FieldName,
			Rule:       "validation",
			Value:      value,
			Message:    err.Error(),
			Constraint: constraint,
		}}
	}

	for _, fe := range ve {
		valErr := ValidationError{
			Field:      fieldInfo.FieldName,
			Rule:       fe.Tag(),
			Value:      value,
			Constraint: constraint,
		}

		if messageStore != nil {
			if customMsg, found := messageStore.GetMessage(fieldInfo.InputType, fieldInfo.FieldName, fe.Tag()); found {
				valErr.Message = strings.ReplaceAll(customMsg, "{value}", fe.Param())
			} else {
				valErr.Message = getDefaultValidationMessage(fe.Tag(), fe.Param())
			}
		} else {
			valErr.Message = getDefaultValidationMessage(fe.Tag(), fe.Param())
		}

		validationErrors = append(validationErrors, valErr)
	}

	return validationErrors
}

func getDefaultValidationMessage(tag string, param string) string {
	switch tag {
	case "required":
		return "This field is required"
	case "min":
		return fmt.Sprintf("Minimum value is %s", param)
	case "max":
		return fmt.Sprintf("Maximum value is %s", param)
	case "len":
		return fmt.Sprintf("Length must be exactly %s", param)
	case "eq":
		return fmt.Sprintf("Must be equal to %s", param)
	case "ne":
		return fmt.Sprintf("Must not be equal to %s", param)
	case "gt":
		return fmt.Sprintf("Must be greater than %s", param)
	case "gte":
		return fmt.Sprintf("Must be greater than or equal to %s", param)
	case "lt":
		return fmt.Sprintf("Must be less than %s", param)
	case "lte":
		return fmt.Sprintf("Must be less than or equal to %s", param)
	case "email":
		return "Must be a valid email address"
	case "url":
		return "Must be a valid URL"
	case "alpha":
		return "Must contain only alphabetic characters"
	case "alphanum":
		return "Must contain only alphanumeric characters"
	case "numeric":
		return "Must contain only numeric characters"
	case "json":
		return "Must be valid JSON"
	default:
		return fmt.Sprintf("Failed validation on rule: %s", tag)
	}
}
