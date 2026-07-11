package tmsgraphql

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/client/postgresql"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/observability"
	"github.com/TMS360/backend-pkg/response"
	"github.com/TMS360/backend-pkg/validate"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// captureFunc / captureWarningFunc report to Sentry at Error / Warning level.
// Indirected through package vars so tests can assert which severity each path
// uses: server faults (5xx) capture as errors, user rejections (4xx) as
// warnings.
var (
	captureFunc        = observability.CaptureWithCtx
	captureWarningFunc = observability.CaptureWarningWithCtx
)

// NewErrorPresenter creates a consistent error formatter for all services.
// Every emitted error carries `code` and `requestId` in extensions so clients
// can branch on the code (never the message) and quote the request ID for
// support. 5xx errors are captured to Sentry as errors (they alert); 4xx
// PublicErrors are captured as warnings so user friction is queryable without
// paging anyone.
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
		if !errors.As(err, &customErr) {
			// Backstop: a raw Postgres constraint violation (FK/unique/check/
			// not-null/exclusion) is a routine user-input error, not a server
			// fault. Map it to a public 4xx so a service that forgot to
			// translate still returns a clean message — and route it through the
			// same handling below (surfaced as a Sentry warning, not an error).
			if pub, ok := postgresql.AsPublicError(err); ok {
				customErr = pub
			}
		}

		if customErr != nil {
			gqlErr.Message = customErr.UserMessage()
			gqlErr.Extensions = map[string]any{
				"code":   customErr.ErrorCode(),
				"status": customErr.ErrorStatus(),
			}
			// Merge the error's structured payload (if any) so callers can attach
			// details like a blocking resource's id/number that the FE reads
			// directly from extensions instead of parsing the human message.
			// code/status are reserved keys — the payload cannot overwrite them.
			for k, v := range customErr.Extensions() {
				if k == "code" || k == "status" {
					continue
				}
				gqlErr.Extensions[k] = v
			}
			// 5xx are server faults — capture as errors (they alert). 4xx are
			// user-facing rejections — capture as warnings so friction is
			// queryable in Sentry without paging anyone.
			if customErr.ErrorStatus() >= http.StatusInternalServerError {
				captureFunc(ctx, err)
			} else {
				captureWarningFunc(ctx, err)
			}
		} else {
			// 3. Unexpected errors — always treat as 500-class.
			slog.Error("GraphQL Internal Error", "err", err, "path", gqlErr.Path, "request_id", requestID)
			captureFunc(ctx, err)

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

// BatchError wraps a batch-wide failure so dataloadgen propagates it to every
// caller. The library only treats a single-element errors slice as a global
// error; returning N errors alongside nil results trips its length check and
// masks the real cause with "bug in fetch function: 0 values returned for N keys".
func BatchError(err error) []error {
	return []error{err}
}

// FillErrors is a helper function to create a slice of errors with the same error repeated n times. This can be useful for batch operations where you want to return the same error for multiple items.
func FillErrors(n int, err error) []error {
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		errs[i] = err
	}
	return errs
}
