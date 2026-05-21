package factoring

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Triumph SFTP transport constants. The per-carrier inbound directory is
// `.` because Triumph drops every carrier into a chrooted home — uploads at
// the root of the SSH user's home folder are picked up by their poller.
// Override is intentionally not supported in v1; if Triumph rotates the host
// or assigns a per-carrier subdirectory, change these constants in a single
// place.
const (
	triumphSFTPHost       = "files.triumphbcap.com"
	triumphSFTPPort       = 22
	triumphSFTPInboundDir = "."
)

// sftpUploader is the subset of *sftpClient that TriumphSFTPProvider needs.
// Exists as an interface so tests can inject a fake without spinning up a real
// SSH server. Production path uses the dialSFTP adapter below.
type sftpUploader interface {
	EnsureDir(remoteDir string) error
	Upload(remoteDir, filename string, content []byte) (string, error)
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
	username string
	password string
	dialFn   func(ctx context.Context, d sftpDialer) (sftpUploader, error)
	now      func() time.Time
}

// NewTriumphSFTP builds a TriumphSFTPProvider for a single submission. Reuse
// across calls is safe (no internal state), but the connection is opened and
// closed inside each SubmitBatch.
func NewTriumphSFTP(cred Credential) *TriumphSFTPProvider {
	return &TriumphSFTPProvider{
		username: cred.Username,
		password: cred.Password,
		dialFn:   defaultSFTPDial,
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
func (p *TriumphSFTPProvider) SubmitBatch(ctx context.Context, batch Batch) (SubmitResult, error) {
	if err := batch.validate(); err != nil {
		return SubmitResult{}, err
	}

	csvBytes, err := BuildTriumphCSV(batch.Invoices)
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
		Host:     triumphSFTPHost,
		Port:     triumphSFTPPort,
		Username: p.username,
		Password: p.password,
	})
	if err != nil {
		return SubmitResult{}, err
	}
	defer client.Close()

	if err := client.EnsureDir(triumphSFTPInboundDir); err != nil {
		return SubmitResult{}, err
	}

	uploaded := make([]string, 0, len(batch.PDFs)+1)
	for i, pdf := range batch.PDFs {
		if err := ctx.Err(); err != nil {
			return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded}, err
		}
		fileName := sanitizePDFName(pdf.InvoiceNumber)
		remote, uerr := client.Upload(triumphSFTPInboundDir, fileName, pdf.Bytes)
		if uerr != nil {
			return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded},
				fmt.Errorf("upload pdf[%d] %s: %w", i, pdf.InvoiceNumber, uerr)
		}
		uploaded = append(uploaded, remote)
	}

	csvPath, err := client.Upload(triumphSFTPInboundDir, csvFileName, csvBytes)
	if err != nil {
		return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded},
			fmt.Errorf("upload csv manifest: %w", err)
	}
	uploaded = append(uploaded, csvPath)

	return SubmitResult{CSVFileName: csvFileName, Uploaded: uploaded}, nil
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
