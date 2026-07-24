package factoring

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProviderRules captures per-factor submission constraints as data so callers
// (backend-accounting's Submit pre-checks and scheduler) never branch on a
// concrete provider type. A zero-value ProviderRules means "no constraints" —
// Triumph today.
type ProviderRules struct {
	// MaxRecords caps how many invoice rows one manifest may carry. 0 = no cap.
	// RTS rejects batches over 999 records.
	MaxRecords int
	// MinSubmissionGap is the minimum wall-clock spacing between two
	// submissions of the SAME company via this provider. 0 = none. RTS refuses
	// back-to-back drops closer than 5 minutes; the scheduler defers (never
	// fails) an early row until the window has passed.
	MinSubmissionGap time.Duration
	// RequireUniqueInvoiceNumbers rejects a batch referencing the same invoice
	// number twice.
	RequireUniqueInvoiceNumbers bool
	// RequirePositiveAmounts rejects rows with an amount of zero or less.
	RequirePositiveAmounts bool
	// RequireAllFields rejects rows with a blank (or whitespace-only)
	// DebtorName / PONumber / InvoiceNumber — RTS forbids empty cells.
	RequireAllFields bool
	// TriggerAndClear marks factors whose manifest upload TRIGGERS ingestion
	// and CLEARS the inbound folder (RTS). For them, re-uploading the same
	// deterministic filename after a successful drop is NOT an idempotent
	// overwrite — the folder is empty, so the re-ship lands as a brand-new
	// submission (duplicate funding). Callers must never auto-re-ship a row of
	// such a provider once the prior attempt may have reached the manifest
	// upload: quarantine for manual review instead (DEV-840 does not protect
	// these providers).
	TriggerAndClear bool
}

// RulesFor returns the submission constraints of a provider. Unknown types get
// the zero value (no constraints) — the registry rejects them earlier anyway.
func RulesFor(pt ProviderType) ProviderRules {
	switch pt {
	case ProviderRTSSFTP:
		return ProviderRules{
			MaxRecords:                  999,
			MinSubmissionGap:            5 * time.Minute,
			RequireUniqueInvoiceNumbers: true,
			RequirePositiveAmounts:      true,
			RequireAllFields:            true,
			TriggerAndClear:             true,
		}
	default:
		return ProviderRules{}
	}
}

// BatchValidationError is a structured provider-rule violation. It always
// names the offending invoice (or carries an empty InvoiceNumber for
// batch-level violations such as the record cap) so backend-accounting can
// surface an actionable message instead of a silent skip.
type BatchValidationError struct {
	ProviderType  ProviderType
	InvoiceNumber string // offending invoice; "" for batch-level violations
	Field         string // offending field for blank-field violations; "" otherwise
	Reason        string // human-readable explanation
}

func (e *BatchValidationError) Error() string {
	switch {
	case e.InvoiceNumber != "" && e.Field != "":
		return fmt.Sprintf("factoring: %s rejected invoice %s: %s (%s)",
			e.ProviderType, e.InvoiceNumber, e.Reason, e.Field)
	case e.InvoiceNumber != "":
		return fmt.Sprintf("factoring: %s rejected invoice %s: %s",
			e.ProviderType, e.InvoiceNumber, e.Reason)
	default:
		return fmt.Sprintf("factoring: %s rejected batch: %s", e.ProviderType, e.Reason)
	}
}

// IsBatchValidationError reports whether err (or any error it wraps) is a
// *BatchValidationError. Mirrors IsAuthError.
func IsBatchValidationError(err error) bool {
	var ve *BatchValidationError
	return errors.As(err, &ve)
}

// ValidateInvoicesForProvider applies RulesFor(pt) to the manifest lines and
// returns the FIRST violation as a *BatchValidationError (nil when the batch
// is compliant or the provider has no rules). Callers run it before uploading
// anything so the factor's CSV trigger never fires on a non-compliant batch.
func ValidateInvoicesForProvider(pt ProviderType, invoices []InvoiceLine) error {
	rules := RulesFor(pt)

	if rules.MaxRecords > 0 && len(invoices) > rules.MaxRecords {
		return &BatchValidationError{
			ProviderType: pt,
			Reason: fmt.Sprintf("batch has %d invoices; %s accepts at most %d per submission",
				len(invoices), pt, rules.MaxRecords),
		}
	}

	if rules.RequireUniqueInvoiceNumbers {
		seen := make(map[string]struct{}, len(invoices))
		for _, inv := range invoices {
			if _, dup := seen[inv.InvoiceNumber]; dup {
				return &BatchValidationError{
					ProviderType:  pt,
					InvoiceNumber: inv.InvoiceNumber,
					Reason:        "duplicate invoice number in batch",
				}
			}
			seen[inv.InvoiceNumber] = struct{}{}
		}
	}

	for _, inv := range invoices {
		if rules.RequireAllFields {
			switch {
			case strings.TrimSpace(inv.InvoiceNumber) == "":
				return &BatchValidationError{
					ProviderType: pt,
					Field:        "invoice number",
					Reason:       "blank invoice number",
				}
			case strings.TrimSpace(inv.DebtorName) == "":
				return &BatchValidationError{
					ProviderType:  pt,
					InvoiceNumber: inv.InvoiceNumber,
					Field:         "debtor name",
					Reason:        "blank debtor (customer) name",
				}
			case strings.TrimSpace(inv.PONumber) == "":
				return &BatchValidationError{
					ProviderType:  pt,
					InvoiceNumber: inv.InvoiceNumber,
					Field:         "po number",
					Reason:        "blank PO (load) number",
				}
			}
		}
		if rules.RequirePositiveAmounts && inv.AmountUSD <= 0 {
			return &BatchValidationError{
				ProviderType:  pt,
				InvoiceNumber: inv.InvoiceNumber,
				Field:         "invoice amount",
				Reason: fmt.Sprintf("invoice amount %.2f is not greater than zero",
					inv.AmountUSD),
			}
		}
	}

	return nil
}
