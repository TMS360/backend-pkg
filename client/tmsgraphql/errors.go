package tmsgraphql

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/observability"
	"github.com/TMS360/backend-pkg/response"
	"github.com/TMS360/backend-pkg/validate"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// NewErrorPresenter creates a consistent error formatter for all services.
// Every emitted error carries `code` and `requestId` in extensions so clients
// can branch on the code (never the message) and quote the request ID for
// support. 5xx errors are also captured to Sentry via observability.
func NewErrorPresenter(isDebug bool) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		requestID := middleware.GetRequestID(ctx)

		if validationErrors := validate.GetValidationErrors(ctx); validationErrors != nil && validationErrors.HasErrors() {
			return &gqlerror.Error{
				Message: "Validation failed",
				Extensions: map[string]interface{}{
					"code":             "VALIDATION_ERROR",
					"requestId":        requestID,
					"validationErrors": validationErrors.ToArray(),
				},
			}
		}

		gqlErr := graphql.DefaultErrorPresenter(ctx, err)

		// 2. Check for your custom "PublicError"
		var customErr response.PublicError
		if errors.As(err, &customErr) {
			gqlErr.Message = customErr.UserMessage()
			gqlErr.Extensions = map[string]any{
				"code":   customErr.ErrorCode(),
				"status": customErr.ErrorStatus(),
			}
			// Capture server-side errors (5xx) to Sentry. 4xx are user errors —
			// noisy and rarely actionable for ops.
			if customErr.ErrorStatus() >= http.StatusInternalServerError {
				observability.CaptureWithCtx(ctx, err)
			}
		} else {
			// 3. Unexpected errors — always treat as 500-class.
			slog.Error("GraphQL Internal Error", "err", err, "path", gqlErr.Path, "request_id", requestID)
			observability.CaptureWithCtx(ctx, err)

			if !isDebug {
				gqlErr.Message = "Internal Server Error"
				gqlErr.Extensions = map[string]any{
					"code": "INTERNAL_SERVER_ERROR",
				}
			}
		}

		// 4. Always include requestId so the user/client can quote it.
		if gqlErr.Extensions == nil {
			gqlErr.Extensions = make(map[string]any)
		}
		gqlErr.Extensions["requestId"] = requestID

		// 5. Debug info
		if isDebug {
			gqlErr.Extensions["technical"] = err.Error()
		}

		return gqlErr
	}
}

// FillErrors is a helper function to create a slice of errors with the same error repeated n times. This can be useful for batch operations where you want to return the same error for multiple items.
func FillErrors(n int, err error) []error {
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		errs[i] = err
	}
	return errs
}
