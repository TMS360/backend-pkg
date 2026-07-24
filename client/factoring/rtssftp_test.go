package factoring

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRTSProvider(t *testing.T, fake *fakeUploader) *RTSSFTPProvider {
	t.Helper()
	p := NewRTSSFTP(Credential{
		ProviderType: ProviderRTSSFTP,
		Username:     "truckco1",
		Password:     "p",
	})
	p.dialFn = func(ctx context.Context, d sftpDialer) (sftpUploader, error) {
		return fake, nil
	}
	p.now = func() time.Time { return time.Date(2026, 1, 11, 14, 30, 52, 0, time.UTC) }
	return p
}

func rtsBatch() Batch {
	return Batch{
		BatchNumber: "IB-000001",
		SubmittedAt: time.Date(2026, 1, 11, 14, 30, 52, 0, time.UTC),
		Invoices: []InvoiceLine{
			{DebtorName: "ABC Logistics, LLC", InvoiceNumber: "IN-000000", InvoiceDate: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC), PONumber: "5555555", AmountUSD: 1000},
			{DebtorName: "XYZ Brokers", InvoiceNumber: "IN-000001", InvoiceDate: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC), PONumber: "6666666", AmountUSD: 500},
		},
		PDFs: []InvoicePDF{
			{InvoiceNumber: "IN-000000", Bytes: []byte("pdf1")},
			{InvoiceNumber: "IN-000001", Bytes: []byte("pdf2")},
		},
	}
}

func TestRTSSubmitBatch_UploadOrder_PDFsBeforeCSV(t *testing.T) {
	fake := &fakeUploader{}
	p := newRTSProvider(t, fake)

	res, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
	require.NoError(t, err)

	require.Len(t, fake.uploads, 3, "expected 2 PDFs + 1 CSV")
	assert.Equal(t, "IN-000000.pdf", fake.uploads[0].filename)
	assert.Equal(t, "IN-000001.pdf", fake.uploads[1].filename)
	// The CSV lands LAST and in two trigger-safe steps: uploaded under an
	// inert temp name, then renamed into the live spreadsheet name — RTS grabs
	// and clears the folder the moment a *.csv appears, so a truncated direct
	// write would fire the trigger on a partial manifest.
	assert.Equal(t, "invoices_20260111_143052.csv.uploading", fake.uploads[2].filename)
	require.Len(t, fake.renames, 1)
	assert.Equal(t, recordedRename{dir: "", from: "invoices_20260111_143052.csv.uploading", to: "invoices_20260111_143052.csv"}, fake.renames[0])
	assert.Equal(t, "invoices_20260111_143052.csv", res.CSVFileName)
	assert.True(t, fake.closed, "client should be closed on success")
}

// A failed rename (temp manifest never activated) is an error — the batch is
// not silently considered shipped while the trigger never fired.
func TestRTSSubmitBatch_RenameFailureIsError(t *testing.T) {
	fake := &fakeUploader{renameErr: errors.New("permission denied")}
	p := newRTSProvider(t, fake)

	_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "activate csv manifest")
}

func TestRTSSubmitBatch_UploadsToRootDir(t *testing.T) {
	fake := &fakeUploader{}
	p := newRTSProvider(t, fake)

	_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
	require.NoError(t, err)
	// RTS drops land in the chrooted home dir — no inbound subfolder.
	require.NotEmpty(t, fake.uploads)
	for _, up := range fake.uploads {
		assert.Equal(t, "", up.dir)
	}
	require.Len(t, fake.dirs, 1)
	assert.Equal(t, "", fake.dirs[0])
}

func TestRTSSubmitBatch_CSVIsRTSManifest(t *testing.T) {
	fake := &fakeUploader{}
	p := newRTSProvider(t, fake)

	_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
	require.NoError(t, err)

	csvContent := string(fake.uploads[2].content)
	lines := strings.Split(csvContent, "\r\n")
	assert.Equal(t, "Client,Invoice#,DebtorNo,Debtor Name,Load #,InvDate,InvAmt", lines[0])
	// Client Number column = the SFTP username of the credential.
	assert.True(t, strings.HasPrefix(lines[1], "truckco1,IN-000000,"), "row: %q", lines[1])
}

func TestRTSSubmitBatch_ValidationBlocksBeforeDial(t *testing.T) {
	cases := map[string]func(b *Batch){
		"duplicate invoice number": func(b *Batch) {
			b.Invoices[1].InvoiceNumber = b.Invoices[0].InvoiceNumber
			b.PDFs[1].InvoiceNumber = b.PDFs[0].InvoiceNumber
		},
		"zero amount": func(b *Batch) {
			b.Invoices[0].AmountUSD = 0
		},
		"blank debtor name": func(b *Batch) {
			b.Invoices[0].DebtorName = "  "
		},
		"blank po number": func(b *Batch) {
			b.Invoices[0].PONumber = ""
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dialed := false
			p := newRTSProvider(t, &fakeUploader{})
			p.dialFn = func(ctx context.Context, d sftpDialer) (sftpUploader, error) {
				dialed = true
				return &fakeUploader{}, nil
			}

			b := rtsBatch()
			mutate(&b)

			_, err := p.SubmitBatch(context.Background(), b, nil)
			require.Error(t, err)
			assert.True(t, IsBatchValidationError(err), "want BatchValidationError, got %v", err)
			assert.False(t, dialed, "validation must block before dialing SFTP")
		})
	}
}

func TestRTSSubmitBatch_RecordCapBlocksBeforeDial(t *testing.T) {
	dialed := false
	p := newRTSProvider(t, &fakeUploader{})
	p.dialFn = func(ctx context.Context, d sftpDialer) (sftpUploader, error) {
		dialed = true
		return &fakeUploader{}, nil
	}

	b := Batch{SubmittedAt: time.Now()}
	for _, line := range validRTSLines(1000) {
		b.Invoices = append(b.Invoices, line)
		b.PDFs = append(b.PDFs, InvoicePDF{InvoiceNumber: line.InvoiceNumber, Bytes: []byte("x")})
	}

	_, err := p.SubmitBatch(context.Background(), b, nil)
	require.Error(t, err)
	assert.True(t, IsBatchValidationError(err))
	assert.False(t, dialed)
}

func TestRTSSubmitBatch_PDFFailureNeverUploadsCSV(t *testing.T) {
	fake := &fakeUploader{uploadErr: errors.New("disk full")}
	p := newRTSProvider(t, fake)

	_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
	require.Error(t, err)
	// The upload error fired on the first PDF; nothing was recorded, so the
	// CSV trigger never dropped — RTS never sees a half-uploaded batch.
	assert.Empty(t, fake.uploads)
}

func TestRTSSubmitBatch_ReportsPerFileProgress(t *testing.T) {
	fake := &fakeUploader{}
	p := newRTSProvider(t, fake)

	var ticks []Progress
	res, err := p.SubmitBatch(context.Background(), rtsBatch(), func(pr Progress) { ticks = append(ticks, pr) })
	require.NoError(t, err)

	require.Len(t, ticks, 3)
	for i, tk := range ticks {
		assert.Equal(t, "uploading", tk.Phase)
		assert.Equal(t, 3, tk.Total)
		assert.Equal(t, i+1, tk.Done)
	}
	assert.Equal(t, res.CSVFileName, ticks[2].Detail)
}

func TestNewProviderFromCredential_ReturnsRTSForKnownType(t *testing.T) {
	prov, err := NewProviderFromCredential(Credential{
		ProviderType: ProviderRTSSFTP,
		Username:     "u",
		Password:     "p",
	})
	require.NoError(t, err)
	_, ok := prov.(*RTSSFTPProvider)
	assert.True(t, ok, "expected *RTSSFTPProvider")
}

func TestBuildManifest_PerProviderFormats(t *testing.T) {
	lines := rtsBatch().Invoices

	triumph := NewTriumphSFTP(Credential{ProviderType: ProviderTriumphSFTP, Username: "u", Password: "p"})
	tb, err := triumph.BuildManifest(lines)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(tb), "DTR_NAME,"), "triumph manifest keeps the 5-column layout")

	rts := NewRTSSFTP(Credential{ProviderType: ProviderRTSSFTP, Username: "truckco1", Password: "p"})
	rb, err := rts.BuildManifest(lines)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(rb), "Client,"), "rts manifest uses the 7-column layout")
	assert.Contains(t, string(rb), "truckco1,")
}
