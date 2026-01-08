package validate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/99designs/gqlgen/graphql"
)

var regexCache sync.Map

func compileRegex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexCache.Store(pattern, re)
	return re, nil
}

func Directive() func(ctx context.Context, obj interface{}, next graphql.Resolver,
	pattern *string, min *float64, max *float64,
	minLen *int32, maxLen *int32, minItems *int32, maxItems *int32,
	enum []string, email *bool, url *bool, uuid *bool,
	notEmpty *bool, alphanumeric *bool, numeric *bool, alpha *bool,
	contains *string, notContains *string, startsWith *string, endsWith *string,
	message *string,
	patternMessage *string, minMessage *string, maxMessage *string,
	minLenMessage *string, maxLenMessage *string,
	minItemsMessage *string, maxItemsMessage *string,
	enumMessage *string, emailMessage *string, urlMessage *string, uuidMessage *string,
	notEmptyMessage *string, alphanumericMessage *string, numericMessage *string, alphaMessage *string,
	containsMessage *string, notContainsMessage *string,
	startsWithMessage *string, endsWithMessage *string) (interface{}, error) {

	return func(ctx context.Context, obj interface{}, next graphql.Resolver,
		pattern *string, min *float64, max *float64,
		minLen *int32, maxLen *int32, minItems *int32, maxItems *int32,
		enum []string, email *bool, url *bool, uuid *bool,
		notEmpty *bool, alphanumeric *bool, numeric *bool, alpha *bool,
		contains *string, notContains *string, startsWith *string, endsWith *string,
		message *string,
		patternMessage *string, minMessage *string, maxMessage *string,
		minLenMessage *string, maxLenMessage *string,
		minItemsMessage *string, maxItemsMessage *string,
		enumMessage *string, emailMessage *string, urlMessage *string, uuidMessage *string,
		notEmptyMessage *string, alphanumericMessage *string, numericMessage *string, alphaMessage *string,
		containsMessage *string, notContainsMessage *string,
		startsWithMessage *string, endsWithMessage *string) (interface{}, error) {

		value, err := next(ctx)
		if err != nil {
			return nil, err
		}

		if value == nil {
			return value, nil
		}
		fieldContext := graphql.GetFieldContext(ctx)
		fieldName := ""
		if fieldContext != nil {
			if pathContext := graphql.GetPathContext(ctx); pathContext != nil {
				segments := []string{}
				current := pathContext
				for current != nil {
					if current.Field != nil {
						segments = append([]string{*current.Field}, segments...)
					} else if current.Index != nil {
						segments[len(segments)-1] = fmt.Sprintf("%s[%d]", segments[len(segments)-1], *current.Index)
					}
					current = current.Parent
				}
				if len(segments) > 0 {
					fieldName = strings.Join(segments, ".")
				}
			}
			if fieldName == "" {
				fieldName = fieldContext.Field.Name
			}
		}

		validationError := &FieldValidationError{
			Field:    fieldName,
			Value:    value,
			Rules:    []string{},
			Messages: []string{},
		}

		if email != nil && *email {
			if strVal, ok := value.(string); ok {
				emailRegex := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
				re, _ := compileRegex(emailRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "email")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must be a valid email address", fieldName))
					}
				}
			}
		}

		if url != nil && *url {
			if strVal, ok := value.(string); ok {
				urlRegex := `^(https?|ftp)://[^\s/$.?#].[^\s]*$`
				re, _ := compileRegex(urlRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "url")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must be a valid URL", fieldName))
					}
				}
			}
		}

		if uuid != nil && *uuid {
			if strVal, ok := value.(string); ok {
				uuidRegex := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
				re, _ := compileRegex(uuidRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "uuid")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must be a valid UUID", fieldName))
					}
				}
			}
		}

		if notEmpty != nil && *notEmpty {
			isEmpty := false
			switch v := value.(type) {
			case string:
				isEmpty = strings.TrimSpace(v) == ""
			case []interface{}:
				isEmpty = len(v) == 0
			}
			if isEmpty {
				validationError.Rules = append(validationError.Rules, "notEmpty")
				if message != nil && *message != "" {
					validationError.Messages = append(validationError.Messages, *message)
				} else {
					validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s cannot be empty", fieldName))
				}
			}
		}

		if alphanumeric != nil && *alphanumeric {
			if strVal, ok := value.(string); ok {
				alphaNumRegex := `^[a-zA-Z0-9]+$`
				re, _ := compileRegex(alphaNumRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "alphanumeric")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must contain only letters and numbers", fieldName))
					}
				}
			}
		}

		if numeric != nil && *numeric {
			if strVal, ok := value.(string); ok {
				numericRegex := `^[0-9]+$`
				re, _ := compileRegex(numericRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "numeric")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must contain only numbers", fieldName))
					}
				}
			}
		}

		if alpha != nil && *alpha {
			if strVal, ok := value.(string); ok {
				alphaRegex := `^[a-zA-Z]+$`
				re, _ := compileRegex(alphaRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "alpha")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must contain only letters", fieldName))
					}
				}
			}
		}

		if contains != nil {
			if strVal, ok := value.(string); ok {
				if !strings.Contains(strVal, *contains) {
					validationError.Rules = append(validationError.Rules, "contains")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must contain '%s'", fieldName, *contains))
					}
				}
			}
		}

		if notContains != nil {
			if strVal, ok := value.(string); ok {
				if strings.Contains(strVal, *notContains) {
					validationError.Rules = append(validationError.Rules, "notContains")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must not contain '%s'", fieldName, *notContains))
					}
				}
			}
		}

		if startsWith != nil {
			if strVal, ok := value.(string); ok {
				if !strings.HasPrefix(strVal, *startsWith) {
					validationError.Rules = append(validationError.Rules, "startsWith")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must start with '%s'", fieldName, *startsWith))
					}
				}
			}
		}

		if endsWith != nil {
			if strVal, ok := value.(string); ok {
				if !strings.HasSuffix(strVal, *endsWith) {
					validationError.Rules = append(validationError.Rules, "endsWith")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must end with '%s'", fieldName, *endsWith))
					}
				}
			}
		}

		if pattern != nil {
			if strVal, ok := value.(string); ok {
				re, err := compileRegex(*pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid regex pattern: %v", err)
				}
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "pattern")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages,
							fmt.Sprintf("%s does not match required pattern", fieldName))
					}
				}
			}
		}

		if min != nil {
			valid := false
			errorMsg := ""

			switch v := value.(type) {
			case int:
				if float64(v) < *min {
					errorMsg = fmt.Sprintf("%s must be at least %g", fieldName, *min)
				} else {
					valid = true
				}
			case int32:
				if float64(v) < *min {
					errorMsg = fmt.Sprintf("%s must be at least %g", fieldName, *min)
				} else {
					valid = true
				}
			case int64:
				if float64(v) < *min {
					errorMsg = fmt.Sprintf("%s must be at least %g", fieldName, *min)
				} else {
					valid = true
				}
			case float32:
				if float64(v) < *min {
					errorMsg = fmt.Sprintf("%s must be at least %g", fieldName, *min)
				} else {
					valid = true
				}
			case float64:
				if v < *min {
					errorMsg = fmt.Sprintf("%s must be at least %g", fieldName, *min)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "min")
				if message != nil && *message != "" {
					validationError.Messages = append(validationError.Messages, *message)
				} else {
					validationError.Messages = append(validationError.Messages, errorMsg)
				}
			}
		}

		if max != nil {
			valid := false
			errorMsg := ""

			switch v := value.(type) {
			case int:
				if float64(v) > *max {
					errorMsg = fmt.Sprintf("%s must be at most %g", fieldName, *max)
				} else {
					valid = true
				}
			case int32:
				if float64(v) > *max {
					errorMsg = fmt.Sprintf("%s must be at most %g", fieldName, *max)
				} else {
					valid = true
				}
			case int64:
				if float64(v) > *max {
					errorMsg = fmt.Sprintf("%s must be at most %g", fieldName, *max)
				} else {
					valid = true
				}
			case float32:
				if float64(v) > *max {
					errorMsg = fmt.Sprintf("%s must be at most %g", fieldName, *max)
				} else {
					valid = true
				}
			case float64:
				if v > *max {
					errorMsg = fmt.Sprintf("%s must be at most %g", fieldName, *max)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "max")
				if message != nil && *message != "" {
					validationError.Messages = append(validationError.Messages, *message)
				} else {
					validationError.Messages = append(validationError.Messages, errorMsg)
				}
			}
		}

		if minLen != nil {
			valid := false
			errorMsg := ""
			actualLen := 0

			switch v := value.(type) {
			case string:
				actualLen = len(v)
				if actualLen < int(*minLen) {
					errorMsg = fmt.Sprintf("%s must be at least %d characters", fieldName, *minLen)
				} else {
					valid = true
				}
			case []interface{}:
				actualLen = len(v)
				if actualLen < int(*minLen) {
					errorMsg = fmt.Sprintf("%s must have at least %d items", fieldName, *minLen)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "minLen")
				if message != nil && *message != "" {
					validationError.Messages = append(validationError.Messages, *message)
				} else {
					validationError.Messages = append(validationError.Messages, errorMsg)
				}
			}
		}

		if maxLen != nil {
			valid := false
			errorMsg := ""
			actualLen := 0

			switch v := value.(type) {
			case string:
				actualLen = len(v)
				if actualLen > int(*maxLen) {
					errorMsg = fmt.Sprintf("%s must be at most %d characters", fieldName, *maxLen)
				} else {
					valid = true
				}
			case []interface{}:
				actualLen = len(v)
				if actualLen > int(*maxLen) {
					errorMsg = fmt.Sprintf("%s must have at most %d items", fieldName, *maxLen)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "maxLen")
				if message != nil && *message != "" {
					validationError.Messages = append(validationError.Messages, *message)
				} else {
					validationError.Messages = append(validationError.Messages, errorMsg)
				}
			}
		}

		if minItems != nil {
			if arrVal, ok := value.([]interface{}); ok {
				if len(arrVal) < int(*minItems) {
					validationError.Rules = append(validationError.Rules, "minItems")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must have at least %d items", fieldName, *minItems))
					}
				}
			}
		}

		if maxItems != nil {
			if arrVal, ok := value.([]interface{}); ok {
				if len(arrVal) > int(*maxItems) {
					validationError.Rules = append(validationError.Rules, "maxItems")
					if message != nil && *message != "" {
						validationError.Messages = append(validationError.Messages, *message)
					} else {
						validationError.Messages = append(validationError.Messages, fmt.Sprintf("%s must have at most %d items", fieldName, *maxItems))
					}
				}
			}
		}

		if len(enum) > 0 {
			strVal := fmt.Sprintf("%v", value)
			found := false
			for _, allowed := range enum {
				if strVal == allowed {
					found = true
					break
				}
			}

			if !found {
				validationError.Rules = append(validationError.Rules, "enum")
				if message != nil && *message != "" {
					validationError.Messages = append(validationError.Messages, *message)
				} else {
					validationError.Messages = append(validationError.Messages,
						fmt.Sprintf("%s must be one of: %s", fieldName, strings.Join(enum, ", ")))
				}
			}
		}

		if len(validationError.Rules) > 0 {
			ctx = WithValidationError(ctx, validationError)
		}

		return value, nil
	}
}
