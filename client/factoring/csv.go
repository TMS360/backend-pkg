package factoring

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
)

// Triumph CSV column order is fixed by the factor and must not be reordered.
// Unknown columns are silently ignored on their end, so keep this minimal.
var triumphCSVHeader = []string{"DTR_NAME", "INVOICE#", "INV_DATE", "PO", "INVAMT"}

// BuildTriumphCSV renders the manifest for Triumph SFTP drops. Spec:
//   - UTF-8, no BOM
//   - CRLF line endings
//   - header row + one row per invoice
//   - INV_DATE formatted as MM/DD/YYYY
//   - INVAMT formatted as %.2f with no currency symbol
//   - RFC 4180 quoting via encoding/csv handles commas/quotes inside DTR_NAME
func BuildTriumphCSV(invoices []InvoiceLine) ([]byte, error) {
	if len(invoices) == 0 {
		return nil, fmt.Errorf("factoring: cannot build CSV with zero invoices")
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.UseCRLF = true

	if err := w.Write(triumphCSVHeader); err != nil {
		return nil, fmt.Errorf("factoring: write csv header: %w", err)
	}

	for i, inv := range invoices {
		if inv.InvoiceNumber == "" {
			return nil, fmt.Errorf("factoring: invoice[%d] missing invoice number", i)
		}
		row := []string{
			inv.DebtorName,
			inv.InvoiceNumber,
			inv.InvoiceDate.Format("01/02/2006"),
			inv.PONumber,
			strconv.FormatFloat(inv.AmountUSD, 'f', 2, 64),
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("factoring: write csv row %d: %w", i, err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("factoring: flush csv: %w", err)
	}

	return buf.Bytes(), nil
}
