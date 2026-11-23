package consts

import "github.com/TMS360/backend-pkg/response"

var (
	ErrValidation         = response.NewBadRequest("validation_error", "Validation Error")
	ErrInvalidCredentials = response.NewBadRequest("invalid_credentials", "Invalid Credentials")
	ErrUnauthorized       = response.NewUnauthorized("unauthorized", "Unauthorized!")
	ErrForbidden          = response.NewForbidden("forbidden", "Forbidden")
	ErrInvalidRequestBody = response.NewBadRequest("invalid_request_body", "Invalid Request Body")
)
