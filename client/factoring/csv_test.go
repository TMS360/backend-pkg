package factoring

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTriumphCSV_HeaderAndBasicRow(t *testing.T) {
	csv, err := BuildTriumphCSV([]InvoiceLine{{
		DebtorName:    "ABC Logistics, LLC",
		InvoiceNumber: "IN-000000",
		InvoiceDate:   time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
		PONumber:      "5555555",
		AmountUSD:     1000.0,
	}})
	require.NoError(t, err)

	got := string(csv)
	lines := strings.Split(got, "\r\n")

	// Expect header, one data row, and a trailing empty element after final CRLF.
	require.Len(t, lines, 3, "expected header + 1 row + trailing empty, got %q", got)
	assert.Equal(t, "DTR_NAME,INVOICE#,INV_DATE,PO,INVAMT", lines[0])
	// DTR_NAME contains a comma so encoding/csv RFC-4180-quotes the field.
	assert.Equal(t, `"ABC Logistics, LLC",IN-000000,01/11/2026,5555555,1000.00`, lines[1])
	assert.Equal(t, "", lines[2])
}

func TestBuildTriumphCSV_AmountFormatting(t *testing.T) {
	csv, err := BuildTriumphCSV([]InvoiceLine{{
		DebtorName:    "X",
		InvoiceNumber: "INV-1",
		InvoiceDate:   time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
		PONumber:      "PO1",
		AmountUSD:     1234.5,
	}})
	require.NoError(t, err)
	// Always two decimals, no thousands separators, no currency.
	assert.Contains(t, string(csv), ",1234.50\r\n")
}

func TestBuildTriumphCSV_DateFormatting(t *testing.T) {
	csv, err := BuildTriumphCSV([]InvoiceLine{{
		DebtorName:    "X",
		InvoiceNumber: "INV-1",
		InvoiceDate:   time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC),
		PONumber:      "PO1",
		AmountUSD:     1,
	}})
	require.NoError(t, err)
	// MM/DD/YYYY with zero-padding.
	assert.Contains(t, string(csv), ",12/05/2026,")
}

func TestBuildTriumphCSV_MultipleRows(t *testing.T) {
	csv, err := BuildTriumphCSV([]InvoiceLine{
		{DebtorName: "A", InvoiceNumber: "INV-1", InvoiceDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), PONumber: "PO1", AmountUSD: 100},
		{DebtorName: "B", InvoiceNumber: "INV-2", InvoiceDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), PONumber: "PO2", AmountUSD: 200},
	})
	require.NoError(t, err)
	// header + 2 rows + trailing empty after final CRLF
	assert.Equal(t, 4, len(strings.Split(string(csv), "\r\n")))
}

func TestBuildTriumphCSV_ErrorsOnEmpty(t *testing.T) {
	_, err := BuildTriumphCSV(nil)
	require.Error(t, err)
}

func TestBuildTriumphCSV_ErrorsOnMissingInvoiceNumber(t *testing.T) {
	_, err := BuildTriumphCSV([]InvoiceLine{{
		DebtorName: "X", InvoiceNumber: "", InvoiceDate: time.Now(), PONumber: "p", AmountUSD: 1,
	}})
	require.Error(t, err)
}
