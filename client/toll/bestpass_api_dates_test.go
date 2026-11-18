package toll

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bestPassMaxPageSize is the pageSize.maximum of each BestPass list, as the
// live API enforces it: /equipment answers 400 above 5000 (DEV-4019).
var bestPassMaxPageSize = map[string]int{
	"/equipment":    5000,
	"/vehicles":     10000,
	"/transactions": 10000,
	"/statements":   10000,
}

// pageLimitHook answers every list like BestPass: 400 when pageSize is over
// that list's maximum, else one empty page.
func pageLimitHook(w http.ResponseWriter, r *http.Request) bool {
	size, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if limit := bestPassMaxPageSize[r.URL.Path]; err != nil || size < 1 || size > limit {
		http.Error(w, fmt.Sprintf(`{"message":"pageSize must be between 1 and %d"}`, limit), http.StatusBadRequest)
		return true
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"pagingInfo": map[string]any{"nextPage": nil}, "results": []any{}})
	return true
}

func TestBestPassAPI_EveryListAsksForAPageSizeItAllows(t *testing.T) {
	c, _ := newFakeClient(t, &fakeBestPass{hook: pageLimitHook})
	ctx := t.Context()
	calls := map[string]func() error{
		"Equipment":             func() error { _, err := c.Equipment(ctx, "90001"); return err },
		"Vehicles":              func() error { _, err := c.Vehicles(ctx, "90001"); return err },
		"Statements":            func() error { _, err := c.Statements(ctx, "90001"); return err },
		"Transactions":          func() error { _, err := c.Transactions(ctx, "90001", window[0], window[1]); return err },
		"StatementTransactions": func() error { _, err := c.StatementTransactions(ctx, "90001", "S-1"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) { require.NoError(t, call()) })
	}
}

// bestPassDateCases are the date shapes BestPass sends (no zone, as on
// production 2026-11-10) or may send, and what they must read as.
var bestPassDateCases = []struct {
	name string
	json string // the JSON value of the date field, "" = field missing
	want *time.Time
}{
	{"no zone with millis (live statement)", `"2026-10-01T01:00:00.817"`, ptrTime(time.Date(2026, 10, 1, 1, 0, 0, 817_000_000, time.UTC))},
	{"no zone (live inactive tag)", `"2024-11-01T00:00:00"`, ptrTime(time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC))},
	{"Z", `"2026-10-01T01:00:00Z"`, ptrTime(time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC))},
	{"offset", `"2026-10-01T06:00:00+05:00"`, ptrTime(time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC))},
	{"null", `null`, nil},
	{"empty", `""`, nil},
	{"missing", ``, nil},
	{"unreadable", `"01/10/2026"`, nil},
}

func ptrTime(t time.Time) *time.Time { return &t }

func withDate(fields, key, value string) string {
	if value == "" {
		return "{" + fields + "}"
	}
	return "{" + fields + `,"` + key + `":` + value + "}"
}

func TestBestPassStatement_ReadsEveryDateShape(t *testing.T) {
	for _, tc := range bestPassDateCases {
		t.Run(tc.name, func(t *testing.T) {
			var st BestPassStatement
			require.NoError(t, json.Unmarshal([]byte(withDate(`"statementNumber":"S-1"`, "dateCreated", tc.json)), &st))
			assert.Equal(t, "S-1", st.Number)
			if tc.want == nil {
				assert.True(t, st.DateCreated.IsZero(), st.DateCreated)
				return
			}
			assert.True(t, tc.want.Equal(st.DateCreated), "got %s", st.DateCreated)
		})
	}
}

func TestBestPassAssignment_ReadsEveryDateShape(t *testing.T) {
	for _, tc := range bestPassDateCases {
		t.Run(tc.name, func(t *testing.T) {
			var a BestPassAssignment
			require.NoError(t, json.Unmarshal([]byte(withDate(`"unit":"U-7"`, "inactiveDate", tc.json)), &a))
			assert.Equal(t, "U-7", a.Unit)
			if tc.want == nil {
				assert.Nil(t, a.InactiveDate)
				return
			}
			require.NotNil(t, a.InactiveDate)
			assert.True(t, tc.want.Equal(*a.InactiveDate), "got %s", *a.InactiveDate)
		})
	}
}

// The live SAAD answers (2026-11-10): the whole list reads, and a tag whose
// assignment ended stays in the list with no assigned unit.
func TestBestPassAPI_LiveDateShapesReadTheWholeList(t *testing.T) {
	hook := func(w http.ResponseWriter, r *http.Request) bool {
		page := map[string]any{"pagingInfo": map[string]any{"nextPage": nil}}
		switch r.URL.Path {
		case "/statements":
			page["results"] = []map[string]any{
				{"statementNumber": "S-10", "dateCreated": "2026-10-01T01:00:00.817", "usageCutoffDate": "2026-09-30T23:59:59.997"},
				{"statementNumber": "S-11", "dateCreated": "2026-11-01T01:00:00.4"},
			}
		case "/equipment":
			page["results"] = []map[string]any{
				{"equipmentNumber": "T-1", "statusCode": "ACTIVE", "activeDate": "2024-01-05T00:00:00",
					"assignedVehicle": map[string]any{"unit": "U-1", "inactiveDate": nil}},
				{"equipmentNumber": "T-2", "statusCode": "INACTIVE", "activeDate": "2023-02-01T00:00:00",
					"assignedVehicle": map[string]any{"unit": "U-2", "inactiveDate": "2024-11-01T00:00:00"}},
			}
		default:
			return false
		}
		_ = json.NewEncoder(w).Encode(page)
		return true
	}
	c, _ := newFakeClient(t, &fakeBestPass{hook: hook})

	st, err := c.Statements(t.Context(), "90001")
	require.NoError(t, err)
	require.Len(t, st, 2)
	assert.Equal(t, time.Date(2026, 10, 1, 1, 0, 0, 817_000_000, time.UTC), st[0].DateCreated)

	eq, err := c.Equipment(t.Context(), "90001")
	require.NoError(t, err)
	require.Len(t, eq, 2)
	assert.Equal(t, "U-1", eq[0].AssignedUnit())
	assert.Equal(t, "", eq[1].AssignedUnit())
	require.NotNil(t, eq[1].AssignedVehicle.InactiveDate)
	assert.Equal(t, time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC), *eq[1].AssignedVehicle.InactiveDate)
}
