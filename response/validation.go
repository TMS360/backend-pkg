package response

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-playground/validator/v10"
)

// ValidatorMessages interface should be implemented by your DTOs
type ValidatorMessages interface {
	GetMessages() map[string]string
}

// ParseValidationErrors tries to parse the error as validation errors.
func ParseValidationErrors(err error, req any) (map[string]string, bool) {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return nil, false
	}

	errorMap := make(map[string]string)

	// Check for custom messages
	var customMessages map[string]string
	if v, ok := req.(ValidatorMessages); ok {
		customMessages = v.GetMessages()
	}

	for _, fe := range ve {
		fieldName := fe.Field()
		tagName := fe.Tag()

		// Key: "Field.Tag" (e.g., "Password.min")
		if msg, exists := customMessages[fieldName+"."+tagName]; exists {
			errorMap[fieldName] = fmt.Sprintf(msg, fe.Param())
		} else {
			errorMap[fieldName] = getDefaultMessage(tagName, fe.Param())
		}
	}

	return errorMap, true
}

// ValidationError writes the specific 400 response
func ValidationError(w http.ResponseWriter, details map[string]string) {
	JSON(w, http.StatusBadRequest, map[string]any{
		"status":  http.StatusBadRequest,
		"message": "Validation failed for one or more fields.",
		"errors":  details,
	})
}

func getDefaultMessage(tag string, param string) string {
	switch tag {
	case "required":
		return "This field is required"
	case "email":
		return "Invalid email format"
	case "min":
		return fmt.Sprintf("Must be at least %s characters", param)
	default:
		return fmt.Sprintf("Failed validation on tag: %s", tag)
	}
}
