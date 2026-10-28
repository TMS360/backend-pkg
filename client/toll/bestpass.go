package toll

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// BestPass column names, as printed in the portal's toll export. Read by name,
// like PrePass: extra, missing or reordered columns do not break the file.
const (
	bpColPostDate      = "Post Date"
	bpColTag           = "Tag No"
	bpColPlate         = "License Plate"
	bpColUnit          = "Unit"
	bpColEntryDate     = "Entry Date"
	bpColEntryTime     = "Entry Time"
	bpColEntryPlaza    = "Entry Plaza Name"
	bpColExitDate      = "Exit Date"
	bpColExitTime      = "Exit Time"
	bpColExitPlaza     = "Exit Plaza Name"
	bpColClass         = "Toll Class"
	bpColMiles         = "Miles"
	bpColAmount        = "Amount"
	bpColDiscounted    = "Discounted Amount"
	bpColFee           = "Fee Amount"
	bpColAgency        = "Agency"
	bpColService       = "Service"
	bpColTransactionID = "Transaction Id"
	bpColAccountNumber = "AccountNumber"
)

// errRepeatedTxAmount is the row error for one Transaction Id printed twice
// in a file with two different amounts.
const errRepeatedTxAmount = "Transaction Id repeated with another amount"

// BestPass reads the toll export a carrier downloads from the BestPass portal
// and uploads by hand (DEV-3421). There is no folder to read yet, so every
// transport call answers ErrUploadOnly; Parse is the whole provider.
type BestPass struct{}

// List has no folder to read.
func (BestPass) List(context.Context) ([]RemoteFile, error) { return nil, ErrUploadOnly }

// Fetch has no folder to read.
func (BestPass) Fetch(context.Context, string) ([]byte, error) { return nil, ErrUploadOnly }

// TestConnection has nothing to dial.
func (BestPass) TestConnection(context.Context) error { return ErrUploadOnly }

// bestPassParsed is one read row with the Transaction Id it is keyed by.
type bestPassParsed struct {
	row  Row
	txID string
}

// Parse reads a BestPass export into rows. Pure, like every Parse. A row that
// cannot be read becomes a RowError; the rest of the file still loads.
func (BestPass) Parse(name string, content []byte) (ParseResult, error) {
	sheets, err := readSheets(name, content)
	if err != nil {
		return ParseResult{}, err
	}
	t, ok := findTable(sheets, bpColTag, bpColTransactionID, bpColAmount)
	if !ok {
		return ParseResult{}, fmt.Errorf("%w: %q has no header row with %q, %q and %q",
			ErrNotBestPassFile, name, bpColTag, bpColTransactionID, bpColAmount)
	}

	res := ParseResult{Account: bestPassAccount(t)}
	parsed := make([]bestPassParsed, 0, len(t.rows))
	for i, raw := range t.rows {
		rowNum := t.headerRow + 1 + i
		if isBlank(raw) {
			res.Skipped++
			continue
		}
		row, err := parseBestPassRow(t, rowNum, raw)
		if err != nil {
			res.Errors = append(res.Errors, asRowError(rowNum, err))
			continue
		}
		parsed = append(parsed, bestPassParsed{row: row, txID: bpCell(t, raw, bpColTransactionID)})
	}

	rows, keyErrs := keyBestPassRows(parsed)
	res.Rows = rows
	res.Errors = append(res.Errors, keyErrs...)
	return res, nil
}

// parseBestPassRow maps one export row onto Row. The date rule is PrePass's:
// Post Date may be blank when the exit is there; text that is not a date, or
// no date at all, is a row error.
func parseBestPassRow(t table, rowNum int, raw []string) (Row, error) {
	postRaw := bpCell(t, raw, bpColPostDate)
	post, hasPost := ParseFileTime(postRaw)
	exitAt := combineDateTime(t, raw, bpColExitDate, bpColExitTime)
	switch {
	case !hasPost && postRaw != "":
		return Row{}, RowError{RowNumber: rowNum, Column: bpColPostDate, Value: postRaw, Err: fmt.Errorf("unreadable date")}
	case !hasPost && exitAt == nil:
		return Row{}, RowError{RowNumber: rowNum, Column: bpColPostDate, Value: postRaw, Err: fmt.Errorf("no post date and no exit date")}
	}

	amount, err := bestPassAmount(t, rowNum, raw)
	if err != nil {
		return Row{}, err
	}

	return Row{
		RowNumber:  rowNum,
		PostDate:   post,
		EntryAt:    combineDateTime(t, raw, bpColEntryDate, bpColEntryTime),
		ExitAt:     exitAt,
		Source:     bpCell(t, raw, bpColService),
		Agency:     bpCell(t, raw, bpColAgency),
		DeviceID:   NormalizeDeviceID(bpCell(t, raw, bpColTag)),
		Plate:      NormalizePlate(bpCell(t, raw, bpColPlate)),
		TruckRef:   NormalizeTruckRef(bpCell(t, raw, bpColUnit)),
		EntryPlaza: bpCell(t, raw, bpColEntryPlaza),
		ExitPlaza:  bpCell(t, raw, bpColExitPlaza),
		Class:      bpCell(t, raw, bpColClass),
		Miles:      bpCell(t, raw, bpColMiles),
		Amount:     amount,
		Raw:        t.rowMap(raw),
	}, nil
}

// bestPassAmount is what the carrier is charged: Discounted Amount (Amount when
// it is blank) plus Fee Amount (blank = 0). The sign is kept as printed — a
// minus is a refund.
func bestPassAmount(t table, rowNum int, raw []string) (decimal.Decimal, error) {
	col := bpColDiscounted
	v := bpCell(t, raw, col)
	if v == "" {
		col = bpColAmount
		v = bpCell(t, raw, col)
	}
	if v == "" {
		return decimal.Zero, RowError{RowNumber: rowNum, Column: bpColAmount, Err: fmt.Errorf("no amount")}
	}
	base, err := ParseMoney(v)
	if err != nil {
		return decimal.Zero, RowError{RowNumber: rowNum, Column: col, Value: v, Err: err}
	}
	feeRaw := bpCell(t, raw, bpColFee)
	fee, err := ParseMoney(feeRaw)
	if err != nil {
		return decimal.Zero, RowError{RowNumber: rowNum, Column: bpColFee, Value: feeRaw, Err: err}
	}
	return base.Add(fee), nil
}

// keyBestPassRows gives every row its dedup key: provider + Transaction Id, so
// a re-export with another Post Date or a revised discount is the same row.
// A row without an id falls back to the content hash PrePass uses. The same id
// twice in one file is a duplicate when the amount agrees and a row error when
// it does not — the file cannot say which amount is right.
func keyBestPassRows(parsed []bestPassParsed) ([]Row, []RowError) {
	rows := make([]Row, 0, len(parsed))
	var errs []RowError
	var noID []int
	amountByID := make(map[string]decimal.Decimal, len(parsed))
	for _, p := range parsed {
		if p.txID == "" {
			noID = append(noID, len(rows))
			rows = append(rows, p.row)
			continue
		}
		if prev, seen := amountByID[p.txID]; seen && !prev.Equal(p.row.Amount) {
			errs = append(errs, RowError{RowNumber: p.row.RowNumber, Column: bpColTransactionID, Value: p.txID, Err: errors.New(errRepeatedTxAmount)})
			continue
		}
		amountByID[p.txID] = p.row.Amount
		p.row.Hash = TransactionRowKey(ProviderBestPass, p.txID)
		rows = append(rows, p.row)
	}
	hashByContent(rows, noID)
	return rows, errs
}

// hashByContent fills the content hash on rows[idx], numbering identical rows
// among themselves exactly as assignHashes does.
func hashByContent(rows []Row, idx []int) {
	if len(idx) == 0 {
		return
	}
	sub := make([]Row, len(idx))
	for i, j := range idx {
		sub[i] = rows[j]
	}
	assignHashes(ProviderBestPass, sub)
	for i, j := range idx {
		rows[j].Hash = sub[i].Hash
	}
}

// bestPassAccount is every distinct AccountNumber in the file, joined. One
// account gives exactly that number; a file mixing accounts gives a value no
// single saved account equals, so the caller's account check refuses it.
func bestPassAccount(t table) string {
	seen := map[string]bool{}
	for _, raw := range t.rows {
		if v := bpCell(t, raw, bpColAccountNumber); v != "" {
			seen[v] = true
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// bpCell reads a cell by column name; BestPass writes "-" and "None" for
// nothing.
func bpCell(t table, raw []string, col string) string {
	v := t.cell(raw, col)
	if v == "-" || strings.EqualFold(v, "none") {
		return ""
	}
	return v
}

// asRowError keeps a RowError as is and wraps anything else onto the row.
func asRowError(rowNum int, err error) RowError {
	var re RowError
	if errors.As(err, &re) {
		return re
	}
	return RowError{RowNumber: rowNum, Err: err}
}
