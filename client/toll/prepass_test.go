package toll

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// Fixtures are built in code rather than committed as sample files: real
// PrePass exports carry a live carrier name, account number, transponder ids
// and licence plates, which do not belong in a repository. Every structural
// quirk these tests exercise was observed in real files:
//
//   - the same column appearing at different positions week to week, because
//     PrePass inserts unnamed spacer columns,
//   - the Cl and Mi columns missing entirely from some weeks,
//   - a cover sheet ahead of the data sheet in some exports and absent in
//     others,
//   - dates as Excel serials while Invoice Date is text in the same file,
//   - $0.00 rows, leading-zero truck numbers, and "IL-"-prefixed plates.

// ---------- fixture helpers ----------

type testSheet struct {
	name string
	rows [][]any
}

func buildXLSX(t *testing.T, sheets []testSheet) []byte {
	t.Helper()
	require.NotEmpty(t, sheets)

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	for i, s := range sheets {
		if i == 0 {
			require.NoError(t, f.SetSheetName(f.GetSheetName(0), s.name))
		} else {
			_, err := f.NewSheet(s.name)
			require.NoError(t, err)
		}
		for r, row := range s.rows {
			for c, v := range row {
				if v == nil {
					continue
				}
				axis, err := excelize.CoordinatesToCellName(c+1, r+1)
				require.NoError(t, err)
				require.NoError(t, f.SetCellValue(s.name, axis, v))
			}
		}
	}

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf.Bytes()
}

func newTestProvider() *PrePassSFTP {
	return NewPrePassSFTP(Credential{
		ProviderType: ProviderPrePassSFTP,
		Host:         "sftp.example.test",
		Username:     "u",
		Secret:       "p",
		Directory:    "/inbound",
	})
}

// serial converts a wall-clock time to the Excel serial the file would carry.
func serial(ts time.Time) float64 {
	return float64(ts.Sub(excelEpoch)) / float64(24*time.Hour)
}

// wideHeader is the 17-column layout, with the unnamed spacer columns PrePass
// leaves between groups (nil entries).
func wideHeader() []any {
	return []any{
		"Post Date", "Invoice Date", "Source", "Read Type", "PP Device ID", nil,
		"Toll Device ID or Plate", "Truck ID", "Agency", nil,
		"Entry\nPlaza", "Entry Date", "Entry Time", nil, nil,
		"Exit\nPlaza", nil, "Exit Date", "Exit Time", "Cl", "Mi", nil, "Toll $",
	}
}

func wideRow(post, exit time.Time, device, ref, truck, plaza, amount any) []any {
	return []any{
		serial(post), "2024-08-31", "EZPass    ", "Transponder", device, nil,
		ref, truck, "ILTOLL", nil,
		"", nil, nil, nil, nil,
		plaza, nil, serial(exit), serial(exit), "5", nil, nil, amount,
	}
}

// narrowHeader is the 15-column layout: no Cl, no Mi, and the columns packed
// tightly with no spacers. Same logical data, entirely different positions.
func narrowHeader() []any {
	return []any{
		"Post Date", "Invoice Date", "Source", "Read Type", "PP Device ID",
		"Toll Device ID or Plate", "Truck ID", "Agency",
		"Entry Plaza", "Entry Date", "Entry Time",
		"Exit Plaza", "Exit Date", "Exit Time", "Toll $",
	}
}

func narrowRow(post, exit time.Time, device, ref, truck, plaza, amount any) []any {
	return []any{
		serial(post), "2024-08-31", "EZPass", "Transponder", device,
		ref, truck, "ILTOLL",
		"", nil, nil,
		plaza, serial(exit), serial(exit), amount,
	}
}

var (
	postDay  = time.Date(2024, 9, 2, 0, 0, 0, 0, time.UTC)
	exitTime = time.Date(2024, 8, 31, 17, 23, 46, 0, time.UTC)
)

// ---------- parsing ----------

func TestParse_ColumnsAreAddressedByNameNotPosition(t *testing.T) {
	p := newTestProvider()

	wide := buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		wideHeader(),
		wideRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
	}}})
	narrow := buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
	}}})

	wideRes, err := p.Parse("wide.xlsx", wide)
	require.NoError(t, err)
	narrowRes, err := p.Parse("narrow.xlsx", narrow)
	require.NoError(t, err)

	require.Len(t, wideRes.Rows, 1)
	require.Len(t, narrowRes.Rows, 1)
	require.Empty(t, wideRes.Errors)
	require.Empty(t, narrowRes.Errors)

	w, n := wideRes.Rows[0], narrowRes.Rows[0]
	assert.Equal(t, "777751461", w.DeviceID)
	assert.Equal(t, "01606029503", w.AgencyRef)
	assert.Equal(t, "206", w.TruckRef)
	assert.Equal(t, "82nd St.", w.ExitPlaza)
	assert.Equal(t, "7.70", w.Amount.StringFixed(2))
	assert.Equal(t, ReadTransponder, w.ReadType)

	// The narrow file has no Cl/Mi at all; everything else must still line up.
	assert.Equal(t, w.DeviceID, n.DeviceID)
	assert.Equal(t, w.AgencyRef, n.AgencyRef)
	assert.Equal(t, w.ExitPlaza, n.ExitPlaza)
	assert.Equal(t, w.Amount.StringFixed(2), n.Amount.StringFixed(2))
	assert.Equal(t, "5", w.Class)
	assert.Empty(t, n.Class)
}

// The dedup key must survive the layout differences above. This is the
// property the real sample files cannot demonstrate — the four weeks they
// cover do not overlap, so no row appears twice anywhere in them.
func TestHash_SameCrossingInDifferentLayoutsHashesEqual(t *testing.T) {
	p := newTestProvider()

	wide, err := p.Parse("wide.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		wideHeader(),
		wideRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
	}}}))
	require.NoError(t, err)

	narrow, err := p.Parse("narrow.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
	}}}))
	require.NoError(t, err)

	assert.Equal(t, wide.Rows[0].Hash, narrow.Rows[0].Hash,
		"the same crossing re-sent in a differently-shaped file must not create a second pool row")
}

func TestHash_DifferentCrossingsDiffer(t *testing.T) {
	p := newTestProvider()
	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
		narrowRow(postDay, exitTime.Add(time.Minute), "777751461", "01606029503", "206", "82nd St.", 7.7),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "95th St.", 7.7),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 5.8),
	}}}))
	require.NoError(t, err)
	require.Len(t, res.Rows, 4)

	seen := map[string]bool{}
	for _, r := range res.Rows {
		assert.False(t, seen[r.Hash], "row %d collided", r.RowNumber)
		seen[r.Hash] = true
	}
}

// Two rows that are byte-identical (possible only when the export omits the
// exit timestamp) must still land as two distinct pool rows, not one.
func TestHash_IdenticalRowsAreNumbered(t *testing.T) {
	p := newTestProvider()
	row := []any{
		serial(postDay), "2024-08-31", "EZPass", "Transponder", "777751461",
		"01606029503", "206", "ILTOLL",
		"", nil, nil,
		"82nd St.", nil, nil, 7.7,
	}
	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(), row, row,
	}}}))
	require.NoError(t, err)
	require.Len(t, res.Rows, 2)
	assert.NotEqual(t, res.Rows[0].Hash, res.Rows[1].Hash)
}

func TestParse_CoverSheetIsSkippedAndAccountExtracted(t *testing.T) {
	p := newTestProvider()
	content := buildXLSX(t, []testSheet{
		{name: "Cover", rows: [][]any{
			{nil, nil, "Customer Toll Details\nACME CARRIERS CO 516484"},
			{},
			{nil, "Account", nil, "516484"},
			{nil, "Start Date", nil, "9/2/2024"},
		}},
		{name: "Data", rows: [][]any{
			narrowHeader(),
			narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
		}},
	})

	res, err := p.Parse("with-cover.xlsx", content)
	require.NoError(t, err)
	require.Len(t, res.Rows, 1)
	assert.Equal(t, "516484", res.Account)
}

func TestParse_PlateRowHasNoDeviceAndNormalisedPlate(t *testing.T) {
	p := newTestProvider()
	row := narrowRow(postDay, exitTime, "", "IL-P1264873", "470", "SR836 EAST", 0.56000000000000005)
	row[3] = "Plate"

	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(), row,
	}}}))
	require.NoError(t, err)
	require.Len(t, res.Rows, 1)

	got := res.Rows[0]
	assert.Equal(t, ReadPlate, got.ReadType)
	assert.Empty(t, got.DeviceID, "a plate read carries no transponder id")
	assert.Equal(t, "IL-P1264873", got.AgencyRef, "the raw plate is kept verbatim for audit")
	assert.Equal(t, "P1264873", got.Plate)
	// The spreadsheet stores $0.56 as 0.56000000000000005; parsing through a
	// float and summing a few thousand of those drifts the week total.
	assert.Equal(t, "0.56", got.Amount.StringFixed(2))
}

func TestParse_ZeroAmountRowIsKept(t *testing.T) {
	p := newTestProvider()
	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "SR836", 0),
	}}}))
	require.NoError(t, err)
	require.Len(t, res.Rows, 1, "a $0.00 read is a real record and must be stored")
	assert.True(t, res.Rows[0].Amount.IsZero())
	assert.Empty(t, res.Errors)
}

func TestParse_DatesAndTimes(t *testing.T) {
	p := newTestProvider()
	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
	}}}))
	require.NoError(t, err)
	got := res.Rows[0]

	assert.Equal(t, "2024-09-02", got.PostDate.Format("2006-01-02"))
	require.NotNil(t, got.ExitAt)
	// Exit runs well ahead of Post — real files showed up to 20 days.
	assert.Equal(t, "2024-08-31 17:23:46", got.ExitAt.Format("2006-01-02 15:04:05"))
	require.NotNil(t, got.InvoiceDate, "Invoice Date arrives as text while the others are serials")
	assert.Equal(t, "2024-08-31", got.InvoiceDate.Format("2006-01-02"))
	assert.Nil(t, got.EntryAt, "entry data is blank on most rows")
}

func TestParse_BadRowIsCollectedNotFatal(t *testing.T) {
	p := newTestProvider()
	bad := narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7)
	bad[0] = "not a date"

	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
		bad,
		narrowRow(postDay, exitTime, "777751462", "01606029504", "207", "95th St.", 5.8),
	}}}))
	require.NoError(t, err, "one unreadable row must not cost the whole file")
	assert.Len(t, res.Rows, 2)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, 3, res.Errors[0].RowNumber, "the row is pointed at by its sheet row number")
	assert.Equal(t, colPostDate, res.Errors[0].Column)
}

func TestParse_BlankRowsAreSkippedNotReported(t *testing.T) {
	p := newTestProvider()
	// The blank row sits between two data rows on purpose: trailing blank rows
	// are dropped by the reader before we ever see them, so only an interior
	// one exercises the skip path.
	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		narrowHeader(),
		narrowRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
		{"", "", ""},
		narrowRow(postDay, exitTime, "777751462", "01606029504", "207", "95th St.", 5.8),
	}}}))
	require.NoError(t, err)
	assert.Len(t, res.Rows, 2)
	assert.Empty(t, res.Errors, "a spacer row is not a defect")
	assert.Equal(t, 1, res.Skipped)
}

func TestParse_CSVUsesTheSameParser(t *testing.T) {
	p := newTestProvider()
	csv := "Post Date,Invoice Date,Source,Read Type,PP Device ID,Toll Device ID or Plate," +
		"Truck ID,Agency,Entry Plaza,Entry Date,Entry Time,Exit Plaza,Exit Date,Exit Time,Toll $\n" +
		"2024-09-02,2024-08-31,EZPass,Transponder,777751461,01606029503,008,ILTOLL,,,,82nd St.,2024-08-31 17:23:46,,7.70\n"

	res, err := p.Parse("weekly.csv", []byte(csv))
	require.NoError(t, err)
	require.Len(t, res.Rows, 1)

	got := res.Rows[0]
	assert.Equal(t, "2024-09-02", got.PostDate.Format("2006-01-02"))
	assert.Equal(t, "7.70", got.Amount.StringFixed(2))
	assert.Equal(t, "8", got.TruckRef, "leading zeros are dropped from the unit number")
	assert.Equal(t, "01606029503", got.AgencyRef, "leading zeros are significant in a device id")
}

func TestParse_NoDataSheetIsAnError(t *testing.T) {
	p := newTestProvider()
	_, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Cover", rows: [][]any{
		{"Customer Toll Details"}, {nil, "Account", nil, "516484"},
	}}}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no PrePass data sheet")
}

func TestParse_RawRowIsPreserved(t *testing.T) {
	p := newTestProvider()
	res, err := p.Parse("f.xlsx", buildXLSX(t, []testSheet{{name: "Sheet1", rows: [][]any{
		wideHeader(),
		wideRow(postDay, exitTime, "777751461", "01606029503", "206", "82nd St.", 7.7),
	}}}))
	require.NoError(t, err)

	raw := res.Rows[0].Raw
	assert.Equal(t, "ILTOLL", raw["Agency"])
	assert.Equal(t, "5", raw["Cl"])
	// The embedded newline in the header is squeezed to a single space so the
	// key is usable, and spacer columns never appear.
	assert.Equal(t, "82nd St.", raw["Exit Plaza"])
	assert.NotContains(t, raw, "")
}

// ---------- value helpers ----------

func TestNormalizePlate(t *testing.T) {
	cases := map[string]string{
		"IL-P1264873": "P1264873",
		"IL P1264873": "P1264873",
		"il-p1264873": "P1264873",
		"P1264873":    "P1264873",
		"ABC123":      "ABC123", // no separator: not a state prefix, keep it
		// Three letters then a dash is a plate with punctuation, not a state
		// prefix — a state code is always two letters, so nothing is stripped.
		"ABC-123": "ABC123",
		" ":       "",
		"":        "",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizePlate(in), "plate %q", in)
	}
}

func TestNormalizeTruckRef(t *testing.T) {
	cases := map[string]string{
		"008":  "8",
		"009":  "9",
		"206":  "206",
		"000":  "0",
		"0A1":  "0A1", // not all digits: leave it alone
		"N/A":  "",
		"n/a":  "",
		"-":    "",
		"":     "",
		" 42 ": "42",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeTruckRef(in), "truck ref %q", in)
	}
}

func TestParseMoney(t *testing.T) {
	cases := map[string]string{
		"7.7":                 "7.70",
		"0.56000000000000005": "0.56",
		"$1,234.50":           "1234.50",
		"0":                   "0.00",
		"":                    "0.00",
		"(12.34)":             "-12.34",
	}
	for in, want := range cases {
		got, err := ParseMoney(in)
		require.NoError(t, err, "money %q", in)
		assert.Equal(t, want, got.StringFixed(2), "money %q", in)
	}

	_, err := ParseMoney("not money")
	assert.Error(t, err)
}

func TestParseFileTime(t *testing.T) {
	ts, ok := ParseFileTime("45537")
	require.True(t, ok)
	assert.Equal(t, "2024-09-02", ts.Format("2006-01-02"))

	ts, ok = ParseFileTime("45535.724837962996")
	require.True(t, ok)
	assert.Equal(t, "2024-08-31 17:23:46", ts.Format("2006-01-02 15:04:05"),
		"the serial's float tail must not leak into the seconds")

	ts, ok = ParseFileTime("2024-08-31")
	require.True(t, ok)
	assert.Equal(t, "2024-08-31", ts.Format("2006-01-02"))

	_, ok = ParseFileTime("")
	assert.False(t, ok)
	_, ok = ParseFileTime("N/A")
	assert.False(t, ok)
	// Small numbers sit in Excel's 1900 leap-year bug window and are refused
	// rather than silently decoded a day off.
	_, ok = ParseFileTime("42")
	assert.False(t, ok)
}

// ---------- transport ----------

type fakeFetcher struct {
	files    []RemoteFile
	blobs    map[string][]byte
	listErr  error
	fetchErr error
	closed   bool
	lastDir  string
	lastExts []string
}

func (f *fakeFetcher) List(dir string, exts []string) ([]RemoteFile, error) {
	f.lastDir, f.lastExts = dir, exts
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.files, nil
}

func (f *fakeFetcher) Fetch(dir, name string) ([]byte, error) {
	f.lastDir = dir
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.blobs[name], nil
}

func (f *fakeFetcher) Close() error { f.closed = true; return nil }

func withFake(p *PrePassSFTP, fake *fakeFetcher) *PrePassSFTP {
	p.dialFn = func(context.Context, sftpDialer) (sftpFetcher, error) { return fake, nil }
	return p
}

func TestList_PassesDirectoryAndAcceptedExtensions(t *testing.T) {
	fake := &fakeFetcher{files: []RemoteFile{{Name: "a.xlsx"}}}
	p := withFake(newTestProvider(), fake)

	got, err := p.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, "/inbound", fake.lastDir)
	assert.Equal(t, []string{".xlsx", ".xls", ".csv"}, fake.lastExts)
	assert.True(t, fake.closed, "the connection is closed even on the happy path")
}

func TestList_EmptyFolderIsNotAnError(t *testing.T) {
	p := withFake(newTestProvider(), &fakeFetcher{})
	got, err := p.List(context.Background())
	require.NoError(t, err, "between weekly drops the folder is simply empty")
	assert.Empty(t, got)
}

func TestFetch_ReturnsBytesAndClosesConnection(t *testing.T) {
	fake := &fakeFetcher{blobs: map[string][]byte{"weekly.xlsx": []byte("payload")}}
	p := withFake(newTestProvider(), fake)

	got, err := p.Fetch(context.Background(), "weekly.xlsx")
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), got)
	assert.True(t, fake.closed)
}

func TestTestConnection_DialsAndCloses(t *testing.T) {
	fake := &fakeFetcher{}
	p := withFake(newTestProvider(), fake)
	require.NoError(t, p.TestConnection(context.Background()))
	assert.True(t, fake.closed, "the probe must not download anything")
}

func TestTestConnection_SurfacesAuthError(t *testing.T) {
	p := newTestProvider()
	p.dialFn = func(context.Context, sftpDialer) (sftpFetcher, error) {
		return nil, &AuthError{ProviderType: ProviderPrePassSFTP, Cause: errors.New("permission denied")}
	}
	err := p.TestConnection(context.Background())
	require.Error(t, err)
	assert.True(t, IsAuthError(err), "bad credentials must be distinguishable from a host being down")
}

func TestHostKeyCallback(t *testing.T) {
	cb, err := hostKeyCallback("")
	require.NoError(t, err)
	assert.NotNil(t, cb, "an empty pin is the documented opt-out, not an error")

	_, err = hostKeyCallback("this is not a key")
	assert.Error(t, err, "a malformed pin must fail loudly rather than fall back to accepting anything")
}

// ---------- registry ----------

func TestNewProviderFromCredential(t *testing.T) {
	p, err := NewProviderFromCredential(Credential{
		ProviderType: ProviderPrePassSFTP,
		Host:         "sftp.example.test",
		Username:     "u",
		Secret:       "p",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)

	_, err = NewProviderFromCredential(Credential{ProviderType: "bestpass_api"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider_type")

	_, err = NewProviderFromCredential(Credential{
		ProviderType: ProviderPrePassSFTP, Host: "h", Username: "u",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing secret")

	_, err = NewProviderFromCredential(Credential{
		ProviderType: ProviderPrePassSFTP, Username: "u", Secret: "p",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing host")
}

func TestProviderTypeUnmarshalGQLRejectsUnknown(t *testing.T) {
	var pt ProviderType
	require.NoError(t, pt.UnmarshalGQL("prepass_sftp"))
	assert.Equal(t, ProviderPrePassSFTP, pt)

	assert.Error(t, pt.UnmarshalGQL("ezpass_ftps"))
	assert.Error(t, pt.UnmarshalGQL(42))
}

// PrePassSFTP must satisfy Provider — the compile-time check that the read
// contract is actually implemented.
var _ Provider = (*PrePassSFTP)(nil)

// ---------- credential shape ----------

func TestCredential_RedactedHidesOnlyTheSecret(t *testing.T) {
	c := Credential{
		ProviderType: ProviderPrePassSFTP,
		AccountName:  "516484",
		Username:     "carrier",
		Secret:       "hunter2",
		Host:         "sftp.example.test",
		Port:         22,
		Directory:    "/inbound",
	}
	r := c.Redacted()

	assert.Equal(t, "[REDACTED]", r.Secret)
	assert.NotContains(t, r.Secret, "hunter2")
	assert.Len(t, r.Secret, len("[REDACTED]"), "the marker must not leak the real length")
	assert.Equal(t, c.Username, r.Username)
	assert.Equal(t, c.Host, r.Host)
	assert.Equal(t, "hunter2", c.Secret, "Redacted returns a copy and must not mutate the original")

	// An unset secret stays unset, so callers can still tell "not configured"
	// from "configured but hidden".
	assert.Empty(t, Credential{}.Redacted().Secret)
}

// The credential is persisted as one opaque blob, so a provider on a transport
// we have not written yet must not need a schema change. Only the fields a
// transport actually uses may appear.
func TestCredential_JSONOmitsUnusedTransportFields(t *testing.T) {
	blob, err := json.Marshal(Credential{
		ProviderType: ProviderPrePassSFTP,
		Username:     "carrier",
		Secret:       "s",
		Host:         "sftp.example.test",
		Directory:    "/inbound",
	})
	require.NoError(t, err)

	for _, unused := range []string{"base_url", "tls_mode", "passive", "port", "host_key", "account_name"} {
		assert.NotContains(t, string(blob), unused,
			"an SFTP credential must not carry other transports' fields")
	}

	// ...and a future FTPS credential round-trips through the same blob with
	// no migration: the fields simply appear.
	passive := true
	ftps := Credential{
		ProviderType: ProviderPrePassSFTP,
		Username:     "u", Secret: "s", Host: "h",
		TLSMode: TLSExplicit, Passive: &passive,
	}
	blob, err = json.Marshal(ftps)
	require.NoError(t, err)

	var back Credential
	require.NoError(t, json.Unmarshal(blob, &back))
	assert.Equal(t, TLSExplicit, back.TLSMode)
	require.NotNil(t, back.Passive)
	assert.True(t, *back.Passive)
}

func TestRulesFor_DrivesValidationPerProvider(t *testing.T) {
	r := RulesFor(ProviderPrePassSFTP)
	assert.Equal(t, TransportSFTP, r.Transport)
	assert.True(t, r.RequiresUsername)
	assert.True(t, r.RequiresSecret)
	assert.True(t, r.RequiresHost)
	assert.False(t, r.RequiresBaseURL, "a folder provider has no base url")

	// An unknown type demands nothing, so callers must gate on IsValid first —
	// which Validate does.
	assert.Equal(t, Rules{}, RulesFor("ezpass_ftps"))
	assert.Error(t, Credential{ProviderType: "ezpass_ftps"}.Validate())
}
