package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID reads X-Request-ID from the incoming header (or mints a new UUIDv4
// if absent), stores it in the request context, and echoes it back in the
// response header so clients can quote it when reporting issues. Combined with
// the GraphQL error presenter and Sentry tags, this lets a user-reported
// "5cf3-1a2b…" map to the exact server log line and Sentry event.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Request = c.Request.WithContext(WithRequestID(c.Request.Context(), id))
		c.Writer.Header().Set(RequestIDHeader, id)
		c.Next()
	}
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// GetRequestID returns the current request's ID, or "" if none is bound.
func GetRequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}
