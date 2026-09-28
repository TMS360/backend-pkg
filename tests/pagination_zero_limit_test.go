package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/stretchr/testify/require"
)

// DEV-903. `limit: 0` used to be mixed in with "the caller said nothing" and
// came back as the default page of twenty rows, on every paginated list at
// once, because they all share this helper. Zero is now an answer: an empty
// page whose totals are still counted. "Say nothing" is a missing pagination
// object or a negative limit.

// renderPageAfterCount renders the SELECT a list repository actually runs: the
// count first, then the page. The order matters — Count() leaves a LIMIT clause
// of its own on the statement, and that leftover is what a zero page has to
// survive. DryRun keeps the rendered SQL on the statement, so it is cleared
// between the two queries the way a live session would start the second one
// from scratch.
func renderPageAfterCount(t *testing.T, fb *tmsdb.FilterBuilder, p *tmsdb.PaginationInput) (string, []string) {
	t.Helper()

	_, err := fb.Count()
	require.NoError(t, err)

	fb.DB().Statement.SQL.Reset()
	fb.DB().Statement.Vars = nil

	fb.Paginate(p)
	return renderSQL(t, fb)
}

// AC1: `limit: 0` asks for zero rows, not for the default twenty.
func TestZeroLimitAsksForAnEmptyPage(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 1, Limit: 0}

	require.Equal(t, 0, p.GetLimit())
	require.Equal(t, 0, p.GetOffset())

	fb := dryRunFilter(t)
	fb.Paginate(p)

	sql, args := renderSQL(t, fb)
	require.Contains(t, sql, "LIMIT")
	require.Equal(t, []string{"0"}, args)
}

// AC1: the empty page still reports the totals — that is the whole point of
// asking for zero rows.
func TestZeroLimitKeepsTheTotals(t *testing.T) {
	page := tmsdb.NewPagination(&tmsdb.PaginationInput{Page: 1, Limit: 0}, 57)

	require.Equal(t, int32(0), page.Limit)
	require.Equal(t, int32(57), page.Total)
	require.Equal(t, int32(1), page.Page)
	// No page size means no pages to walk; claiming one would invite a request
	// for a page that can never hold a row.
	require.Equal(t, int32(0), page.TotalPages)
}

// AC1: the empty page survives the Count() that every list runs before it.
// gorm refuses to let a zero LIMIT overwrite the `LIMIT -1` Count() leaves
// behind, and `LIMIT -1` renders as no LIMIT at all — so without the fix this
// query came back with the entire table instead of nothing.
func TestZeroLimitSurvivesThePrecedingCount(t *testing.T) {
	fb := dryRunFilter(t)

	sql, args := renderPageAfterCount(t, fb, &tmsdb.PaginationInput{Page: 1, Limit: 0})

	require.Contains(t, sql, "LIMIT")
	require.Equal(t, []string{"0"}, args)
}

// AC2: a caller that states no page size still gets the default page. Neither
// shape can come from GraphQL (`page` and `limit` are both non-null), so this
// is the Go call sites that pass nothing.
func TestUnstatedPaginationStillServesTheDefaultPage(t *testing.T) {
	var missing *tmsdb.PaginationInput
	require.Equal(t, tmsdb.DefaultLimit, missing.GetLimit())
	require.Equal(t, 0, missing.GetOffset())

	empty := &tmsdb.PaginationInput{}
	require.Equal(t, tmsdb.DefaultLimit, empty.GetLimit())
	require.Equal(t, 1, empty.GetPage())

	page := tmsdb.NewPagination(nil, 0)
	require.Equal(t, int32(1), page.Page)
	require.Equal(t, int32(tmsdb.DefaultLimit), page.Limit)
	require.Equal(t, int32(1), page.TotalPages)

	fb := dryRunFilter(t)
	fb.Paginate(nil)
	_, args := renderSQL(t, fb)
	require.Equal(t, []string{"20"}, args)
}

// AC2: a negative limit is the explicit "use the default" sentinel, so a caller
// that wants the default without hard-coding twenty still gets it — and the
// offset is computed from the default, not from the negative number.
func TestNegativeLimitMeansUseTheDefault(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 2, Limit: -1}

	require.Equal(t, tmsdb.DefaultLimit, p.GetLimit())
	require.Equal(t, tmsdb.DefaultLimit, p.GetOffset())
}

// AC2: an ordinary page is untouched — same limit, same offset, same totals.
func TestOrdinaryPageIsUnchanged(t *testing.T) {
	p := &tmsdb.PaginationInput{Page: 3, Limit: 25}

	require.Equal(t, 25, p.GetLimit())
	require.Equal(t, 50, p.GetOffset())

	page := tmsdb.NewPagination(p, 57)
	require.Equal(t, int32(25), page.Limit)
	require.Equal(t, int32(3), page.Page)
	require.Equal(t, int32(3), page.TotalPages)

	fb := dryRunFilter(t)
	sql, args := renderPageAfterCount(t, fb, p)
	require.Contains(t, sql, "LIMIT")
	require.Contains(t, sql, "OFFSET")
	require.Equal(t, []string{"25", "50"}, args)
}

// AC2: the per-list ceiling still wins over what the client asked for.
func TestLimitIsStillClampedToMaxLimit(t *testing.T) {
	fb := dryRunFilter(t).SetMaxLimit(100)
	fb.Paginate(&tmsdb.PaginationInput{Page: 1, Limit: 1000})

	_, args := renderSQL(t, fb)
	require.Equal(t, []string{"100"}, args)
}
