package toll

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The layout of a real weekly PrePass export, de-identified.
//
// Rebuilt cell by cell from an actual file rather than simplified, because
// every quirk below is one the reader has to survive and none of them survive
// a hand-written CSV — which is exactly what QA sent DEV-1799 back for:
//
//   - the header row carries a LINE BREAK inside two of its names, "Entry\r\n
//     Plaza" and "Exit\r\nPlaza", so a reader matching on the literal string
//     misses both columns;
//   - column Q has no header at all — an unnamed spacer sitting between Mi and
//     Toll $, so anything addressing columns by position bills the wrong number;
//   - Post Date is an Excel SERIAL while Invoice Date, in the same row, is
//     text;
//   - Exit Date and Exit Time are the same datetime serial repeated, while
//     Entry Date, Entry Time and Entry Plaza are blank;
//   - Source arrives padded — "EZPass    " — and Mi is empty;
//   - the workbook opens on the data sheet, with the report's cover block on a
//     second sheet.
//
// The carrier, the transponders and the money are invented. The shape is not.
func realFormatSheets(postDate, exitAt time.Time) []testSheet {
	header := []any{
		"Post Date", "Invoice Date", "Source", "Read Type",
		"PP Device ID", "Toll Device ID or Plate", "Truck ID", "Agency",
		"Entry\r\nPlaza", "Entry Date", "Entry Time",
		"Exit\r\nPlaza", "Exit Date", "Exit Time",
		"Cl", "Mi",
		nil, // the unnamed spacer column
		"Toll $",
	}
	dataRow := func(device, agencyRef, truck, agency, exitPlaza string, cl int, amount float64) []any {
		return []any{
			serial(postDate), "2024-08-31", "EZPass    ", "Transponder",
			device, agencyRef, truck, agency,
			nil, nil, nil,
			exitPlaza, serial(exitAt), serial(exitAt),
			cl, nil,
			nil,
			amount,
		}
	}
	return []testSheet{
		{
			name: "Sheet1",
			rows: [][]any{
				header,
				dataRow("700000001", "01100000001", "206", "ILTOLL", "82nd St.", 5, 7.7),
				dataRow("700000001", "01100000001", "206", "ILTOLL", "95th St.", 5, 5.8),
				dataRow("700000002", "01100000002", "207", "WVPEDTA", "Chelyan", 8, 12.33),
			},
		},
		{
			// The cover block PrePass puts on its own sheet. It carries no
			// "Account" label, so the reader must come back with no account
			// rather than mistaking the title for one.
			name: "Sheet2",
			rows: [][]any{
				{"Customer Toll Details\r\nSAMPLE CARRIER CO 000000, Agency: All Agencies\r\nPost Dates Between 9/2/2024 and 9/8/2024"},
			},
		},
	}
}

// The whole point: this file, as PrePass actually writes it, reads cleanly.
func TestParse_RealPrePassLayout(t *testing.T) {
	postDate := time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC)
	exitAt := time.Date(2024, 9, 6, 17, 23, 47, 0, time.UTC)

	p := NewPrePassSFTP(Credential{ProviderType: ProviderPrePassSFTP})
	res, err := p.Parse("prepass-2024-09-02-to-08.xlsx", buildXLSX(t, realFormatSheets(postDate, exitAt)))
	require.NoError(t, err)
	require.Empty(t, res.Errors, "a real weekly export must not produce row errors")
	require.Len(t, res.Rows, 3)

	first := res.Rows[0]
	assert.Equal(t, postDate, first.PostDate, "Post Date arrives as an Excel serial, not a date string")
	assert.InDelta(t, 7.7, first.Amount.InexactFloat64(), 0.001,
		"the unnamed spacer column must not shift Toll $")
	assert.Equal(t, "82nd St.", first.ExitPlaza,
		`the header reads "Exit\r\nPlaza" — matching the literal name finds nothing`)
	assert.Empty(t, first.EntryPlaza, "Entry Plaza is genuinely blank in this export")
	assert.Equal(t, "EZPass", first.Source, "Source arrives padded")
	assert.Equal(t, "206", first.TruckRef)
	assert.Equal(t, ReadTransponder, first.ReadType)
	require.NotNil(t, first.ExitAt)
	assert.Equal(t, exitAt.Truncate(time.Second), first.ExitAt.Truncate(time.Second))

	assert.Equal(t, "WVPEDTA", res.Rows[2].Agency, "later rows keep their own agency")
	assert.InDelta(t, 12.33, res.Rows[2].Amount.InexactFloat64(), 0.001)

	assert.Empty(t, res.Account,
		"the cover sheet has no Account label; inventing one from the title would be worse than none")
}

// The same layout with the spacer column removed shifts every name one to the
// left. Addressing by name has to be immune to that — this is the regression
// that made column-by-position unusable in the first place.
func TestParse_RealPrePassLayout_SurvivesTheSpacerMoving(t *testing.T) {
	postDate := time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC)
	exitAt := time.Date(2024, 9, 6, 17, 23, 47, 0, time.UTC)

	sheets := realFormatSheets(postDate, exitAt)
	for i := range sheets[0].rows {
		row := sheets[0].rows[i]
		sheets[0].rows[i] = append(row[:16:16], row[17]) // drop the spacer at Q
	}

	p := NewPrePassSFTP(Credential{ProviderType: ProviderPrePassSFTP})
	res, err := p.Parse("prepass-no-spacer.xlsx", buildXLSX(t, sheets))
	require.NoError(t, err)
	require.Len(t, res.Rows, 3)
	assert.InDelta(t, 7.7, res.Rows[0].Amount.InexactFloat64(), 0.001)
	assert.Equal(t, "82nd St.", res.Rows[0].ExitPlaza)
}

// A workbook that is not a PrePass export must fail as a whole, naming what it
// could not find. The import records that reason and the screen shows it.
func TestParse_NotAPrePassExportNamesTheMissingColumns(t *testing.T) {
	sheets := []testSheet{{
		name: "Sheet1",
		rows: [][]any{
			{"Truck", "Driver", "Miles"},
			{"206", "A. Driver", 431},
		},
	}}

	p := NewPrePassSFTP(Credential{ProviderType: ProviderPrePassSFTP})
	_, err := p.Parse("payroll.xlsx", buildXLSX(t, sheets))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payroll.xlsx")
	assert.Contains(t, err.Error(), colPostDate)
	assert.Contains(t, err.Error(), colAmount)
}

// The daily csv PrePass drops into the SFTP folder, as it is actually written.
//
// Same fields as the weekly workbook above and a different spelling for almost
// every one of them, which is the whole of DEV-1961: the reader found no
// "Post Date" / "Toll $" column, failed the file, and the claim it had already
// taken made the next Check folder answer "already taken".
//
// The header is copied from a live prod file; the rows are invented. Both
// filename styles seen in the folder carry this exact header.
const sftpDailyHeader = "PostingDate,InvoiceDate,CustID,Source,ReadType,PPTagID,ETagID_Plate," +
	"EquipID,Agency,Entry_Plaza,Entry_Time,Entry_Date,Exit_Plaza,Exit_Date,Exit_Time," +
	"Toll_Class,Miles,Toll_Amount"

func TestParse_SFTPDailyCSV(t *testing.T) {
	// A UTF-8 BOM leads every one of these files.
	csv := "\xEF\xBB\xBF" + sftpDailyHeader + "\n" +
		"8/20/26,2026-08-31,577048,EZPass    ,Transponder,785049753,01606764587,707,WVPEDTA,,,,Pax,08/19/26,9:36:24,8,,13.00\n" +
		"8/20/26,2026-08-31,577048,ELITE     ,Transponder,785026226,01606695783,689,FTE,,,,154 - SR 91,08/19/26,0:45:11,5,,8.85\n"

	p := NewPrePassSFTP(Credential{ProviderType: ProviderPrePassSFTP})
	res, err := p.Parse("577048_AZSAFILLC_Tolls_From_2026-08-20.csv", []byte(csv))
	require.NoError(t, err, "the daily csv must read, not fail on a missing sheet")
	require.Empty(t, res.Errors, "no row of a real daily file is unreadable")
	require.Len(t, res.Rows, 2)

	first := res.Rows[0]
	assert.Equal(t, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), first.PostDate,
		`PostingDate is "8/20/26" — a two-digit year, which is the other half of the bug`)
	assert.InDelta(t, 13.00, first.Amount.InexactFloat64(), 0.001, "Toll_Amount carries no $")
	assert.Equal(t, "Pax", first.ExitPlaza)
	assert.Equal(t, "WVPEDTA", first.Agency)
	assert.Equal(t, "EZPass", first.Source, "Source arrives padded here too")
	assert.Equal(t, "785049753", first.DeviceID, "PPTagID is the transponder")
	assert.Equal(t, "01606764587", first.AgencyRef, "ETagID_Plate is the agency's tag")
	assert.Equal(t, "707", first.TruckRef, "EquipID is the carrier's own unit")
	assert.Equal(t, ReadTransponder, first.ReadType)
	assert.Equal(t, "8", first.Class)
	require.NotNil(t, first.InvoiceDate)
	assert.Equal(t, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), *first.InvoiceDate,
		"InvoiceDate keeps its four-digit form in the same row")
	require.NotNil(t, first.ExitAt)
	assert.Equal(t, time.Date(2026, 8, 19, 9, 36, 24, 0, time.UTC), *first.ExitAt,
		"Exit_Date holds no clock, so Exit_Time has to be spliced in")
	assert.Nil(t, first.EntryAt, "Entry_* are genuinely blank in the daily file")

	// Midnight-hour times are the case a splice can silently drop.
	require.NotNil(t, res.Rows[1].ExitAt)
	assert.Equal(t, time.Date(2026, 8, 19, 0, 45, 11, 0, time.UTC), *res.Rows[1].ExitAt)
	assert.InDelta(t, 8.85, res.Rows[1].Amount.InexactFloat64(), 0.001)

	// The jsonb the pool keeps must show the name the FILE used, not the name
	// we look the column up by — that column is what makes an unmodelled field
	// recoverable later.
	assert.Equal(t, "13.00", first.Raw["Toll_Amount"],
		"Raw keeps the file's own header spelling")
}

// The folder holds two filename styles over one header. Neither the extension
// nor the name may decide anything.
func TestParse_SFTPDailyCSV_BothFilenameStyles(t *testing.T) {
	csv := sftpDailyHeader + "\n" +
		"8/9/26,2026-08-31,577048,EZPass,Transponder,777498908,00413887631,887,ILTOLL,,,,Cermak Rd.,08/08/26,9:21:50,5,,8.45\n"

	p := NewPrePassSFTP(Credential{ProviderType: ProviderPrePassSFTP})
	for _, name := range []string{
		"577048_AZSAFILLC_Tolls_From_2026-08-20.csv",
		"2026.08.09_577048_AZSAFILLC_Tolls.csv",
	} {
		res, err := p.Parse(name, []byte(csv))
		require.NoError(t, err, name)
		require.Len(t, res.Rows, 1, name)
		assert.Equal(t, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC), res.Rows[0].PostDate, name)
		assert.InDelta(t, 8.45, res.Rows[0].Amount.InexactFloat64(), 0.001, name)
	}
}

// A two-digit year must not cost the four-digit forms already in use.
func TestParseFileTime_TwoDigitYear(t *testing.T) {
	for in, want := range map[string]time.Time{
		"8/20/26":    time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		"08/19/26":   time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC),
		"1/2/26":     time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		"8/20/2026":  time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		"2026-08-31": time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	} {
		got, ok := ParseFileTime(in)
		require.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
}
