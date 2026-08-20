package toll

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// headerScanDepth is how many leading rows are searched for the header row.
// Aggregators sometimes put a title or a blank line above the column names;
// beyond a handful of rows we are looking at the wrong sheet, not a deep header.
const headerScanDepth = 10

// sheet is one tab of a workbook (or the single table of a CSV) as raw cells.
type sheet struct {
	name string
	rows [][]string
}

// table is a sheet that has been recognised as data: a header row plus the
// rows beneath it, addressable by column name.
type table struct {
	sheet     string
	headers   []string       // normalised, in column order; blanks kept for alignment
	index     map[string]int // normalised-lower header → column index
	rows      [][]string     // data rows below the header
	headerRow int            // 1-based sheet row where the header was found
}

// readSheets parses file bytes into raw sheets, choosing the reader from the
// file extension. Unknown extensions are tried as CSV, because aggregators
// hand out .txt and extensionless files that are comma-separated inside.
func readSheets(name string, content []byte) ([]sheet, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("toll: file %q is empty", name)
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xlsm", ".xls":
		return readXLSXSheets(name, content)
	default:
		return readCSVSheets(name, content)
	}
}

// readXLSXSheets reads every worksheet with RawCellValue set.
//
// Raw values matter: with formatting applied, excelize renders a date cell
// through its number format, so the same column arrives as "45537" from one
// file and "9/2/2024" from another depending on how the sheet was styled.
// Reading raw and converting ourselves (ParseFileTime) makes the result
// independent of the sender's formatting choices.
func readXLSXSheets(name string, content []byte) ([]sheet, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("toll: open %q as xlsx: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	names := f.GetSheetList()
	out := make([]sheet, 0, len(names))
	for _, sn := range names {
		rows, err := f.GetRows(sn, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fmt.Errorf("toll: read sheet %q of %q: %w", sn, name, err)
		}
		out = append(out, sheet{name: sn, rows: rows})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("toll: %q has no worksheets", name)
	}
	return out, nil
}

// readCSVSheets reads a delimited file as a single sheet. The delimiter is
// sniffed rather than assumed — some agencies ship semicolon- or tab-separated
// files with a .csv extension.
func readCSVSheets(name string, content []byte) ([]sheet, error) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = sniffDelimiter(content)
	r.FieldsPerRecord = -1 // ragged rows are normal; short rows are padded later
	r.LazyQuotes = true

	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("toll: read %q as csv: %w", name, err)
		}
		rows = append(rows, rec)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("toll: %q has no rows", name)
	}
	return []sheet{{name: "csv", rows: rows}}, nil
}

// sniffDelimiter picks the separator that appears most often on the first
// non-empty line. Defaults to comma when nothing stands out.
func sniffDelimiter(content []byte) rune {
	line := content
	if i := bytes.IndexByte(content, '\n'); i >= 0 {
		line = content[:i]
	}
	best, bestN := ',', 0
	for _, c := range []rune{',', ';', '\t', '|'} {
		if n := bytes.Count(line, []byte(string(c))); n > bestN {
			best, bestN = c, n
		}
	}
	return best
}

// normalizeHeader collapses a column name to its comparable form: whitespace
// (including the embedded newlines PrePass puts in "Entry\nPlaza") squeezed to
// single spaces, then trimmed. Case is preserved here and folded at lookup.
func normalizeHeader(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// findTable locates the sheet and header row that carry the required columns.
// The first sheet whose header row contains every required column wins, so a
// workbook whose first tab is a report cover sheet still resolves to its data
// tab instead of failing.
func findTable(sheets []sheet, required ...string) (table, bool) {
	for _, s := range sheets {
		limit := min(len(s.rows), headerScanDepth)
		for i := 0; i < limit; i++ {
			t := tableAt(s, i)
			if t.hasAll(required...) {
				return t, true
			}
		}
	}
	return table{}, false
}

// tableAt treats row i of s as the header row.
func tableAt(s sheet, i int) table {
	raw := s.rows[i]
	headers := make([]string, len(raw))
	index := make(map[string]int, len(raw))
	for c, h := range raw {
		n := normalizeHeader(h)
		headers[c] = n
		if n == "" {
			continue // spacer column; keeps column alignment, not addressable
		}
		key := strings.ToLower(n)
		if _, dup := index[key]; !dup {
			index[key] = c
		}
	}
	return table{
		sheet:     s.name,
		headers:   headers,
		index:     index,
		rows:      s.rows[i+1:],
		headerRow: i + 1,
	}
}

func (t table) hasAll(names ...string) bool {
	for _, n := range names {
		if _, ok := t.index[strings.ToLower(normalizeHeader(n))]; !ok {
			return false
		}
	}
	return len(names) > 0
}

// has reports whether the optional column exists. Some files omit columns
// entirely (PrePass drops "Cl" and "Mi" in some weeks), which is not an error.
func (t table) has(name string) bool {
	_, ok := t.index[strings.ToLower(normalizeHeader(name))]
	return ok
}

// cell reads a column from a data row by name, trimmed. Missing columns and
// rows shorter than the header both yield "" — trailing empty cells are
// dropped by the readers, so short rows are the common case, not a defect.
func (t table) cell(row []string, name string) string {
	c, ok := t.index[strings.ToLower(normalizeHeader(name))]
	if !ok || c >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[c])
}

// rowMap materialises a whole data row keyed by column name, skipping spacer
// columns and empty values. This is what gets persisted as jsonb so columns
// the model does not cover yet are still recoverable.
func (t table) rowMap(row []string) map[string]string {
	m := make(map[string]string, len(t.headers))
	for c, h := range t.headers {
		if h == "" || c >= len(row) {
			continue
		}
		if v := strings.TrimSpace(row[c]); v != "" {
			m[h] = v
		}
	}
	return m
}

// isBlank reports whether a row has no content at all — trailing blank lines
// and spacer rows, which are skipped rather than reported as errors.
func isBlank(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}
