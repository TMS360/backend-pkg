package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/stretchr/testify/require"
)

// DEV-2596. A list that caps the page size used to report the size that was
// asked for: `limit: 1000` served 100 rows but answered `limit: 1000,
// totalPages: 4`. Paginate now writes the served size back into the input, so
// the NewPagination(p, total) every repository calls afterwards tells the truth.

// AC4: `limit: 1000` on a list capped at 100 reports `limit: 100` and
// totalPages = total / 100 rounded up.
func TestCappedLimitIsReportedAsServed(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 1, Limit: 1000}

	fb := dryRunFilter(t) // default ceiling is 100
	_, args := renderPageAfterCount(t, fb, p)
	require.Equal(t, []string{"100"}, args)

	page := tmsdb.NewPagination(p, 350)
	require.Equal(t, int32(100), page.Limit)
	require.Equal(t, int32(350), page.Total)
	require.Equal(t, int32(4), page.TotalPages)
	require.Equal(t, int32(1), page.Page)
}

// AC4: the second capped page starts right after the first one's 100 rows,
// not at row 1001.
func TestCappedLimitStepsTheOffsetByTheServedSize(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 2, Limit: 1000}

	fb := dryRunFilter(t)
	_, args := renderPageAfterCount(t, fb, p)
	require.Equal(t, []string{"100", "100"}, args)
	require.Equal(t, 100, p.GetOffset())
}

// A list with its own ceiling reports that ceiling.
func TestCustomCeilingIsReportedAsServed(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 1, Limit: 500}

	fb := dryRunFilter(t).SetMaxLimit(200)
	fb.Paginate(p)

	page := tmsdb.NewPagination(p, 401)
	require.Equal(t, int32(200), page.Limit)
	require.Equal(t, int32(3), page.TotalPages)
}

// A request under the ceiling is left exactly as asked.
func TestUncappedLimitIsReportedAsAsked(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 3, Limit: 25}

	fb := dryRunFilter(t)
	_, args := renderPageAfterCount(t, fb, p)
	require.Equal(t, []string{"25", "50"}, args)

	require.Equal(t, int32(25), p.Limit)
	require.Equal(t, int32(25), tmsdb.NewPagination(p, 60).Limit)
}

// AC1/AC3 stay intact: `limit: 0` is still an empty page, and no pagination
// still serves the default twenty — the ceiling never touches either.
func TestCeilingLeavesZeroAndDefaultAlone(t *testing.T) {
	zero := &tmsdb.PaginationInput{Page: 1, Limit: 0}
	_, args := renderPageAfterCount(t, dryRunFilter(t), zero)
	require.Equal(t, []string{"0"}, args)
	page := tmsdb.NewPagination(zero, 57)
	require.Equal(t, int32(0), page.Limit)
	require.Equal(t, int32(0), page.TotalPages)
	require.Equal(t, int32(57), page.Total)

	_, args = renderPageAfterCount(t, dryRunFilter(t), nil)
	require.Equal(t, []string{"20"}, args)
	require.Equal(t, int32(20), tmsdb.NewPagination(nil, 57).Limit)
}
