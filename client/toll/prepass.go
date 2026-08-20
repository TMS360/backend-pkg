package toll

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PrePass column names, as printed in the weekly spreadsheet. Columns are
// addressed by NAME and never by position: across four consecutive weekly
// files the same "Toll $" column appeared in three different positions,
// because PrePass inserts unnamed spacer columns that shift everything to
// their right. Reading by index would silently mis-map entire files.
const (
	colPostDate    = "Post Date"
	colInvoiceDate = "Invoice Date"
	colSource      = "Source"
	colReadType    = "Read Type"
	colDeviceID    = "PP Device ID"
	colAgencyRef   = "Toll Device ID or Plate"
	colTruckID     = "Truck ID"
	colAgency      = "Agency"
	colEntryPlaza  = "Entry Plaza"
	colEntryDate   = "Entry Date"
	colEntryTime   = "Entry Time"
	colExitPlaza   = "Exit Plaza"
	colExitDate    = "Exit Date"
	colExitTime    = "Exit Time"
	colClass       = "Cl"
	colMiles       = "Mi"
	colAmount      = "Toll $"
)

// colAccount is the label PrePass puts beside the carrier account number on
// the report's cover sheet, when the export includes one.
const colAccount = "Account"

// PrePass default SFTP settings. Unlike factoring these are only fallbacks:
// each carrier is issued its own folder, so host/port/directory normally come
// from the company's stored credential and these apply when it leaves them
// blank.
const (
	prePassSFTPPort = 22

	envTestPrePassHost = "TEST_PREPASS_SFTP_HOST"
	envTestPrePassPort = "TEST_PREPASS_SFTP_PORT"
	envTestPrePassDir  = "TEST_PREPASS_SFTP_DIR"
)

// sftpFetcher is the read-side seam that lets tests replace the network.
// Mirrors factoring's sftpUploader, inverted.
type sftpFetcher interface {
	List(remoteDir string, exts []string) ([]RemoteFile, error)
	Fetch(remoteDir, name string) ([]byte, error)
	Close() error
}

type dialFunc func(context.Context, sftpDialer) (sftpFetcher, error)

func defaultSFTPDial(ctx context.Context, d sftpDialer) (sftpFetcher, error) {
	return dialSFTP(ctx, d)
}

// PrePassSFTP pulls the weekly toll spreadsheet PrePass drops into a carrier's
// SFTP folder. PrePass publishes no API — the file is the entire interface,
// and it carries no transaction id, which is why rows are deduplicated by
// content hash rather than by key.
type PrePassSFTP struct {
	host        string
	port        int
	directory   string
	username    string
	password    string
	hostKey     string
	accountName string

	dialFn dialFunc
}

// NewPrePassSFTP builds the provider from a stored credential, applying the
// non-production folder override when one is configured.
func NewPrePassSFTP(cred Credential) *PrePassSFTP {
	host := strings.TrimSpace(cred.Host)
	port := cred.Port
	dir := strings.TrimSpace(cred.Directory)

	// On non-production deployments point every carrier at our own catcher
	// folder regardless of what the database says. Staging databases are
	// restored from production, so without this a stage run would reach into
	// the real PrePass account.
	if isNonProdAppEnv() {
		if v := firstNonEmptyEnv(envTestPrePassHost); v != "" {
			host = v
		}
		if v := firstNonEmptyEnv(envTestPrePassPort); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				port = n
			}
		}
		if v := firstNonEmptyEnv(envTestPrePassDir); v != "" {
			dir = v
		}
	}
	if port == 0 {
		port = prePassSFTPPort
	}

	return &PrePassSFTP{
		host:        host,
		port:        port,
		directory:   dir,
		username:    cred.Username,
		password:    cred.Password,
		hostKey:     cred.HostKey,
		accountName: strings.TrimSpace(cred.AccountName),
		dialFn:      defaultSFTPDial,
	}
}

func (p *PrePassSFTP) dialer() sftpDialer {
	return sftpDialer{
		Host:         p.host,
		Port:         p.port,
		Username:     p.username,
		Password:     p.password,
		ProviderType: ProviderPrePassSFTP,
		HostKey:      p.hostKey,
	}
}

// List returns the spreadsheets waiting in the carrier's folder, newest first.
func (p *PrePassSFTP) List(ctx context.Context) ([]RemoteFile, error) {
	c, err := p.dialFn(ctx, p.dialer())
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	return c.List(p.directory, RulesFor(ProviderPrePassSFTP).FileExtensions)
}

// Fetch downloads one spreadsheet. The remote file is deliberately left in
// place: it is the carrier's own record with PrePass, and re-reading it is
// made harmless by row hashing.
func (p *PrePassSFTP) Fetch(ctx context.Context, name string) ([]byte, error) {
	c, err := p.dialFn(ctx, p.dialer())
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	return c.Fetch(p.directory, name)
}

// TestConnection dials and closes without reading anything.
func (p *PrePassSFTP) TestConnection(ctx context.Context) error {
	c, err := p.dialFn(ctx, p.dialer())
	if err != nil {
		return err
	}
	return c.Close()
}

// Parse reads a PrePass export into normalised rows.
//
// Pure by contract — no network, no clock. This is what the manual-upload path
// calls on bytes a human supplied, so the folder poll and the upload button
// can never diverge in how a file is interpreted.
//
// A malformed row is collected into ParseResult.Errors rather than aborting:
// PrePass files routinely carry a handful of bad rows, and losing the other
// several hundred good ones would cost the carrier real money.
func (p *PrePassSFTP) Parse(name string, content []byte) (ParseResult, error) {
	sheets, err := readSheets(name, content)
	if err != nil {
		return ParseResult{}, err
	}

	// Locate the data tab by its headers. Some exports carry a cover sheet
	// with the report parameters ahead of the data, so the first tab is not
	// reliably the one we want.
	t, ok := findTable(sheets, colPostDate, colAmount)
	if !ok {
		return ParseResult{}, fmt.Errorf(
			"toll: %q has no PrePass data sheet (no header row with %q and %q)",
			name, colPostDate, colAmount)
	}

	res := ParseResult{Account: findAccount(sheets, t.sheet)}
	res.Rows = make([]Row, 0, len(t.rows))

	for i, raw := range t.rows {
		rowNum := t.headerRow + 1 + i // 1-based row in the source sheet
		if isBlank(raw) {
			res.Skipped++
			continue
		}
		row, err := parsePrePassRow(t, rowNum, raw)
		if err != nil {
			var re RowError
			if errors.As(err, &re) {
				res.Errors = append(res.Errors, re)
			} else {
				res.Errors = append(res.Errors, RowError{RowNumber: rowNum, Err: err})
			}
			continue
		}
		res.Rows = append(res.Rows, row)
	}

	assignHashes(ProviderPrePassSFTP, res.Rows)
	return res, nil
}

// parsePrePassRow maps one spreadsheet row onto Row.
//
// Only Post Date and the amount are hard requirements — Post Date buckets the
// charge into a pay week and the amount is the money. Everything else is
// best-effort, because an unmatched row still has to reach a human rather than
// vanish.
func parsePrePassRow(t table, rowNum int, raw []string) (Row, error) {
	postRaw := t.cell(raw, colPostDate)
	post, ok := ParseFileTime(postRaw)
	if !ok {
		return Row{}, RowError{
			RowNumber: rowNum, Column: colPostDate, Value: postRaw,
			Err: fmt.Errorf("unreadable date"),
		}
	}

	amountRaw := t.cell(raw, colAmount)
	amount, err := ParseMoney(amountRaw)
	if err != nil {
		return Row{}, RowError{RowNumber: rowNum, Column: colAmount, Value: amountRaw, Err: err}
	}

	readType := parseReadType(t.cell(raw, colReadType))
	agencyRef := strings.TrimSpace(t.cell(raw, colAgencyRef))

	row := Row{
		RowNumber:  rowNum,
		PostDate:   post,
		Source:     strings.TrimSpace(t.cell(raw, colSource)),
		Agency:     strings.TrimSpace(t.cell(raw, colAgency)),
		ReadType:   readType,
		DeviceID:   NormalizeDeviceID(t.cell(raw, colDeviceID)),
		AgencyRef:  agencyRef,
		TruckRef:   NormalizeTruckRef(t.cell(raw, colTruckID)),
		EntryPlaza: strings.TrimSpace(t.cell(raw, colEntryPlaza)),
		ExitPlaza:  strings.TrimSpace(t.cell(raw, colExitPlaza)),
		Class:      strings.TrimSpace(t.cell(raw, colClass)),
		Miles:      strings.TrimSpace(t.cell(raw, colMiles)),
		Amount:     amount,
		Raw:        t.rowMap(raw),
	}

	// When the plaza could not read a transponder it photographs the plate and
	// writes it into the same column that otherwise holds the agency's tag
	// number. The two are only distinguishable by Read Type.
	if readType == ReadPlate {
		row.Plate = NormalizePlate(agencyRef)
	}

	if inv, ok := ParseFileTime(t.cell(raw, colInvoiceDate)); ok {
		row.InvoiceDate = &inv
	}
	row.EntryAt = combineDateTime(t, raw, colEntryDate, colEntryTime)
	row.ExitAt = combineDateTime(t, raw, colExitDate, colExitTime)

	return row, nil
}

// parseReadType maps the file's wording onto the enum. An unrecognised value
// is kept as ReadUnknown rather than rejected — the charge is still real, and
// matching falls back to whichever identifier the row happens to carry.
func parseReadType(s string) ReadType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "transponder", "tag", "toll tag":
		return ReadTransponder
	case "plate", "licence plate", "license plate", "image", "video":
		return ReadPlate
	default:
		return ReadUnknown
	}
}

// combineDateTime reads a date column and, when the date carries no clock,
// splices in the time column beside it.
//
// PrePass writes the full timestamp into BOTH columns, so the date alone is
// usually enough; the splice covers exports that put a bare date in one and a
// bare time in the other.
func combineDateTime(t table, raw []string, dateCol, timeCol string) *time.Time {
	d, ok := ParseFileTime(t.cell(raw, dateCol))
	if !ok {
		return nil
	}
	if d.Hour() != 0 || d.Minute() != 0 || d.Second() != 0 {
		return &d
	}
	h, m, s, ok := parseTimeOfDay(t.cell(raw, timeCol))
	if !ok {
		return &d
	}
	out := time.Date(d.Year(), d.Month(), d.Day(), h, m, s, 0, d.Location())
	return &out
}

// timeOfDayLayouts are the clock-only forms seen in the time columns.
var timeOfDayLayouts = []string{"15:04:05", "15:04", "3:04:05 PM", "3:04 PM"}

// parseTimeOfDay reads a clock from a cell that may hold a full timestamp, a
// bare clock, or a fraction-of-day serial (Excel stores "17:23:46" as
// 0.7248 when the cell holds a time and no date).
func parseTimeOfDay(s string) (hour, minute, second int, ok bool) {
	v := strings.TrimSpace(s)
	if v == "" {
		return 0, 0, 0, false
	}
	if ts, found := ParseFileTime(v); found {
		return ts.Hour(), ts.Minute(), ts.Second(), true
	}
	for _, layout := range timeOfDayLayouts {
		if ts, err := time.Parse(layout, v); err == nil {
			return ts.Hour(), ts.Minute(), ts.Second(), true
		}
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f < 1 {
		secs := int(f*24*60*60 + 0.5)
		return secs / 3600, (secs % 3600) / 60, secs % 60, true
	}
	return 0, 0, 0, false
}

// findAccount digs the carrier account number out of the report's cover
// sheet. PrePass prints it beside an "Account" label; when the export has no
// cover sheet the result is empty and the caller simply skips the check.
func findAccount(sheets []sheet, dataSheet string) string {
	for _, s := range sheets {
		if s.name == dataSheet {
			continue
		}
		for _, row := range s.rows {
			for c, cell := range row {
				if !strings.EqualFold(normalizeHeader(cell), colAccount) {
					continue
				}
				// The value sits in the next non-empty cell to the right;
				// PrePass pads label/value pairs with merged blank columns.
				for _, v := range row[c+1:] {
					if v = strings.TrimSpace(v); v != "" {
						return v
					}
				}
			}
		}
	}
	return ""
}

// firstNonEmptyEnv returns the first of the named env vars whose trimmed value
// is non-empty.
func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// isNonProdAppEnv reports whether the deployment may honour the TEST_* folder
// overrides. An allowlist, not a deny-list against "prod", so an empty or
// misspelled APP_ENV is treated as production and the override is ignored.
func isNonProdAppEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "dev", "stage", "staging", "local":
		return true
	default:
		return false
	}
}
