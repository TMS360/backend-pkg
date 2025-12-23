package errors

import (
	"context"
	"errors"
	"log/slog"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/response"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// NewErrorPresenter creates a consistent error formatter for all services
func NewErrorPresenter(isDebug bool) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		// 1. Get standard gqlgen error
		gqlErr := graphql.DefaultErrorPresenter(ctx, err)

		// 2. Check for your custom "PublicError"
		var customErr response.PublicError
		if errors.As(err, &customErr) {
			gqlErr.Message = customErr.UserMessage()
			gqlErr.Extensions = map[string]any{
				"code":   customErr.ErrorCode(),
				"status": customErr.ErrorStatus(),
			}
		} else {
			// 3. Unexpected errors
			slog.Error("GraphQL Internal Error", "err", err, "path", gqlErr.Path)

			if !isDebug {
				gqlErr.Message = "Internal Server Error"
				gqlErr.Extensions = map[string]any{
					"code": "INTERNAL_SERVER_ERROR",
				}
			}
		}

		// 4. Debug info
		if isDebug {
			if gqlErr.Extensions == nil {
				gqlErr.Extensions = make(map[string]any)
			}
			gqlErr.Extensions["technical"] = err.Error()
		}

		return gqlErr
	}
}
