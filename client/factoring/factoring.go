// Package factoring is the abstraction for submitting invoice batches to
// freight factoring companies (Triumph, RTS, Apex, etc.).
//
// Carriers sell receivables (invoices) to a factoring company; the factor
// expects a "batch" — typically a CSV manifest plus one PDF per invoice —
// delivered through whatever channel the factor supports. Triumph uses SFTP
// drop, others use APIs or FTPS. This package defines a single Provider
// interface that hides the transport so callers (backend-accounting) write the
// submission flow once.
//
// v1 ships one implementation: TriumphSFTPProvider. Add more (TriumphAPI,
// RTSSFTP, EcapitalAPI...) by writing a new file and registering it in
// registry.go.
//
// Credentials are per-company and stored in tms360-backend's `settings` table
// under one universal key — `factoring_credentials` — mirrored to Redis at
// {company_id}:setting:factoring_credentials. The same JSON shape (Credential)
// is used for every provider; `provider_type` inside the JSON picks the
// concrete implementation. Use provider.JSONClientProvider to fetch and build
// a Provider for the calling company.
package factoring

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

// ProviderType identifies which factoring backend a Batch should be routed to.
// Stored on the FactoringSubmission row in backend-accounting and inside the
// per-company Credential JSON. Add a new constant whenever a new
// provider implementation lands; keep AllProviderTypes / IsValid in sync.
type ProviderType string

const (
	ProviderTriumphSFTP ProviderType = "triumph_sftp"
)

// AllProviderTypes is the canonical list of supported provider types — used by
// tms360-backend's credential validator and by gqlgen for the GraphQL enum.
var AllProviderTypes = []ProviderType{
	ProviderTriumphSFTP,
}

// IsValid reports whether p is a known ProviderType. Useful for validating
// incoming JSON before persisting credentials.
func (p ProviderType) IsValid() bool {
	for _, x := range AllProviderTypes {
		if p == x {
			return true
		}
	}
	return false
}

// String returns the wire form ("triumph_sftp"); satisfies fmt.Stringer.
func (p ProviderType) String() string { return string(p) }

// MarshalGQL renders the enum on the GraphQL wire. gqlgen calls this when
// resolving FactoringProviderType in the schema.
func (p ProviderType) MarshalGQL(w io.Writer) {
	_, _ = io.WriteString(w, strconv.Quote(string(p)))
}

// UnmarshalGQL parses the enum from the GraphQL wire and validates it against
// AllProviderTypes — rejecting unknown values at the schema boundary instead
// of failing later inside the registry.
func (p *ProviderType) UnmarshalGQL(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("factoring: ProviderType must be a string, got %T", v)
	}
	pt := ProviderType(s)
	if !pt.IsValid() {
		return fmt.Errorf("factoring: unknown ProviderType %q", s)
	}
	*p = pt
	return nil
}

// Credential is the single JSON shape used for every factoring provider. The
// universal UI form (Factoring Company / Username / Access Key / Password /
// Billing Address / Remit Notice) maps 1:1 to these fields plus `provider_type`
// (the dropdown selection). Each concrete Provider uses only the fields it
// needs — Triumph SFTP reads Username + Password; an API provider would read
// AccessKey; metadata fields (BillingAddress / RemitNotice) are exposed for
// future invoice rendering but transport implementations may ignore them.
//
// Transport-specific config (SFTP host/port, inbound directory, API base URL)
// is NOT here — it's hardcoded as constants inside each Provider impl.
type Credential struct {
	ProviderType         ProviderType `json:"provider_type"`
	FactoringCompanyName string       `json:"factoring_company_name,omitempty"`
	Username             string       `json:"username"`
	AccessKey            string       `json:"access_key,omitempty"`
	Password             string       `json:"password"`
	BillingAddress       string       `json:"billing_address,omitempty"`
	RemitNotice          string       `json:"remit_notice,omitempty"`
}


// Provider sends a Batch to one factoring company. Implementations are
// stateless except for connection bookkeeping inside SubmitBatch.
type Provider interface {
	// SubmitBatch ships every PDF and the CSV manifest in a single call. The
	// implementation MUST upload supporting files first and the manifest last
	// — many factors (Triumph in particular) poll the inbound folder and will
	// pick up a CSV the moment it appears, so a half-uploaded batch fails.
	SubmitBatch(ctx context.Context, batch Batch) (SubmitResult, error)
}

// Batch is the unit of submission: one InvoiceBatch from backend-accounting
// flattened to a wire-ready shape. Both slices line up by invoice — every
// InvoiceLine[i] has its rendered PDF in PDFs[i].
type Batch struct {
	BatchNumber string        // backend-accounting InvoiceBatch.Number, used in logs only
	SubmittedAt time.Time     // when the upload starts; drives CSV filename timestamp
	Invoices    []InvoiceLine // one row per invoice in the batch
	PDFs        []InvoicePDF  // one PDF per invoice; order matches Invoices
}

// InvoiceLine is a single row of the factor's CSV manifest. Fields are
// transport-neutral; each provider knows how to render them (Triumph: 5
// columns DTR_NAME / INVOICE# / INV_DATE / PO / INVAMT).
type InvoiceLine struct {
	DebtorName    string    // Triumph "DTR_NAME" — customer/broker being billed
	InvoiceNumber string    // "INV-2026-00042"
	InvoiceDate   time.Time // SentAt or CreatedAt, formatted per provider
	PONumber      string    // shipment reference number
	AmountUSD     float64   // invoice total, 2 decimal places, no currency symbol
}

// InvoicePDF is the rendered invoice document. InvoiceNumber MUST match the
// matching InvoiceLine so the file is named consistently in the drop.
type InvoicePDF struct {
	InvoiceNumber string // basename for the uploaded PDF (e.g. INV-2026-00042 → INV-2026-00042.pdf)
	Bytes         []byte
}

// SubmitResult reports what was actually uploaded. Stored alongside the
// FactoringSubmission row for audit.
type SubmitResult struct {
	CSVFileName string   // e.g. "invoices_20260111_143052.csv"
	Uploaded    []string // remote paths in the order they were uploaded
}

// AuthError is returned when a provider rejects credentials (SFTP login
// failure, 401/403 on HTTP, etc.). backend-accounting checks IsAuthError to
// trigger DeactivateIntegration via the same Kafka topic used by Samsara/Relay.
type AuthError struct {
	ProviderType ProviderType
	Cause        error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("factoring: %s authentication failed: %v", e.ProviderType, e.Cause)
}

func (e *AuthError) Unwrap() error { return e.Cause }

// IsAuthError reports whether err (or any error it wraps) is a *AuthError.
func IsAuthError(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// validate is shared sanity-checking used by every provider before opening a
// connection. Failing here is a programmer error in the caller, not a remote
// rejection, so callers should treat it as a 4xx-equivalent.
func (b Batch) validate() error {
	if len(b.Invoices) == 0 {
		return errors.New("factoring: batch has no invoices")
	}
	if len(b.Invoices) != len(b.PDFs) {
		return fmt.Errorf("factoring: invoice count %d does not match PDF count %d",
			len(b.Invoices), len(b.PDFs))
	}
	for i, inv := range b.Invoices {
		if inv.InvoiceNumber == "" {
			return fmt.Errorf("factoring: invoice[%d] missing invoice number", i)
		}
		if b.PDFs[i].InvoiceNumber != inv.InvoiceNumber {
			return fmt.Errorf("factoring: invoice[%d] number %q does not match PDF number %q",
				i, inv.InvoiceNumber, b.PDFs[i].InvoiceNumber)
		}
		if len(b.PDFs[i].Bytes) == 0 {
			return fmt.Errorf("factoring: invoice[%d] (%s) has empty PDF", i, inv.InvoiceNumber)
		}
	}
	return nil
}
