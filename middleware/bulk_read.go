package middleware

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	// HeaderBulkReadLimit asks a list query for a page bigger than its usual
	// ceiling (DEV-3618). It is honoured ONLY next to a valid X-Internal-Token:
	// one of our own services reading a whole board for one user click (footer
	// totals) — a browser or a script with only a user token keeps the normal
	// ceiling, so the header cannot be used to pull huge pages from outside.
	HeaderBulkReadLimit = "X-Bulk-Read-Limit"

	// MaxBulkReadLimit bounds what a bulk read may ask for, whatever the header
	// says. It matches the board summary row budget.
	MaxBulkReadLimit = 5000
)

type bulkReadLimitKey struct{}

// WithBulkReadLimit marks ctx as a trusted bulk read allowed pages up to n rows
// (clamped to MaxBulkReadLimit). n <= 0 leaves ctx unchanged.
func WithBulkReadLimit(ctx context.Context, n int) context.Context {
	if n <= 0 {
		return ctx
	}
	if n > MaxBulkReadLimit {
		n = MaxBulkReadLimit
	}
	return context.WithValue(ctx, bulkReadLimitKey{}, n)
}

// BulkReadLimit is the page size a trusted bulk read may use, or 0 when ctx is
// not one. tmsdb.FilterBuilder.Paginate lifts its ceiling to it.
func BulkReadLimit(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	n, _ := ctx.Value(bulkReadLimitKey{}).(int)
	return n
}

// bulkReadLimitFromRequest returns the bulk page size r may use: the
// X-Bulk-Read-Limit value when r also carries the shared internal secret,
// otherwise 0.
func bulkReadLimitFromRequest(r *http.Request, internalToken string) int {
	if r == nil {
		return 0
	}
	raw := strings.TrimSpace(r.Header.Get(HeaderBulkReadLimit))
	if raw == "" || !isInternalCall(r, internalToken) {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// withRequestBulkReadLimit puts the request's trusted bulk-read limit, if any,
// into its context. Called by IdentifyUser, the one middleware every service
// mounts, so a tms-pkg bump alone turns it on.
func withRequestBulkReadLimit(r *http.Request) *http.Request {
	n := bulkReadLimitFromRequest(r, strings.TrimSpace(os.Getenv(EnvInternalToken)))
	if n == 0 {
		return r
	}
	return r.WithContext(WithBulkReadLimit(r.Context(), n))
}
