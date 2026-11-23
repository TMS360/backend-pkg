package toll

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readBestPassSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/bestpass-sample.csv")
	require.NoError(t, err)
	return b
}

// sampleTable is the sample as header + rows, for building edge files from it.
func sampleTable(t *testing.T) ([]string, [][]string) {
	t.Helper()
	all, err := csv.NewReader(strings.NewReader(string(readBestPassSample(t)))).ReadAll()
	require.NoError(t, err)
	return all[0], all[1:]
}

func setCol(header, row []string, col, v string) []string {
	out := append([]string(nil), row...)
	for i, h := range header {
		if h == col {
			out[i] = v
			return out
		}
	}
	panic("no column " + col)
}

func toCSV(t *testing.T, header []string, rows ...[]string) []byte {
	t.Helper()
	var b strings.Builder
	w := csv.NewWriter(&b)
	require.NoError(t, w.Write(header))
	require.NoError(t, w.WriteAll(rows))
	return []byte(b.String())
}

func TestBestPass_ReadsTheRealSample(t *testing.T) {
	res, err := BestPass{}.Parse("sample-bestpass-10262026.csv", readBestPassSample(t))
	require.NoError(t, err)
	require.Empty(t, res.Errors)
	require.Len(t, res.Rows, 19)
	assert.Equal(t, "90001", res.Account)

	first := res.Rows[0]
	assert.Equal(t, 2, first.RowNumber)
	assert.Equal(t, "8.55", first.Amount.StringFixed(2))
	assert.Equal(t, "Aurora", first.ExitPlaza)
	assert.Equal(t, "", first.EntryPlaza, `"None" is blank`)
	assert.Nil(t, first.EntryAt)
	assert.Equal(t, "ILTOLL", first.Agency)
	assert.Equal(t, "E-ZPass Prepaid", first.Source)
	assert.Equal(t, "QATAG0000001", first.DeviceID)
	assert.Equal(t, "QAPLATE2", first.Plate)
	assert.Equal(t, "QAUNIT1", first.TruckRef)
	assert.Equal(t, "5", first.Class)
	assert.Equal(t, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), first.PostDate)
	require.NotNil(t, first.ExitAt)
	assert.Equal(t, time.Date(2026, 10, 25, 16, 13, 20, 0, time.UTC), *first.ExitAt)
	assert.Equal(t, "900100001", first.Raw["Transaction Id"])
	assert.Equal(t, TransactionRowKey(ProviderBestPass, "900100001"), first.Hash)

	assert.Equal(t, "13.94", res.Rows[4].Amount.StringFixed(2))
	assert.Equal(t, "East End Crossing - NB", res.Rows[4].ExitPlaza)
	assert.Equal(t, "13.10", res.Rows[5].Amount.StringFixed(2))
	assert.Equal(t, "Ghent South", res.Rows[5].ExitPlaza)

	ntta := res.Rows[17]
	assert.Equal(t, "NTTA", ntta.Agency)
	assert.Equal(t, "", ntta.Class, "NTTA prints the lane, not the class")
	require.NotNil(t, ntta.ExitAt)
	assert.Equal(t, time.Date(2026, 10, 6, 8, 47, 15, 0, time.UTC), *ntta.ExitAt)

	keys := map[string]bool{}
	for _, r := range res.Rows {
		keys[r.Hash] = true
	}
	assert.Len(t, keys, 19)
}

func TestBestPass_ExcelReadsLikeCSV(t *testing.T) {
	header, rows := sampleTable(t)
	sheetRows := [][]any{toAny(header)}
	for _, r := range rows {
		sheetRows = append(sheetRows, toAny(r))
	}
	res, err := BestPass{}.Parse("export.xlsx", buildXLSX(t, []testSheet{{name: "Transactions", rows: sheetRows}}))
	require.NoError(t, err)
	require.Empty(t, res.Errors)
	require.Len(t, res.Rows, 19)
	csvRes, err := BestPass{}.Parse("export.csv", readBestPassSample(t))
	require.NoError(t, err)
	for i := range res.Rows {
		assert.Equal(t, csvRes.Rows[i].Hash, res.Rows[i].Hash)
		assert.True(t, csvRes.Rows[i].Amount.Equal(res.Rows[i].Amount))
	}
}

func toAny(r []string) []any {
	out := make([]any, len(r))
	for i, v := range r {
		out[i] = v
	}
	return out
}

// parseEdgeFile is the sample with one edge case per row, rows 2..13.
func parseEdgeFile(t *testing.T) ParseResult {
	t.Helper()
	h, s := sampleTable(t)
	negative := setCol(h, setCol(h, s[0], "Discounted Amount", "-8.45"), "Amount", "-8.45")
	reversal := setCol(h, setCol(h, setCol(h, s[1], "Transaction Desc", "TOLL REVERSAL"), "Transaction Id", "9000000001"), "Discounted Amount", "-8.45")
	noPost := setCol(h, s[2], "Post Date", "")
	noDiscount := setCol(h, setCol(h, setCol(h, s[3], "Discounted Amount", ""), "Amount", "9.10"), "Fee Amount", "0.25")
	unknownDesc := setCol(h, s[4], "Transaction Desc", "MYSTERY")
	badAmount := setCol(h, s[5], "Discounted Amount", "twelve")
	noID := setCol(h, s[6], "Transaction Id", "")
	noDates := setCol(h, setCol(h, s[7], "Post Date", ""), "Exit Date", "")
	blankFee := setCol(h, s[8], "Fee Amount", "")
	dupSame := s[9]
	dupOther := setCol(h, s[9], "Discounted Amount", "1.00")

	res, err := BestPass{}.Parse("edge.csv", toCSV(t, h,
		negative, reversal, noPost, noDiscount, unknownDesc, badAmount, noID, noDates, blankFee, s[9], dupSame, dupOther))
	require.NoError(t, err)
	return res
}

func TestBestPass_EdgeFileGoodRowsLoad(t *testing.T) {
	res := parseEdgeFile(t)

	amounts := map[int]string{}
	for _, r := range res.Rows {
		amounts[r.RowNumber] = r.Amount.StringFixed(2)
	}
	assert.Equal(t, "-8.45", amounts[2], "minus kept")
	assert.Equal(t, "-8.45", amounts[3], "reversal loads with its own id")
	assert.Contains(t, amounts, 4, "blank Post Date loads on the exit")
	assert.Equal(t, "9.35", amounts[5], "Amount + Fee when Discounted is blank")
	assert.Contains(t, amounts, 6, "unknown description loads")
	assert.Contains(t, amounts, 8, "blank Transaction Id loads on the content key")
	assert.Equal(t, "8.55", amounts[10], "blank fee is zero")
	assert.Contains(t, amounts, 11)
	assert.Contains(t, amounts, 12, "same id, same amount: kept, the pool counts it as a duplicate")
	require.Len(t, res.Rows, 9)
}

func TestBestPass_EdgeFileBadRowsAreRowErrors(t *testing.T) {
	res := parseEdgeFile(t)
	byRow := map[int]RowError{}
	for _, e := range res.Errors {
		byRow[e.RowNumber] = e
	}
	require.Len(t, byRow, 3)
	assert.Equal(t, "Discounted Amount", byRow[7].Column)
	assert.Equal(t, "twelve", byRow[7].Value)
	assert.Equal(t, "Post Date", byRow[9].Column)
	assert.Equal(t, "Transaction Id", byRow[13].Column)
	assert.EqualError(t, byRow[13].Err, errRepeatedTxAmount)
}

func TestBestPass_EdgeFileKeys(t *testing.T) {
	res := parseEdgeFile(t)
	hash := map[int]string{}
	for _, r := range res.Rows {
		hash[r.RowNumber] = r.Hash
	}
	assert.Equal(t, hash[11], hash[12])
	assert.NotEqual(t, hash[2], hash[3])
	assert.Equal(t, HashRow(ProviderBestPass, rowAt(res, 8), 0), hash[8])
}

func rowAt(res ParseResult, n int) Row {
	for _, r := range res.Rows {
		if r.RowNumber == n {
			r.Hash = ""
			return r
		}
	}
	return Row{}
}

func TestBestPass_KeyIgnoresPostDateAndAmount(t *testing.T) {
	h, s := sampleTable(t)
	a, err := BestPass{}.Parse("a.csv", toCSV(t, h, s[0]))
	require.NoError(t, err)
	b, err := BestPass{}.Parse("b.csv", toCSV(t, h, setCol(h, setCol(h, s[0], "Post Date", "10/27/2026"), "Discounted Amount", "7.00")))
	require.NoError(t, err)
	assert.Equal(t, a.Rows[0].Hash, b.Rows[0].Hash)
	assert.Equal(t, "7.00", b.Rows[0].Amount.StringFixed(2))
}

func TestBestPass_HeaderOnlyIsZeroRows(t *testing.T) {
	h, _ := sampleTable(t)
	res, err := BestPass{}.Parse("empty.csv", toCSV(t, h))
	require.NoError(t, err)
	assert.Empty(t, res.Rows)
	assert.Empty(t, res.Errors)
}

func TestBestPass_MixedAccountsReadAsOneValueNoAccountEquals(t *testing.T) {
	h, s := sampleTable(t)
	res, err := BestPass{}.Parse("mixed.csv", toCSV(t, h, s[0], setCol(h, s[1], "AccountNumber", "11111")))
	require.NoError(t, err)
	assert.Equal(t, "11111, 90001", res.Account)
}

func TestBestPass_WrongProviderFilesAreRefused(t *testing.T) {
	prepass := buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		{"Post Date", "PP Device ID", "Exit Date", "Toll $"},
		{"2026-10-20", "PP1", "2026-10-19 10:00:00", "4.50"},
	}}})
	_, err := BestPass{}.Parse("prepass.xlsx", prepass)
	assert.ErrorIs(t, err, ErrNotBestPassFile)

	_, err = newTestProvider().Parse("bestpass.csv", readBestPassSample(t))
	assert.ErrorIs(t, err, ErrNotPrePassFile)
}

func TestBestPass_IsRegisteredUploadOnly(t *testing.T) {
	cred := Credential{ProviderType: ProviderBestPass, UploadOnly: true}
	require.NoError(t, cred.Validate())
	p, err := NewProviderFromCredential(cred)
	require.NoError(t, err)
	assert.ErrorIs(t, p.TestConnection(t.Context()), ErrUploadOnly)
	assert.Contains(t, AllProviderTypes, ProviderBestPass)
}
