package factoring

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// RTS Financial SFTP transport constants. RTS chroots each client to their own
// home directory and their poller watches it directly — there is no inbound
// subfolder; files are dropped at the root of the session. The upload of a
// spreadsheet (.csv/.xlsx) is the trigger: RTS grabs the folder contents and
// clears it, so the manifest MUST land last.
//
// NewRTSSFTP swaps these defaults for TEST_RTS_SFTP_* env vars when APP_ENV is
// dev / stage / local (same allowlist as Triumph, separate variables so a
// stage box can point the two providers at different sftpgo users).
const (
	rtsSFTPHost       = "ftps.financial.rtspro.com"
	rtsSFTPPort       = 22
	rtsSFTPInboundDir = "" // chrooted home dir; EnsureDir("") is a documented no-op

	envTestRTSSFTPHost       = "TEST_RTS_SFTP_HOST"
	envTestRTSSFTPPort       = "TEST_RTS_SFTP_PORT"
	envTestRTSSFTPInboundDir = "TEST_RTS_SFTP_INBOUND_DIR"
)

// RTSSFTPProvider implements Provider for RTS Financial's FTPS/SFTP invoice
// upload (we use the SFTP flavour, port 22). The drop is one-way: PDFs first
// (one per invoice, named <invoice number>.pdf), then the 7-column CSV
// manifest last. RTS enforces batch rules server-side (no duplicates, ≤999
// records, no blanks, no non-positive amounts) — SubmitBatch pre-validates
// via ValidateInvoicesForProvider so the trigger never fires on a batch RTS
// would reject.
type RTSSFTPProvider struct {
	username     string
	password     string
	host         string
	port         int
	inboundDir   string
	providerType ProviderType
	dialFn       func(ctx context.Context, d sftpDialer) (sftpUploader, error)
	now          func() time.Time
}

// NewRTSSFTP builds an RTSSFTPProvider for a single submission. Reuse across
// calls is safe (no internal state); the connection is opened and closed
// inside each SubmitBatch. Reads only Username + Password from the universal
// Credential — the username doubles as the RTS "Client Number" (first CSV
// column).
func NewRTSSFTP(cred Credential) *RTSSFTPProvider {
	host := rtsSFTPHost
	port := rtsSFTPPort
	inboundDir := rtsSFTPInboundDir

	if isNonProdAppEnv() {
		if h := strings.TrimSpace(os.Getenv(envTestRTSSFTPHost)); h != "" {
			host = h
		}
		if raw := strings.TrimSpace(os.Getenv(envTestRTSSFTPPort)); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				port = n
			}
		}
		if d := strings.TrimSpace(os.Getenv(envTestRTSSFTPInboundDir)); d != "" {
			inboundDir = d
		}
	}

	return &RTSSFTPProvider{
		username:     cred.Username,
		password:     cred.Password,
		host:         host,
		port:         port,
		inboundDir:   inboundDir,
		providerType: ProviderRTSSFTP,
		dialFn:       defaultSFTPDial,
	}
}

// BuildManifest renders the RTS 7-column CSV — the same bytes SubmitBatch
// ships. Client Number (column 1) is the SFTP username from the credential.
func (p *RTSSFTPProvider) BuildManifest(invoices []InvoiceLine) ([]byte, error) {
	return BuildRTSCSV(p.username, invoices)
}

// SubmitBatch uploads every invoice PDF first, then the CSV manifest last.
// Order is load-bearing for RTS: the spreadsheet upload triggers the transfer
// and the folder is cleared, so any file landing after the CSV is lost.
// Provider rules are validated BEFORE dialing — a non-compliant batch never
// reaches RTS at all (structured *BatchValidationError names the offender).
//
// File naming:
//   - PDFs: <INVOICE#>.pdf (InvoiceNumber sanitized for filesystem safety)
//   - CSV:  invoices_YYYYMMDD_HHMMSS.csv (UTC timestamp from batch.SubmittedAt
//     — deterministic, so a reclaimed retry overwrites the same file instead
//     of producing a second manifest; DEV-840)
func (p *RTSSFTPProvider) SubmitBatch(ctx context.Context, batch Batch, onProgress ProgressFunc) (SubmitResult, error) {
	if err := batch.validate(); err != nil {
		return SubmitResult{}, err
	}
	if err := ValidateInvoicesForProvider(p.providerType, batch.Invoices); err != nil {
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

	// Trigger-safe manifest drop: RTS ingests on ANY *.csv/*.xlsx the moment it
	// appears, so a connection drop mid-write must never leave a truncated CSV
	// behind — a partial manifest would fire the trigger with an incomplete
	// batch. Upload under an inert temp name (not a spreadsheet extension),
	// then atomically rename into place; the trigger only ever sees a complete
	// manifest. A leftover *.uploading from a crash is ignored by RTS.
	tmpName := csvFileName + ".uploading"
	if _, err := client.Upload(p.inboundDir, tmpName, csvBytes); err != nil {
		return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded},
			fmt.Errorf("upload csv manifest (temp): %w", err)
	}
	if err := client.Rename(p.inboundDir, tmpName, csvFileName); err != nil {
		return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded},
			fmt.Errorf("activate csv manifest: %w", err)
	}
	uploaded = append(uploaded, path.Join(p.inboundDir, csvFileName))
	if onProgress != nil {
		onProgress(Progress{Phase: "uploading", Done: total, Total: total, Detail: csvFileName})
	}

	return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded}, nil
}

func (p *RTSSFTPProvider) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}
