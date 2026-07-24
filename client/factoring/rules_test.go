package factoring

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validRTSLine(n int) InvoiceLine {
	return InvoiceLine{
		DebtorName:    "Debtor",
		InvoiceNumber: fmt.Sprintf("INV-%05d", n),
		InvoiceDate:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		PONumber:      fmt.Sprintf("SH-%05d", n),
		AmountUSD:     100,
	}
}

func validRTSLines(count int) []InvoiceLine {
	lines := make([]InvoiceLine, 0, count)
	for i := 0; i < count; i++ {
		lines = append(lines, validRTSLine(i))
	}
	return lines
}

func TestRulesFor_TriumphIsZero(t *testing.T) {
	assert.Equal(t, ProviderRules{}, RulesFor(ProviderTriumphSFTP))
}

func TestRulesFor_RTS(t *testing.T) {
	r := RulesFor(ProviderRTSSFTP)
	assert.Equal(t, 999, r.MaxRecords)
	assert.Equal(t, 5*time.Minute, r.MinSubmissionGap)
	assert.True(t, r.RequireUniqueInvoiceNumbers)
	assert.True(t, r.RequirePositiveAmounts)
	assert.True(t, r.RequireAllFields)
}

func TestValidateInvoices_RecordLimit999PassesAnd1000Blocks(t *testing.T) {
	require.NoError(t, ValidateInvoicesForProvider(ProviderRTSSFTP, validRTSLines(999)))

	err := ValidateInvoicesForProvider(ProviderRTSSFTP, validRTSLines(1000))
	require.Error(t, err)
	require.True(t, IsBatchValidationError(err))
	var ve *BatchValidationError
	require.True(t, errors.As(err, &ve))
	assert.Empty(t, ve.InvoiceNumber, "record cap is a batch-level violation")
	assert.Contains(t, err.Error(), "1000")
	assert.Contains(t, err.Error(), "999")
}

func TestValidateInvoices_DuplicateInvoiceNumber(t *testing.T) {
	lines := validRTSLines(3)
	lines[2].InvoiceNumber = lines[0].InvoiceNumber

	err := ValidateInvoicesForProvider(ProviderRTSSFTP, lines)
	require.Error(t, err)
	var ve *BatchValidationError
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, lines[0].InvoiceNumber, ve.InvoiceNumber)
	assert.Contains(t, err.Error(), "duplicate")
	assert.Contains(t, err.Error(), lines[0].InvoiceNumber, "error must name the offending invoice")
}

func TestValidateInvoices_WhitespaceDebtorNameIsBlank(t *testing.T) {
	lines := validRTSLines(2)
	lines[1].DebtorName = "   "

	err := ValidateInvoicesForProvider(ProviderRTSSFTP, lines)
	require.Error(t, err)
	var ve *BatchValidationError
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, lines[1].InvoiceNumber, ve.InvoiceNumber)
	assert.Equal(t, "debtor name", ve.Field)
}

func TestValidateInvoices_BlankPONumber(t *testing.T) {
	lines := validRTSLines(1)
	lines[0].PONumber = ""

	err := ValidateInvoicesForProvider(ProviderRTSSFTP, lines)
	require.Error(t, err)
	var ve *BatchValidationError
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, "po number", ve.Field)
	assert.Contains(t, err.Error(), lines[0].InvoiceNumber)
}

func TestValidateInvoices_NonPositiveAmountBlocks(t *testing.T) {
	for _, amount := range []float64{0, -50} {
		lines := validRTSLines(1)
		lines[0].AmountUSD = amount

		err := ValidateInvoicesForProvider(ProviderRTSSFTP, lines)
		require.Error(t, err, "amount %v must block", amount)
		var ve *BatchValidationError
		require.True(t, errors.As(err, &ve))
		assert.Equal(t, lines[0].InvoiceNumber, ve.InvoiceNumber)
		assert.Equal(t, "invoice amount", ve.Field)
	}
}

func TestValidateInvoices_TriumphHasNoRules(t *testing.T) {
	// The same batch that violates every RTS rule sails through for Triumph.
	lines := validRTSLines(2)
	lines[0].DebtorName = ""
	lines[0].AmountUSD = 0
	lines[1].InvoiceNumber = lines[0].InvoiceNumber

	assert.NoError(t, ValidateInvoicesForProvider(ProviderTriumphSFTP, lines))
}

func TestIsBatchValidationError_FalseForOtherErrors(t *testing.T) {
	assert.False(t, IsBatchValidationError(errors.New("boom")))
	assert.False(t, IsBatchValidationError(nil))
}
