package validate

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/cache"
)

// cachedUserRoles mirrors the structure stored in Redis by the auth service.
type cachedUserRoles struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
}

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

// resolveMessage returns the first non-empty message in priority:
// ruleMsg (e.g. emailMessage) → globalMsg (message) → defaultMsg.
func resolveMessage(ruleMsg, globalMsg *string, defaultMsg string) string {
	if ruleMsg != nil && *ruleMsg != "" {
		return *ruleMsg
	}
	if globalMsg != nil && *globalMsg != "" {
		return *globalMsg
	}
	return defaultMsg
}

// luhnCheck implements the Luhn algorithm for credit card number validation.
func luhnCheck(number string) bool {
	cleaned := strings.NewReplacer(" ", "", "-", "").Replace(number)
	if len(cleaned) < 13 || len(cleaned) > 19 {
		return false
	}
	sum := 0
	alt := false
	for i := len(cleaned) - 1; i >= 0; i-- {
		n := int(cleaned[i] - '0')
		if n < 0 || n > 9 {
			return false
		}
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

// toFloat64 attempts to convert a value to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

// compareValues compares two values. Returns -1, 0, or 1.
// Tries numeric comparison first, then falls back to string comparison.
func compareValues(a, b interface{}) int {
	af, aOk := toFloat64(a)
	bf, bOk := toFloat64(b)
	if aOk && bOk {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}

	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	default:
		return 0
	}
}

// isFieldPresent checks if a field exists and is non-nil in the object map.
func isFieldPresent(obj interface{}, field string) bool {
	if m, ok := obj.(map[string]interface{}); ok {
		val, exists := m[field]
		return exists && val != nil
	}
	return false
}

// getFieldValue returns the value of a field from the object map.
func getFieldValue(obj interface{}, field string) (interface{}, bool) {
	if m, ok := obj.(map[string]interface{}); ok {
		val, exists := m[field]
		return val, exists
	}
	return nil, false
}

// isEmptyValue returns true for nil, empty string, or empty slice.
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val) == ""
	case []interface{}:
		return len(val) == 0
	}
	return false
}

// derefPointer unwraps pointer types that gqlgen uses for nullable/optional
// fields (e.g. *string, *int32, *float64) into their underlying values.
// Returns nil for nil pointers, the dereferenced value otherwise.
func derefPointer(v interface{}) interface{} {
	switch ptr := v.(type) {
	case *string:
		if ptr == nil {
			return nil
		}
		return *ptr
	case *int:
		if ptr == nil {
			return nil
		}
		return *ptr
	case *int32:
		if ptr == nil {
			return nil
		}
		return *ptr
	case *int64:
		if ptr == nil {
			return nil
		}
		return *ptr
	case *float32:
		if ptr == nil {
			return nil
		}
		return *ptr
	case *float64:
		if ptr == nil {
			return nil
		}
		return *ptr
	case *bool:
		if ptr == nil {
			return nil
		}
		return *ptr
	default:
		return v
	}
}

// Length helpers used inside the Directive closure where the built-in len
// is shadowed by the 'len' parameter.
func stringLen(s string) int       { return len(s) }
func sliceLen(s []interface{}) int { return len(s) }
func strSliceLen(s []string) int   { return len(s) }

func Directive() func(ctx context.Context, obj interface{}, next graphql.Resolver,
	pattern *string, min *float64, max *float64,
	minLen *int32, maxLen *int32, minItems *int32, maxItems *int32,
	enum []string, email *bool, url *bool, uuid *bool,
	notEmpty *bool, alphanumeric *bool, numeric *bool, alpha *bool,
	contains *string, notContains *string, startsWith *string, endsWith *string,
	ip *bool, ipv4 *bool, ipv6 *bool, mac *bool, cidr *bool,
	hostname *bool, fqdn *bool, base64 *bool, jsonValid *bool, jwt *bool,
	lowercase *bool, uppercase *bool, creditCard *bool,
	latitude *bool, longitude *bool, timezone *bool,
	eq *float64, ne *float64, gt *float64, lt *float64,
	len *int32,
	eqField *string, neField *string, gtField *string, ltField *string,
	requiredWith *string, requiredWithout *string, excludedWith *string,
	userExist *bool, userHasRole *string,
	message *string,
	patternMessage *string, minMessage *string, maxMessage *string,
	minLenMessage *string, maxLenMessage *string,
	minItemsMessage *string, maxItemsMessage *string,
	enumMessage *string, emailMessage *string, urlMessage *string, uuidMessage *string,
	notEmptyMessage *string, alphanumericMessage *string, numericMessage *string, alphaMessage *string,
	containsMessage *string, notContainsMessage *string,
	startsWithMessage *string, endsWithMessage *string,
	ipMessage *string, ipv4Message *string, ipv6Message *string, macMessage *string, cidrMessage *string,
	hostnameMessage *string, fqdnMessage *string, base64Message *string, jsonValidMessage *string, jwtMessage *string,
	lowercaseMessage *string, uppercaseMessage *string, creditCardMessage *string,
	latitudeMessage *string, longitudeMessage *string, timezoneMessage *string,
	eqMessage *string, neMessage *string, gtMessage *string, ltMessage *string,
	lenMessage *string,
	eqFieldMessage *string, neFieldMessage *string, gtFieldMessage *string, ltFieldMessage *string,
	requiredWithMessage *string, requiredWithoutMessage *string, excludedWithMessage *string,
	userExistMessage *string, userHasRoleMessage *string,
) (interface{}, error) {

	return func(ctx context.Context, obj interface{}, next graphql.Resolver,
		pattern *string, min *float64, max *float64,
		minLen *int32, maxLen *int32, minItems *int32, maxItems *int32,
		enum []string, email *bool, url *bool, uuid *bool,
		notEmpty *bool, alphanumeric *bool, numeric *bool, alpha *bool,
		contains *string, notContains *string, startsWith *string, endsWith *string,
		ip *bool, ipv4 *bool, ipv6 *bool, mac *bool, cidr *bool,
		hostname *bool, fqdn *bool, base64 *bool, jsonValid *bool, jwt *bool,
		lowercase *bool, uppercase *bool, creditCard *bool,
		latitude *bool, longitude *bool, timezone *bool,
		eq *float64, ne *float64, gt *float64, lt *float64,
		len *int32,
		eqField *string, neField *string, gtField *string, ltField *string,
		requiredWith *string, requiredWithout *string, excludedWith *string,
		userExist *bool, userHasRole *string,
		message *string,
		patternMessage *string, minMessage *string, maxMessage *string,
		minLenMessage *string, maxLenMessage *string,
		minItemsMessage *string, maxItemsMessage *string,
		enumMessage *string, emailMessage *string, urlMessage *string, uuidMessage *string,
		notEmptyMessage *string, alphanumericMessage *string, numericMessage *string, alphaMessage *string,
		containsMessage *string, notContainsMessage *string,
		startsWithMessage *string, endsWithMessage *string,
		ipMessage *string, ipv4Message *string, ipv6Message *string, macMessage *string, cidrMessage *string,
		hostnameMessage *string, fqdnMessage *string, base64Message *string, jsonValidMessage *string, jwtMessage *string,
		lowercaseMessage *string, uppercaseMessage *string, creditCardMessage *string,
		latitudeMessage *string, longitudeMessage *string, timezoneMessage *string,
		eqMessage *string, neMessage *string, gtMessage *string, ltMessage *string,
		lenMessage *string,
		eqFieldMessage *string, neFieldMessage *string, gtFieldMessage *string, ltFieldMessage *string,
		requiredWithMessage *string, requiredWithoutMessage *string, excludedWithMessage *string,
		userExistMessage *string, userHasRoleMessage *string,
	) (interface{}, error) {

		value, err := next(ctx)
		if err != nil {
			return nil, err
		}

		// Unwrap pointer types for validation.
		// gqlgen uses pointers for nullable/optional fields (*string, *int32, etc.)
		// but all validation checks expect concrete types (string, int32, etc.).
		returnValue := value
		value = derefPointer(value)

		// --- Resolve field name ---
		fieldContext := graphql.GetFieldContext(ctx)
		fieldName := ""
		if fieldContext != nil {
			if pathContext := graphql.GetPathContext(ctx); pathContext != nil {
				segments := []string{}
				current := pathContext
				isArray := false

				for current != nil {
					if current.Field != nil {
						fieldStr := *current.Field
						if current.Index != nil {
							fieldStr = fmt.Sprintf("%s[%d]", fieldStr, *current.Index)
							isArray = true
						}
						segments = append([]string{fieldStr}, segments...)
					}
					current = current.Parent
				}

				if strSliceLen(segments) > 0 {
					if isArray && strSliceLen(segments) >= 2 {
						fieldName = strings.Join(segments[strSliceLen(segments)-2:], ".")
					} else if strSliceLen(segments) == 1 {
						fieldName = segments[0]
					} else if strSliceLen(segments) >= 2 {
						lastSegment := segments[strSliceLen(segments)-1]
						secondLastSegment := segments[strSliceLen(segments)-2]
						if strings.Contains(secondLastSegment, "[") {
							fieldName = secondLastSegment + "." + lastSegment
						} else {
							fieldName = lastSegment
						}
					}
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

		// ============================================================
		// 1. Cross-field required/excluded checks (work with nil values)
		// ============================================================

		if requiredWith != nil {
			if isFieldPresent(obj, *requiredWith) {
				if isEmptyValue(value) {
					validationError.Rules = append(validationError.Rules, "requiredWith")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(requiredWithMessage, message,
							fmt.Sprintf("%s is required when %s is present", fieldName, *requiredWith)))
				}
			}
		}

		if requiredWithout != nil {
			if !isFieldPresent(obj, *requiredWithout) {
				if isEmptyValue(value) {
					validationError.Rules = append(validationError.Rules, "requiredWithout")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(requiredWithoutMessage, message,
							fmt.Sprintf("%s is required when %s is not present", fieldName, *requiredWithout)))
				}
			}
		}

		if excludedWith != nil {
			if isFieldPresent(obj, *excludedWith) {
				if !isEmptyValue(value) {
					validationError.Rules = append(validationError.Rules, "excludedWith")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(excludedWithMessage, message,
							fmt.Sprintf("%s must be empty when %s is present", fieldName, *excludedWith)))
				}
			}
		}

		// ============================================================
		// 2. Nil check — save any cross-field errors and return
		// ============================================================

		if value == nil {
			if strSliceLen(validationError.Rules) > 0 {
				ctx = WithValidationError(ctx, validationError)
			}
			return returnValue, nil
		}

		// ============================================================
		// 3. Cross-field comparison validators (need non-nil value)
		// ============================================================

		if eqField != nil {
			if otherVal, ok := getFieldValue(obj, *eqField); ok {
				if compareValues(value, otherVal) != 0 {
					validationError.Rules = append(validationError.Rules, "eqField")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(eqFieldMessage, message,
							fmt.Sprintf("%s must be equal to %s", fieldName, *eqField)))
				}
			}
		}

		if neField != nil {
			if otherVal, ok := getFieldValue(obj, *neField); ok {
				if compareValues(value, otherVal) == 0 {
					validationError.Rules = append(validationError.Rules, "neField")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(neFieldMessage, message,
							fmt.Sprintf("%s must not be equal to %s", fieldName, *neField)))
				}
			}
		}

		if gtField != nil {
			if otherVal, ok := getFieldValue(obj, *gtField); ok {
				if compareValues(value, otherVal) <= 0 {
					validationError.Rules = append(validationError.Rules, "gtField")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(gtFieldMessage, message,
							fmt.Sprintf("%s must be greater than %s", fieldName, *gtField)))
				}
			}
		}

		if ltField != nil {
			if otherVal, ok := getFieldValue(obj, *ltField); ok {
				if compareValues(value, otherVal) >= 0 {
					validationError.Rules = append(validationError.Rules, "ltField")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(ltFieldMessage, message,
							fmt.Sprintf("%s must be less than %s", fieldName, *ltField)))
				}
			}
		}

		// ============================================================
		// 4. Bool format validators
		// ============================================================

		if email != nil && *email {
			if strVal, ok := value.(string); ok {
				emailRegex := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
				re, _ := compileRegex(emailRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "email")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(emailMessage, message,
							fmt.Sprintf("%s must be a valid email address", fieldName)))
				}
			}
		}

		if url != nil && *url {
			if strVal, ok := value.(string); ok {
				urlRegex := `^(https?|ftp)://[^\s/$.?#].[^\s]*$`
				re, _ := compileRegex(urlRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "url")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(urlMessage, message,
							fmt.Sprintf("%s must be a valid URL", fieldName)))
				}
			}
		}

		if uuid != nil && *uuid {
			if strVal, ok := value.(string); ok {
				uuidRegex := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
				re, _ := compileRegex(uuidRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "uuid")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(uuidMessage, message,
							fmt.Sprintf("%s must be a valid UUID", fieldName)))
				}
			}
		}

		if ip != nil && *ip {
			if strVal, ok := value.(string); ok {
				if net.ParseIP(strVal) == nil {
					validationError.Rules = append(validationError.Rules, "ip")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(ipMessage, message,
							fmt.Sprintf("%s must be a valid IP address", fieldName)))
				}
			}
		}

		if ipv4 != nil && *ipv4 {
			if strVal, ok := value.(string); ok {
				parsed := net.ParseIP(strVal)
				if parsed == nil || parsed.To4() == nil {
					validationError.Rules = append(validationError.Rules, "ipv4")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(ipv4Message, message,
							fmt.Sprintf("%s must be a valid IPv4 address", fieldName)))
				}
			}
		}

		if ipv6 != nil && *ipv6 {
			if strVal, ok := value.(string); ok {
				parsed := net.ParseIP(strVal)
				if parsed == nil || parsed.To4() != nil {
					validationError.Rules = append(validationError.Rules, "ipv6")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(ipv6Message, message,
							fmt.Sprintf("%s must be a valid IPv6 address", fieldName)))
				}
			}
		}

		if mac != nil && *mac {
			if strVal, ok := value.(string); ok {
				if _, err := net.ParseMAC(strVal); err != nil {
					validationError.Rules = append(validationError.Rules, "mac")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(macMessage, message,
							fmt.Sprintf("%s must be a valid MAC address", fieldName)))
				}
			}
		}

		if cidr != nil && *cidr {
			if strVal, ok := value.(string); ok {
				if _, _, err := net.ParseCIDR(strVal); err != nil {
					validationError.Rules = append(validationError.Rules, "cidr")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(cidrMessage, message,
							fmt.Sprintf("%s must be a valid CIDR notation", fieldName)))
				}
			}
		}

		if hostname != nil && *hostname {
			if strVal, ok := value.(string); ok {
				hostnameRegex := `^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`
				re, _ := compileRegex(hostnameRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "hostname")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(hostnameMessage, message,
							fmt.Sprintf("%s must be a valid hostname", fieldName)))
				}
			}
		}

		if fqdn != nil && *fqdn {
			if strVal, ok := value.(string); ok {
				fqdnRegex := `^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
				re, _ := compileRegex(fqdnRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "fqdn")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(fqdnMessage, message,
							fmt.Sprintf("%s must be a valid FQDN", fieldName)))
				}
			}
		}

		if base64 != nil && *base64 {
			if strVal, ok := value.(string); ok {
				if _, err := b64.StdEncoding.DecodeString(strVal); err != nil {
					validationError.Rules = append(validationError.Rules, "base64")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(base64Message, message,
							fmt.Sprintf("%s must be a valid base64 string", fieldName)))
				}
			}
		}

		if jsonValid != nil && *jsonValid {
			if strVal, ok := value.(string); ok {
				if !json.Valid([]byte(strVal)) {
					validationError.Rules = append(validationError.Rules, "jsonValid")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(jsonValidMessage, message,
							fmt.Sprintf("%s must be a valid JSON string", fieldName)))
				}
			}
		}

		if jwt != nil && *jwt {
			if strVal, ok := value.(string); ok {
				jwtRegex := `^[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]*$`
				re, _ := compileRegex(jwtRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "jwt")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(jwtMessage, message,
							fmt.Sprintf("%s must be a valid JWT token", fieldName)))
				}
			}
		}

		if lowercase != nil && *lowercase {
			if strVal, ok := value.(string); ok {
				if strVal != strings.ToLower(strVal) {
					validationError.Rules = append(validationError.Rules, "lowercase")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(lowercaseMessage, message,
							fmt.Sprintf("%s must be lowercase", fieldName)))
				}
			}
		}

		if uppercase != nil && *uppercase {
			if strVal, ok := value.(string); ok {
				if strVal != strings.ToUpper(strVal) {
					validationError.Rules = append(validationError.Rules, "uppercase")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(uppercaseMessage, message,
							fmt.Sprintf("%s must be uppercase", fieldName)))
				}
			}
		}

		if creditCard != nil && *creditCard {
			if strVal, ok := value.(string); ok {
				if !luhnCheck(strVal) {
					validationError.Rules = append(validationError.Rules, "creditCard")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(creditCardMessage, message,
							fmt.Sprintf("%s must be a valid credit card number", fieldName)))
				}
			}
		}

		if latitude != nil && *latitude {
			valid := false
			if numVal, ok := toFloat64(value); ok {
				if numVal >= -90 && numVal <= 90 {
					valid = true
				}
			}
			if !valid {
				validationError.Rules = append(validationError.Rules, "latitude")
				validationError.Messages = append(validationError.Messages,
					resolveMessage(latitudeMessage, message,
						fmt.Sprintf("%s must be a valid latitude", fieldName)))
			}
		}

		if longitude != nil && *longitude {
			valid := false
			if numVal, ok := toFloat64(value); ok {
				if numVal >= -180 && numVal <= 180 {
					valid = true
				}
			}
			if !valid {
				validationError.Rules = append(validationError.Rules, "longitude")
				validationError.Messages = append(validationError.Messages,
					resolveMessage(longitudeMessage, message,
						fmt.Sprintf("%s must be a valid longitude", fieldName)))
			}
		}

		if timezone != nil && *timezone {
			if strVal, ok := value.(string); ok {
				if _, err := time.LoadLocation(strVal); err != nil {
					validationError.Rules = append(validationError.Rules, "timezone")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(timezoneMessage, message,
							fmt.Sprintf("%s must be a valid timezone", fieldName)))
				}
			}
		}

		// ============================================================
		// 5. String validators
		// ============================================================

		if notEmpty != nil && *notEmpty {
			isEmpty := false
			switch v := value.(type) {
			case string:
				isEmpty = strings.TrimSpace(v) == ""
			case []interface{}:
				isEmpty = sliceLen(v) == 0
			}
			if isEmpty {
				validationError.Rules = append(validationError.Rules, "notEmpty")
				validationError.Messages = append(validationError.Messages,
					resolveMessage(notEmptyMessage, message,
						fmt.Sprintf("%s cannot be empty", fieldName)))
			}
		}

		if alphanumeric != nil && *alphanumeric {
			if strVal, ok := value.(string); ok {
				alphaNumRegex := `^[a-zA-Z0-9]+$`
				re, _ := compileRegex(alphaNumRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "alphanumeric")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(alphanumericMessage, message,
							fmt.Sprintf("%s must contain only letters and numbers", fieldName)))
				}
			}
		}

		if numeric != nil && *numeric {
			if strVal, ok := value.(string); ok {
				numericRegex := `^[0-9]+$`
				re, _ := compileRegex(numericRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "numeric")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(numericMessage, message,
							fmt.Sprintf("%s must contain only numbers", fieldName)))
				}
			}
		}

		if alpha != nil && *alpha {
			if strVal, ok := value.(string); ok {
				alphaRegex := `^[a-zA-Z]+$`
				re, _ := compileRegex(alphaRegex)
				if !re.MatchString(strVal) {
					validationError.Rules = append(validationError.Rules, "alpha")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(alphaMessage, message,
							fmt.Sprintf("%s must contain only letters", fieldName)))
				}
			}
		}

		if contains != nil {
			if strVal, ok := value.(string); ok {
				if !strings.Contains(strVal, *contains) {
					validationError.Rules = append(validationError.Rules, "contains")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(containsMessage, message,
							fmt.Sprintf("%s must contain '%s'", fieldName, *contains)))
				}
			}
		}

		if notContains != nil {
			if strVal, ok := value.(string); ok {
				if strings.Contains(strVal, *notContains) {
					validationError.Rules = append(validationError.Rules, "notContains")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(notContainsMessage, message,
							fmt.Sprintf("%s must not contain '%s'", fieldName, *notContains)))
				}
			}
		}

		if startsWith != nil {
			if strVal, ok := value.(string); ok {
				if !strings.HasPrefix(strVal, *startsWith) {
					validationError.Rules = append(validationError.Rules, "startsWith")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(startsWithMessage, message,
							fmt.Sprintf("%s must start with '%s'", fieldName, *startsWith)))
				}
			}
		}

		if endsWith != nil {
			if strVal, ok := value.(string); ok {
				if !strings.HasSuffix(strVal, *endsWith) {
					validationError.Rules = append(validationError.Rules, "endsWith")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(endsWithMessage, message,
							fmt.Sprintf("%s must end with '%s'", fieldName, *endsWith)))
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
					validationError.Messages = append(validationError.Messages,
						resolveMessage(patternMessage, message,
							fmt.Sprintf("%s does not match the required pattern", fieldName)))
				}
			}
		}

		// ============================================================
		// 6. Numeric validators
		// ============================================================

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
				validationError.Messages = append(validationError.Messages,
					resolveMessage(minMessage, message, errorMsg))
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
				validationError.Messages = append(validationError.Messages,
					resolveMessage(maxMessage, message, errorMsg))
			}
		}

		if eq != nil {
			if numVal, ok := toFloat64(value); ok {
				if numVal != *eq {
					validationError.Rules = append(validationError.Rules, "eq")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(eqMessage, message,
							fmt.Sprintf("%s must be equal to %g", fieldName, *eq)))
				}
			}
		}

		if ne != nil {
			if numVal, ok := toFloat64(value); ok {
				if numVal == *ne {
					validationError.Rules = append(validationError.Rules, "ne")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(neMessage, message,
							fmt.Sprintf("%s must not be equal to %g", fieldName, *ne)))
				}
			}
		}

		if gt != nil {
			if numVal, ok := toFloat64(value); ok {
				if numVal <= *gt {
					validationError.Rules = append(validationError.Rules, "gt")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(gtMessage, message,
							fmt.Sprintf("%s must be greater than %g", fieldName, *gt)))
				}
			}
		}

		if lt != nil {
			if numVal, ok := toFloat64(value); ok {
				if numVal >= *lt {
					validationError.Rules = append(validationError.Rules, "lt")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(ltMessage, message,
							fmt.Sprintf("%s must be less than %g", fieldName, *lt)))
				}
			}
		}

		// ============================================================
		// 7. Length validators
		// ============================================================

		if minLen != nil {
			valid := false
			errorMsg := ""

			switch v := value.(type) {
			case string:
				if stringLen(v) < int(*minLen) {
					errorMsg = fmt.Sprintf("%s must be at least %d characters", fieldName, *minLen)
				} else {
					valid = true
				}
			case []interface{}:
				if sliceLen(v) < int(*minLen) {
					errorMsg = fmt.Sprintf("%s must have at least %d items", fieldName, *minLen)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "minLen")
				validationError.Messages = append(validationError.Messages,
					resolveMessage(minLenMessage, message, errorMsg))
			}
		}

		if maxLen != nil {
			valid := false
			errorMsg := ""

			switch v := value.(type) {
			case string:
				if stringLen(v) > int(*maxLen) {
					errorMsg = fmt.Sprintf("%s must be at most %d characters", fieldName, *maxLen)
				} else {
					valid = true
				}
			case []interface{}:
				if sliceLen(v) > int(*maxLen) {
					errorMsg = fmt.Sprintf("%s must have at most %d items", fieldName, *maxLen)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "maxLen")
				validationError.Messages = append(validationError.Messages,
					resolveMessage(maxLenMessage, message, errorMsg))
			}
		}

		if len != nil {
			valid := false
			errorMsg := ""

			switch v := value.(type) {
			case string:
				if stringLen(v) != int(*len) {
					errorMsg = fmt.Sprintf("%s must be exactly %d characters", fieldName, *len)
				} else {
					valid = true
				}
			case []interface{}:
				if sliceLen(v) != int(*len) {
					errorMsg = fmt.Sprintf("%s must have exactly %d items", fieldName, *len)
				} else {
					valid = true
				}
			default:
				valid = true
			}

			if !valid && errorMsg != "" {
				validationError.Rules = append(validationError.Rules, "len")
				validationError.Messages = append(validationError.Messages,
					resolveMessage(lenMessage, message, errorMsg))
			}
		}

		// ============================================================
		// 8. Array validators
		// ============================================================

		if minItems != nil {
			if arrVal, ok := value.([]interface{}); ok {
				if sliceLen(arrVal) < int(*minItems) {
					validationError.Rules = append(validationError.Rules, "minItems")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(minItemsMessage, message,
							fmt.Sprintf("%s must have at least %d items", fieldName, *minItems)))
				}
			}
		}

		if maxItems != nil {
			if arrVal, ok := value.([]interface{}); ok {
				if sliceLen(arrVal) > int(*maxItems) {
					validationError.Rules = append(validationError.Rules, "maxItems")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(maxItemsMessage, message,
							fmt.Sprintf("%s must have at most %d items", fieldName, *maxItems)))
				}
			}
		}

		// ============================================================
		// 9. Enum validator
		// ============================================================

		if strSliceLen(enum) > 0 {
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
				validationError.Messages = append(validationError.Messages,
					resolveMessage(enumMessage, message,
						fmt.Sprintf("%s must be one of: %s", fieldName, strings.Join(enum, ", "))))
			}
		}

		// ============================================================
		// 10. User existence & role check (Redis)
		// ============================================================

		if userExist != nil && *userExist {
			if strVal, ok := value.(string); ok && strVal != "" {
				cacheKey := fmt.Sprintf("user:roles:%s", strVal)
				var cached cachedUserRoles
				if err := cache.Get(ctx, cacheKey, &cached); err != nil {
					validationError.Rules = append(validationError.Rules, "userExist")
					validationError.Messages = append(validationError.Messages,
						resolveMessage(userExistMessage, message,
							fmt.Sprintf("user with ID %s not found", strVal)))
				} else if userHasRole != nil && *userHasRole != "" {
					hasRole := false
					for _, r := range cached.Roles {
						if r == *userHasRole {
							hasRole = true
							break
						}
					}
					if !hasRole {
						validationError.Rules = append(validationError.Rules, "userHasRole")
						validationError.Messages = append(validationError.Messages,
							resolveMessage(userHasRoleMessage, message,
								fmt.Sprintf("user with ID %s does not have role '%s'", strVal, *userHasRole)))
					}
				}
			}
		}

		// ============================================================
		// Store errors
		// ============================================================

		if strSliceLen(validationError.Rules) > 0 {
			ctx = WithValidationError(ctx, validationError)
		}

		return returnValue, nil
	}
}
