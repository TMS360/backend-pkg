package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DEV-3618. Board footer totals read a whole board through the owning list
// query. A trusted internal call (X-Internal-Token + X-Bulk-Read-Limit) may now
// ask for one page of up to 5,000 rows instead of 18 pages of 100; any other
// caller keeps the list's own ceiling.

const bulkTestSecret = "bulk-test-secret"

func dryRunFilterCtx(t *testing.T, ctx context.Context) *tmsdb.FilterBuilder {
	t.Helper()
	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  "host=127.0.0.1 port=1 user=dryrun dbname=dryrun sslmode=disable",
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Discard},
	)
	require.NoError(t, err)
	return tmsdb.NewGormTransactionManager(db, "pkg-test").Filter(ctx, &struct{}{})
}

// bulkLimitSeen runs IdentifyUser over a request with the given headers and
// returns the bulk-read limit the handler's context carries.
func bulkLimitSeen(t *testing.T, headers map[string]string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	seen := -1
	router := gin.New()
	router.Use(middleware.IdentifyUser(nil))
	router.POST("/query", func(c *gin.Context) {
		seen = middleware.BulkReadLimit(c.Request.Context())
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	router.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

// AC1 (owner side): a bulk-read context serves the whole 1,797-row board in
// one page and reports that page size.
func TestBulkReadLiftsThePageCeiling(t *testing.T) {
	ctx := middleware.WithBulkReadLimit(context.Background(), 5000)
	p := &tmsdb.PaginationInput{Page: 1, Limit: 5000}

	_, args := renderPageAfterCount(t, dryRunFilterCtx(t, ctx), p)
	require.Equal(t, []string{"5000"}, args)
	require.Equal(t, int32(5000), tmsdb.NewPagination(p, 1797).Limit)
	require.Equal(t, int32(1), tmsdb.NewPagination(p, 1797).TotalPages)
}

// AC4: whatever is asked, a bulk read never goes past 5,000 rows a page.
func TestBulkReadIsCappedAtTheSummaryBudget(t *testing.T) {
	ctx := middleware.WithBulkReadLimit(context.Background(), 50000)
	assert.Equal(t, middleware.MaxBulkReadLimit, middleware.BulkReadLimit(ctx))

	p := &tmsdb.PaginationInput{Page: 1, Limit: 50000}
	_, args := renderPageAfterCount(t, dryRunFilterCtx(t, ctx), p)
	require.Equal(t, []string{"5000"}, args)
}

// Without a bulk-read context the list keeps its ceiling of 100.
func TestNoBulkReadKeepsTheCeiling(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 1, Limit: 5000}
	_, args := renderPageAfterCount(t, dryRunFilterCtx(t, context.Background()), p)
	require.Equal(t, []string{"100"}, args)
}

// A list that already allows more than the bulk limit keeps its own ceiling.
func TestBulkReadNeverLowersALargerCeiling(t *testing.T) {
	ctx := middleware.WithBulkReadLimit(context.Background(), 200)
	p := &tmsdb.PaginationInput{Page: 1, Limit: 100000}
	_, args := renderPageAfterCount(t, dryRunFilterCtx(t, ctx).SetMaxLimit(100000), p)
	require.Equal(t, []string{"100000"}, args)
}

// IdentifyUser turns the header into a bulk-read context only next to the
// shared internal secret.
func TestIdentifyUser_BulkReadNeedsTheInternalToken(t *testing.T) {
	t.Setenv(middleware.EnvInternalToken, bulkTestSecret)

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"internal call with limit", map[string]string{
			middleware.HeaderInternalToken: bulkTestSecret, middleware.HeaderBulkReadLimit: "5000"}, 5000},
		{"limit above the cap", map[string]string{
			middleware.HeaderInternalToken: bulkTestSecret, middleware.HeaderBulkReadLimit: "999999"}, 5000},
		{"no token", map[string]string{middleware.HeaderBulkReadLimit: "5000"}, 0},
		{"wrong token", map[string]string{
			middleware.HeaderInternalToken: "guess", middleware.HeaderBulkReadLimit: "5000"}, 0},
		{"token without limit", map[string]string{middleware.HeaderInternalToken: bulkTestSecret}, 0},
		{"garbage limit", map[string]string{
			middleware.HeaderInternalToken: bulkTestSecret, middleware.HeaderBulkReadLimit: "lots"}, 0},
		{"negative limit", map[string]string{
			middleware.HeaderInternalToken: bulkTestSecret, middleware.HeaderBulkReadLimit: "-5"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, bulkLimitSeen(t, tc.headers))
		})
	}
}

// A service with no internal secret configured never grants a bulk read.
func TestIdentifyUser_NoSecretNoBulkRead(t *testing.T) {
	t.Setenv(middleware.EnvInternalToken, "")
	assert.Equal(t, 0, bulkLimitSeen(t, map[string]string{
		middleware.HeaderInternalToken: "", middleware.HeaderBulkReadLimit: "5000"}))
}
