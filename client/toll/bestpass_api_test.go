package toll

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apiTxFromSampleRow builds the Fleet API Transaction BestPass would send for
// one row of the real sample export, so the API path is checked against the
// same 19 tolls the file reader is.
func apiTxFromSampleRow(t *testing.T, header, row []string) map[string]any {
	t.Helper()
	col := func(name string) string {
		for i, h := range header {
			if h == name {
				return row[i]
			}
		}
		t.Fatalf("no column %q", name)
		return ""
	}
	post, err := time.Parse("1/2/2006", col(bpColPostDate))
	require.NoError(t, err)
	exitDay, err := time.Parse("1/2/2006", col(bpColExitDate))
	require.NoError(t, err)
	id, err := strconv.ParseInt(col(bpColTransactionID), 10, 64)
	require.NoError(t, err)
	num := func(s string) float64 {
		f, err := strconv.ParseFloat(s, 64)
		require.NoError(t, err)
		return f
	}
	return map[string]any{
		"transactionId":          id,
		"tollPostDate":           post.Format(time.RFC3339),
		"chargeAmount":           num(col(bpColAmount)),
		"discountedAmount":       num(col(bpColDiscounted)),
		"feeAmount":              num(col(bpColFee)),
		"deviceNumber":           col(bpColTag),
		"licensePlate":           col(bpColPlate),
		"unit":                   col(bpColUnit),
		"exitDateLocal":          exitDay.Format("2006-01-02") + "T" + col(bpColExitTime),
		"exitDate":               exitDay.Add(5 * time.Hour).Format(time.RFC3339), // UTC instant — never used
		"exitPlazaName":          col(bpColExitPlaza),
		"transactionClass":       col(bpColClass),
		"agency":                 col(bpColAgency),
		"serviceName":            col(bpColService),
		"transactionType":        "Toll",
		"transactionDescription": "TOLL",
		"location":               map[string]any{"latitude": 41.7, "longitude": -88.3},
	}
}

// fakeBestPass serves /transactions in pages of pageSize from txs, plus
// /permissions/accounts and /statements. hook may answer instead.
type fakeBestPass struct {
	txs      []map[string]any
	pageSize int
	hook     func(w http.ResponseWriter, r *http.Request) bool
	calls    atomic.Int32
	seenKey  atomic.Value
}

func (f *fakeBestPass) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.seenKey.Store(r.Header.Get("Authorization"))
	if f.hook != nil && f.hook(w, r) {
		return
	}
	switch r.URL.Path {
	case "/permissions/accounts":
		_ = json.NewEncoder(w).Encode([]map[string]string{{"accountNumber": "59471", "accountName": "SAAD Express LLC"}})
	case "/statements":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pagingInfo": map[string]any{"nextPage": nil},
			"results":    []map[string]any{{"statementNumber": "S-1", "dateCreated": "2026-10-01T00:00:00Z"}},
		})
	case "/transactions":
		page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
		from := (page - 1) * f.pageSize
		to := min(from+f.pageSize, len(f.txs))
		var next any
		if to < len(f.txs) {
			next = fmt.Sprintf("/transactions?pageNumber=%d", page+1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pagingInfo": map[string]any{"nextPage": next},
			"results":    f.txs[min(from, len(f.txs)):to],
		})
	default:
		http.NotFound(w, r)
	}
}

func newFakeClient(t *testing.T, f *fakeBestPass) (*BestPassAPI, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := NewBestPassAPI(Credential{ProviderType: ProviderBestPass, Secret: "key-1", BaseURL: srv.URL})
	var waits []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	return c, &waits
}

func sampleTxs(t *testing.T) []map[string]any {
	header, rows := sampleTable(t)
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, apiTxFromSampleRow(t, header, r))
	}
	return out
}

var window = [2]time.Time{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)}

func TestBestPassAPI_RowsMatchTheFileRowsOfTheSameTolls(t *testing.T) {
	file, err := BestPass{}.Parse("bestpass.csv", readBestPassSample(t))
	require.NoError(t, err)
	c, _ := newFakeClient(t, &fakeBestPass{txs: sampleTxs(t), pageSize: 7})

	api, err := c.Transactions(t.Context(), "59471", window[0], window[1])
	require.NoError(t, err)

	require.Empty(t, api.Errors)
	require.Len(t, api.Rows, len(file.Rows), "three pages of 7 must all be read")
	byHash := map[string]Row{}
	for _, r := range file.Rows {
		byHash[r.Hash] = r
	}
	for _, r := range api.Rows {
		f, ok := byHash[r.Hash]
		require.True(t, ok, "API row %d has no file twin: the dedup key differs", r.RowNumber)
		assert.True(t, f.Amount.Equal(r.Amount), "amount %s vs file %s", r.Amount, f.Amount)
		assert.Equal(t, f.ExitAt, r.ExitAt, "exit must be the plaza wall clock, like the file")
		assert.Equal(t, f.DeviceID, r.DeviceID)
		assert.Equal(t, f.Plate, r.Plate)
		assert.Equal(t, f.TruckRef, r.TruckRef)
		assert.Equal(t, f.Agency, r.Agency)
	}
	assert.Equal(t, "59471", api.Account)
}

func TestBestPassAPI_SendsKeyAccountAndPostDateWindow(t *testing.T) {
	var q atomic.Value
	f := &fakeBestPass{pageSize: 10, hook: func(_ http.ResponseWriter, r *http.Request) bool {
		q.Store(r.URL.Query())
		return false
	}}
	c, _ := newFakeClient(t, f)

	_, err := c.Transactions(t.Context(), "59471", window[0], window[1])
	require.NoError(t, err)

	got := q.Load().(url.Values)
	assert.Equal(t, "59471", got["accountNumber"][0])
	assert.Equal(t, "2026-10-01T00:00:00Z", got["startDate"][0], "post-date window, RFC3339 with zone")
	assert.Equal(t, "2026-10-27T00:00:00Z", got["endDate"][0])
	assert.Equal(t, "10000", got["pageSize"][0])
	assert.Equal(t, "key-1", f.seenKey.Load())
}

func TestBestPassAPI_WaitsOut429AndFinishes(t *testing.T) {
	var hits atomic.Int32
	f := &fakeBestPass{txs: sampleTxs(t)[:3], pageSize: 10}
	f.hook = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/transactions" && hits.Add(1) <= 2 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return true
		}
		return false
	}
	c, waits := newFakeClient(t, f)

	res, err := c.Transactions(t.Context(), "59471", window[0], window[1])

	require.NoError(t, err)
	assert.Len(t, res.Rows, 3)
	assert.Equal(t, []time.Duration{7 * time.Second, 7 * time.Second}, *waits)
}

func TestBestPassAPI_RefusedKeyIsAuthError(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		f := &fakeBestPass{hook: func(w http.ResponseWriter, _ *http.Request) bool { w.WriteHeader(code); return true }}
		c, _ := newFakeClient(t, f)

		_, err := c.Accounts(t.Context())
		assert.True(t, IsAuthError(err), "HTTP %d", code)
		_, err = c.Transactions(t.Context(), "59471", window[0], window[1])
		assert.True(t, IsAuthError(err), "HTTP %d through paging", code)
	}
}

func TestBestPassAPI_ServerErrorFailsTheCall(t *testing.T) {
	f := &fakeBestPass{hook: func(w http.ResponseWriter, _ *http.Request) bool { w.WriteHeader(http.StatusBadGateway); return true }}
	c, _ := newFakeClient(t, f)

	_, err := c.Transactions(t.Context(), "59471", window[0], window[1])
	require.Error(t, err)
	assert.False(t, IsAuthError(err))
}

func TestBestPassAPI_EmptyWindowIsZeroRows(t *testing.T) {
	c, _ := newFakeClient(t, &fakeBestPass{pageSize: 10})

	res, err := c.Transactions(t.Context(), "59471", window[0], window[1])

	require.NoError(t, err)
	assert.Empty(t, res.Rows)
	assert.Empty(t, res.Errors)
}

func TestBestPassAPI_RefusesWindowOver31Days(t *testing.T) {
	c, _ := newFakeClient(t, &fakeBestPass{})
	_, err := c.Transactions(t.Context(), "59471", window[0], window[0].Add(BestPassMaxWindow+time.Second))
	require.Error(t, err)
}

func TestBestPassAPI_UnknownWordsAreKeptInRaw(t *testing.T) {
	tx := sampleTxs(t)[0]
	tx["transactionType"] = "Hovercraft"
	tx["transactionDescription"] = "TOLL REVERSAL"
	tx["serviceName"] = "Brand New Network"
	tx["discountedAmount"] = -8.45
	c, _ := newFakeClient(t, &fakeBestPass{txs: []map[string]any{tx}, pageSize: 10})

	res, err := c.Transactions(t.Context(), "59471", window[0], window[1])

	require.NoError(t, err)
	require.Len(t, res.Rows, 1)
	r := res.Rows[0]
	assert.Equal(t, "Hovercraft", r.Raw["transactionType"])
	assert.Equal(t, "TOLL REVERSAL", r.Raw["transactionDescription"])
	assert.Equal(t, "Brand New Network", r.Source)
	assert.Equal(t, "-8.45", r.Amount.String(), "a reversal keeps its sign")
	assert.Contains(t, r.Raw["location"], "latitude", "nested objects kept as JSON")
}

func TestBestPassAPI_FeeAddsAndNullDiscountFallsBackToCharge(t *testing.T) {
	tx := sampleTxs(t)[0]
	tx["chargeAmount"] = 10.0
	tx["discountedAmount"] = nil
	tx["feeAmount"] = 2.5
	c, _ := newFakeClient(t, &fakeBestPass{txs: []map[string]any{tx}, pageSize: 10})

	res, err := c.Transactions(t.Context(), "59471", window[0], window[1])

	require.NoError(t, err)
	require.Len(t, res.Rows, 1)
	assert.Equal(t, "12.5", res.Rows[0].Amount.String())
}

func TestBestPassAPI_BadTransactionIsARowError(t *testing.T) {
	good := sampleTxs(t)[0]
	bad := map[string]any{"transactionId": 1, "chargeAmount": 1}
	c, _ := newFakeClient(t, &fakeBestPass{txs: []map[string]any{good, bad}, pageSize: 10})

	res, err := c.Transactions(t.Context(), "59471", window[0], window[1])

	require.NoError(t, err)
	assert.Len(t, res.Rows, 1)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, 2, res.Errors[0].RowNumber)
}

func TestBestPassAPI_AccountsAndStatements(t *testing.T) {
	c, _ := newFakeClient(t, &fakeBestPass{})

	acc, err := c.Accounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []BestPassAccount{{Number: "59471", Name: "SAAD Express LLC"}}, acc)

	st, err := c.Statements(t.Context(), "59471")
	require.NoError(t, err)
	require.Len(t, st, 1)
	assert.Equal(t, "S-1", st[0].Number)
}

func TestBestPassAPI_DefaultsToProductionURL(t *testing.T) {
	assert.Equal(t, BestPassAPIBaseURL, NewBestPassAPI(Credential{}).baseURL)
}

func TestBestPass_APIModeNeedsTheKey(t *testing.T) {
	assert.Error(t, Credential{ProviderType: ProviderBestPass}.Validate())
	assert.NoError(t, Credential{ProviderType: ProviderBestPass, Secret: "k"}.Validate())
}

func TestBestPassAPI_ExitWithOffsetKeepsThePlazaClock(t *testing.T) {
	tx := sampleTxs(t)[0]
	tx["exitDateLocal"] = "2026-10-25T16:13:20-05:00"
	c, _ := newFakeClient(t, &fakeBestPass{txs: []map[string]any{tx}, pageSize: 10})

	res, err := c.Transactions(t.Context(), "59471", window[0], window[1])

	require.NoError(t, err)
	require.Len(t, res.Rows, 1)
	assert.Equal(t, time.Date(2026, 10, 25, 16, 13, 20, 0, time.UTC), *res.Rows[0].ExitAt,
		"the file prints 16:13:20; converting the offset to UTC would make one toll two crossings")
}
