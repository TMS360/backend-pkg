package factoring

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Triumph SFTP transport constants. The per-carrier inbound directory is
// `TMS_INPUT` — Triumph chroots each carrier to their own home and expects
// the manifest + PDFs in the TMS_INPUT subfolder (their poller scans only
// that path).
//
// NewTriumphSFTP transparently swaps these defaults for TEST_TRIUMPH_SFTP_*
// env vars (legacy TEST_SFTP_* still honored)
// when APP_ENV is dev / stage / local, so a self-hosted sftpgo instance can
// stand in for Triumph end-to-end without any UI or credential changes. The
// allowlist (NOT a deny-list against "prod") means an unset APP_ENV in
// production still keeps the real Triumph host — fail-safe by default.
const (
	triumphSFTPHost       = "files.triumphbcap.com"
	triumphSFTPPort       = 22
	triumphSFTPInboundDir = "TMS_INPUT"

	envTestTriumphSFTPHost       = "TEST_TRIUMPH_SFTP_HOST"
	envTestTriumphSFTPPort       = "TEST_TRIUMPH_SFTP_PORT"
	envTestTriumphSFTPInboundDir = "TEST_TRIUMPH_SFTP_INBOUND_DIR"

	// Legacy names from the single-provider era ("SFTP" meant Triumph back
	// then). Still honored as a fallback so existing dev/stage environments
	// keep working; prefer the TEST_TRIUMPH_SFTP_* names above.
	envTestSFTPHostLegacy       = "TEST_SFTP_HOST"
	envTestSFTPPortLegacy       = "TEST_SFTP_PORT"
	envTestSFTPInboundDirLegacy = "TEST_SFTP_INBOUND_DIR"
)

// sftpUploader is the subset of *sftpClient the SFTP providers need. Exists as
// an interface so tests can inject a fake without spinning up a real SSH
// server. Production path uses the dialSFTP adapter below.
type sftpUploader interface {
	EnsureDir(remoteDir string) error
	Upload(remoteDir, filename string, content []byte) (string, error)
	// Rename moves remoteDir/from to remoteDir/to (replacing the target) —
	// the second half of a trigger-safe two-step manifest drop.
	Rename(remoteDir, from, to string) error
	Close() error
}

// TriumphSFTPProvider implements Provider for Triumph Business Capital's SFTP
// drop. The drop is one-way: Triumph polls the inbound folder every ~5
// minutes, picks up the manifest + PDFs together, and reports status via
// MyTriumph reports — there is no ACK file by protocol.
//
// The provider takes the universal Credential (the same shape used by every
// future factor) and pulls only the fields it needs (Username + Password).
// Host/Port/InboundDirectory are package constants — they are not part of
// the per-company credential because every Triumph SFTP customer hits the
// same endpoint.
type TriumphSFTPProvider struct {
	username     string
	password     string
	host         string
	port         int
	inboundDir   string
	providerType ProviderType
	dialFn       func(ctx context.Context, d sftpDialer) (sftpUploader, error)
	now          func() time.Time
}

// NewTriumphSFTP builds a TriumphSFTPProvider for a single submission. Reuse
// across calls is safe (no internal state), but the connection is opened and
// closed inside each SubmitBatch.
//
// In dev / stage / local environments (APP_ENV allowlist) the transport host
// is swapped for TEST_TRIUMPH_SFTP_HOST / TEST_TRIUMPH_SFTP_PORT /
// TEST_TRIUMPH_SFTP_INBOUND_DIR (legacy TEST_SFTP_* honored as fallback) if
// any of those env vars are set — so the GraphQL surface (one provider type
// "triumph_sftp", one Settings form) stays identical across environments
// while a self-hosted sftpgo instance can stand in for Triumph. In
// production (or when APP_ENV is unset) the env vars are ignored and the
// real Triumph endpoint is always used.
func NewTriumphSFTP(cred Credential) *TriumphSFTPProvider {
	host := triumphSFTPHost
	port := triumphSFTPPort
	inboundDir := triumphSFTPInboundDir

	if isNonProdAppEnv() {
		if h := firstNonEmptyEnv(envTestTriumphSFTPHost, envTestSFTPHostLegacy); h != "" {
			host = h
		}
		if raw := firstNonEmptyEnv(envTestTriumphSFTPPort, envTestSFTPPortLegacy); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				port = n
			}
		}
		if d := firstNonEmptyEnv(envTestTriumphSFTPInboundDir, envTestSFTPInboundDirLegacy); d != "" {
			inboundDir = d
		}
	}

	return &TriumphSFTPProvider{
		username:     cred.Username,
		password:     cred.Password,
		host:         host,
		port:         port,
		inboundDir:   inboundDir,
		providerType: ProviderTriumphSFTP,
		dialFn:       defaultSFTPDial,
	}
}

// firstNonEmptyEnv returns the first of the named env vars whose trimmed
// value is non-empty. Used for the canonical-name-with-legacy-fallback test
// override lookups.
func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// isNonProdAppEnv reports whether the current deployment may honour the
// TEST_*_SFTP_* overrides. Implemented as an allowlist (NOT a deny-list
// against "prod") so an empty / typo'd APP_ENV is treated as production —
// overrides are ignored and the real factor endpoint is used. Match is
// case-insensitive.
func isNonProdAppEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "dev", "stage", "staging", "local":
		return true
	default:
		return false
	}
}

// defaultSFTPDial is the production sftp dialer; wrapped to satisfy the
// uploader interface return type.
func defaultSFTPDial(ctx context.Context, d sftpDialer) (sftpUploader, error) {
	return dialSFTP(ctx, d)
}

// SubmitBatch uploads every invoice PDF first, then the CSV manifest last.
// Order matters: Triumph's poller starts processing the moment it sees the
// CSV, so any PDF uploaded after the CSV is missed.
//
// File naming:
//   - PDFs: <INVOICE#>.pdf (InvoiceNumber sanitized for filesystem safety)
//   - CSV:  invoices_YYYYMMDD_HHMMSS.csv (UTC timestamp from batch.SubmittedAt)
func (p *TriumphSFTPProvider) SubmitBatch(ctx context.Context, batch Batch, onProgress ProgressFunc) (SubmitResult, error) {
	if err := batch.validate(); err != nil {
		return SubmitResult{}, err
	}

	csvBytes, err := p.BuildManifest(batch.Invoices)
	if err != nil {
		return SubmitResult{}, err
	}

	timestamp := batch.SubmittedAt
	if timestamp.IsZero() {
		timestamp = p.clock()
	}
	csvFileName := fmt.Sprintf("invoices_%s.csv", timestamp.UTC().Format("20060102_150405"))

	dial := p.dialFn
	if dial == nil {
		dial = defaultSFTPDial
	}
	client, err := dial(ctx, sftpDialer{
		Host:         p.host,
		Port:         p.port,
		Username:     p.username,
		Password:     p.password,
		ProviderType: p.providerType,
	})
	if err != nil {
		return SubmitResult{}, err
	}
	defer client.Close()

	if err := client.EnsureDir(p.inboundDir); err != nil {
		return SubmitResult{}, err
	}

	// total = every PDF plus the CSV manifest (uploaded last).
	total := len(batch.PDFs) + 1
	uploaded := make([]string, 0, total)
	for i, pdf := range batch.PDFs {
		if err := ctx.Err(); err != nil {
			return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded}, err
		}
		fileName := sanitizePDFName(pdf.InvoiceNumber)
		remote, uerr := client.Upload(p.inboundDir, fileName, pdf.Bytes)
		if uerr != nil {
			return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded},
				fmt.Errorf("upload pdf[%d] %s: %w", i, pdf.InvoiceNumber, uerr)
		}
		uploaded = append(uploaded, remote)
		if onProgress != nil {
			onProgress(Progress{Phase: "uploading", Done: i + 1, Total: total, Detail: fileName})
		}
	}

	csvPath, err := client.Upload(p.inboundDir, csvFileName, csvBytes)
	if err != nil {
		return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded},
			fmt.Errorf("upload csv manifest: %w", err)
	}
	uploaded = append(uploaded, csvPath)
	if onProgress != nil {
		onProgress(Progress{Phase: "uploading", Done: total, Total: total, Detail: csvFileName})
	}

	return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded}, nil
}

// BuildManifest renders the Triumph 5-column CSV — the same bytes SubmitBatch
// ships. Exposed via the Provider interface so backend-accounting's archive
// paths (S3 copy, batch ZIP) emit the active provider's format.
func (p *TriumphSFTPProvider) BuildManifest(invoices []InvoiceLine) ([]byte, error) {
	return BuildTriumphCSV(invoices)
}

func (p *TriumphSFTPProvider) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// sanitizePDFName produces a safe filename from an invoice number. SFTP itself
// accepts most characters, but factoring poll scripts on the other end often
// choke on spaces or `#`. Strategy: keep [A-Za-z0-9._-], replace everything
// else with `_`. Always ends in ".pdf".
func sanitizePDFName(invoiceNumber string) string {
	var b strings.Builder
	b.Grow(len(invoiceNumber) + 4)
	for _, r := range invoiceNumber {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	b.WriteString(".pdf")
	return b.String()
}
