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
