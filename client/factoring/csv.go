package factoring

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
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

// RTS CSV column order is fixed by the factor. Header names mirror the example
// spreadsheet RTS distributes to integrators ("FTPS Example Spreadsheet").
var rtsCSVHeader = []string{"Client", "Invoice#", "DebtorNo", "Debtor Name", "Load #", "InvDate", "InvAmt"}

// BuildRTSCSV renders the manifest for RTS Financial SFTP drops. Spec (RTS
// "FTP Integration Requirements" + upload-process doc):
//   - 7 columns in fixed order: Client Number (the SFTP username), Invoice
//     Number, Debtor Number, Debtor Name, PO Number (load number), Invoice
//     Date (MM/DD/YYYY), Invoice Amount (> 0, two decimals)
//   - header row + one row per invoice, CRLF, UTF-8 no BOM
//   - DebtorNo is rendered as DebtorName for now (no debtor-number mapping
//     exists yet — DEV-1312); a future mapping only changes this row assembly
//
// Rule validation (no blanks, no non-positive amounts, ≤999 rows, no duplicate
// invoice numbers) lives in ValidateInvoicesForProvider — this builder only
// renders, so archive callers can always obtain the bytes.
func BuildRTSCSV(clientNumber string, invoices []InvoiceLine) ([]byte, error) {
	if strings.TrimSpace(clientNumber) == "" {
		return nil, fmt.Errorf("factoring: rts csv requires a client number (sftp username)")
	}
	if len(invoices) == 0 {
		return nil, fmt.Errorf("factoring: cannot build CSV with zero invoices")
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.UseCRLF = true

	if err := w.Write(rtsCSVHeader); err != nil {
		return nil, fmt.Errorf("factoring: write csv header: %w", err)
	}

	for i, inv := range invoices {
		if inv.InvoiceNumber == "" {
			return nil, fmt.Errorf("factoring: invoice[%d] missing invoice number", i)
		}
		row := []string{
			clientNumber,
			inv.InvoiceNumber,
			inv.DebtorName, // DebtorNo = DebtorName until a debtor-number mapping exists
			inv.DebtorName,
			inv.PONumber,
			inv.InvoiceDate.Format("01/02/2006"),
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
