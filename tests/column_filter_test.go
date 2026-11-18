package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/response"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DEV-2065. The shared column registry: a list declares its columns once and
// gets generic per-column filtering plus the sort whitelist out of that one
// table. SQL is rendered through a real gorm DryRun session, so no database is
// needed here.

// registry is the two-column list the acceptance criteria describe.
var registry = tmsdb.NewColumnRegistry(
	tmsdb.Column{Field: "number", SQL: "number", Type: tmsdb.ColumnText},
	tmsdb.Column{Field: "createdAt", SQL: "created_at", Type: tmsdb.ColumnDate},
	tmsdb.Column{Field: "year", SQL: "year", Type: tmsdb.ColumnNumber},
	tmsdb.Column{Field: "driverName", SQL: "d.name", Type: tmsdb.ColumnText},
)

func dryRunFilter(t *testing.T) *tmsdb.FilterBuilder {
	t.Helper()

	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  "host=127.0.0.1 port=1 user=dryrun dbname=dryrun sslmode=disable",
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Discard},
	)
	require.NoError(t, err)

	return tmsdb.NewGormTransactionManager(db, "pkg-test").Filter(context.Background(), &struct{}{})
}

// renderSQL renders what the builder built, plus the bound arguments.
func renderSQL(t *testing.T, fb *tmsdb.FilterBuilder) (string, []string) {
	t.Helper()

	stmt := fb.DB().Table("rows").Select("id").Find(&[]struct{}{}).Statement

	args := make([]string, 0, len(stmt.Vars))
	for _, v := range stmt.Vars {
		args = append(args, fmt.Sprintf("%v", v))
	}
	return stmt.SQL.String(), args
}

func strPtr(s string) *string { return &s }

// AC1: a list that registers "number" (text) and "createdAt" (date) can filter
// contains on number and sort by createdAt.
func TestColumnFilterContainsAndSortByRegisteredColumn(t *testing.T) {
	fb := dryRunFilter(t)

	err := fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{Field: "number", Text: &tmsdb.StringFilter{Contains: strPtr("104"), Mode: tmsdb.QueryModeInsensitive}},
	}, registry)
	require.NoError(t, err)

	fb.ApplySort([]*tmsdb.SortInput{{Field: "createdAt", Order: tmsdb.SortOrderDesc}}, registry.SortFields())

	sql, args := renderSQL(t, fb)
	require.Contains(t, sql, "number ILIKE")
	require.Equal(t, []string{"%104%"}, args)
	require.Contains(t, sql, "ORDER BY created_at DESC")
}

// AC1 (second half): the count the caller shows is taken off the same builder,
// so it counts the filtered set and not the whole table.
func TestColumnFilterIsCountedBeforePagination(t *testing.T) {
	fb := dryRunFilter(t)

	require.NoError(t, fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{Field: "number", Text: &tmsdb.StringFilter{Equals: strPtr("1042")}},
	}, registry))

	stmt := fb.DB().Table("rows").Limit(-1).Offset(-1).Count(new(int64)).Statement
	require.Contains(t, stmt.SQL.String(), "count(")
	require.Contains(t, stmt.SQL.String(), "number = ")
	require.Equal(t, []any{"1042"}, stmt.Vars)
}

// AC2: two column filters combine with AND.
func TestTwoColumnFiltersCombineWithAND(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fb := dryRunFilter(t)

	require.NoError(t, fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{Field: "number", Text: &tmsdb.StringFilter{Contains: strPtr("104")}},
		{Field: "createdAt", Date: &tmsdb.DateTimeFilter{Gte: &from}},
	}, registry))

	sql, _ := renderSQL(t, fb)
	require.Contains(t, sql, "number ILIKE")
	require.Contains(t, sql, "created_at >=")
	require.Contains(t, sql, "AND")
	require.NotContains(t, sql, " OR ")
}

// AC3: a field the list did not register is a validation error — not a 500 and
// not a silently unfiltered list.
func TestUnknownColumnIsRefusedAsBadRequest(t *testing.T) {
	fb := dryRunFilter(t)

	err := fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{Field: "salary", Text: &tmsdb.StringFilter{Contains: strPtr("x")}},
	}, registry)

	var pub response.PublicError
	require.ErrorAs(t, err, &pub)
	require.Equal(t, 400, pub.ErrorStatus())
	require.Contains(t, err.Error(), "salary")

	sql, args := renderSQL(t, fb)
	require.NotContains(t, sql, "salary")
	require.Empty(t, args)
}

// AC3 (same rule, wrong type): a date filter on a text column is refused rather
// than dropped, so a filter never silently does nothing.
func TestWrongFilterTypeForColumnIsRefused(t *testing.T) {
	at := time.Now()
	fb := dryRunFilter(t)

	err := fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{Field: "number", Date: &tmsdb.DateTimeFilter{Gte: &at}},
	}, registry)

	var pub response.PublicError
	require.ErrorAs(t, err, &pub)
	require.Equal(t, 400, pub.ErrorStatus())
}

// Two filters in one entry is ambiguous — refused for the same reason.
func TestTwoFilterTypesInOneEntryAreRefused(t *testing.T) {
	fb := dryRunFilter(t)

	err := fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{
			Field: "number",
			Text:  &tmsdb.StringFilter{Contains: strPtr("104")},
			ID:    &tmsdb.IDFilter{Equals: strPtr("1")},
		},
	}, registry)

	var pub response.PublicError
	require.ErrorAs(t, err, &pub)
	require.Equal(t, 400, pub.ErrorStatus())
}

// Edge case: empty filter list / empty entry and no sort → the list's default
// order, newest created first.
func TestEmptyColumnFiltersAndNoSortKeepDefaultOrder(t *testing.T) {
	fb := dryRunFilter(t)

	require.NoError(t, fb.ApplyColumnFilters(nil, registry))
	require.NoError(t, fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		nil,
		{Field: "number"},
	}, registry))

	fb.ApplySort(nil, registry.SortFields())

	sql, args := renderSQL(t, fb)
	require.NotContains(t, sql, "WHERE")
	require.Empty(t, args)
	require.Contains(t, sql, "ORDER BY created_at DESC")
}

// Edge case: a sort field the list does not publish is skipped, exactly as
// before, and the default order still applies. Old screens send extra keys.
func TestUnknownSortFieldIsSkippedNotRefused(t *testing.T) {
	fb := dryRunFilter(t)

	fb.ApplySort([]*tmsdb.SortInput{{Field: "salary", Order: tmsdb.SortOrderAsc}}, registry.SortFields())

	sql, _ := renderSQL(t, fb)
	require.Contains(t, sql, "ORDER BY created_at DESC")
	require.NotContains(t, sql, "salary")
}

// Edge case: a joined column that is null on some rows (no driver on the row)
// filters through the same path — isNull is just another operator, no crash.
func TestNullJoinedValueFiltersThroughIsNull(t *testing.T) {
	yes := true
	fb := dryRunFilter(t)

	require.NoError(t, fb.ApplyColumnFilters([]*tmsdb.ColumnFilterInput{
		{Field: "driverName", Text: &tmsdb.StringFilter{IsNull: &yes}},
	}, registry))

	sql, _ := renderSQL(t, fb)
	require.Contains(t, sql, "d.name IS NULL")
}

// The registry is the single source for sorting too: what can be filtered is
// what can be sorted, spelled either way the client sends it.
func TestRegistryFeedsSortWhitelistAndAcceptsSnakeCase(t *testing.T) {
	require.Equal(t, map[string]string{
		"number":     "number",
		"createdAt":  "created_at",
		"year":       "year",
		"driverName": "d.name",
	}, registry.SortFields())

	col, ok := registry.Lookup("created_at")
	require.True(t, ok)
	require.Equal(t, "created_at", col.SQL)

	_, ok = registry.Lookup("salary")
	require.False(t, ok)
}

// A half-declared column never becomes a filter nobody whitelisted.
func TestColumnWithoutSQLIsNotRegistered(t *testing.T) {
	reg := tmsdb.NewColumnRegistry(
		tmsdb.Column{Field: "broken", Type: tmsdb.ColumnText},
		tmsdb.Column{SQL: "nameless", Type: tmsdb.ColumnText},
	)
	_, ok := reg.Lookup("broken")
	require.False(t, ok)
	require.Empty(t, reg.SortFields())
}
