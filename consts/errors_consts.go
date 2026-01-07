package consts

import "github.com/TMS360/backend-pkg/response"

var (
	ErrValidation         = response.NewError("validation_error", "Validation Error", 400)
	ErrInvalidCredentials = response.NewError("invalid_credentials", "Invalid Credentials", 400)
	ErrUnauthorized       = response.NewError("unauthorized", "Unauthorized", 401)
	ErrInvalidRequestBody = response.NewError("invalid_request_body", "Invalid Request Body", 400)
)
