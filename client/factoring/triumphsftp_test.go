package factoring

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUploader is an in-memory sftpUploader. It records every Upload call in
// the order it was made, so tests can assert ordering (PDFs first, CSV last).
type fakeUploader struct {
	mu        sync.Mutex
	dirs      []string
	uploads   []recordedUpload
	closed    bool
	uploadErr error
}

type recordedUpload struct {
	dir      string
	filename string
	content  []byte
}

func (f *fakeUploader) EnsureDir(dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirs = append(f.dirs, dir)
	return nil
}

func (f *fakeUploader) Upload(dir, filename string, content []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	f.uploads = append(f.uploads, recordedUpload{dir: dir, filename: filename, content: append([]byte(nil), content...)})
	return dir + "/" + filename, nil
}

func (f *fakeUploader) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func newProvider(t *testing.T, fake *fakeUploader) *TriumphSFTPProvider {
	t.Helper()
	p := NewTriumphSFTP(Credential{
		ProviderType: ProviderTriumphSFTP,
		Username:     "u",
		Password:     "p",
	})
	p.dialFn = func(ctx context.Context, d sftpDialer) (sftpUploader, error) {
		return fake, nil
	}
	p.now = func() time.Time { return time.Date(2026, 1, 11, 14, 30, 52, 0, time.UTC) }
	return p
}

func TestSubmitBatch_UploadOrder_PDFsBeforeCSV(t *testing.T) {
	fake := &fakeUploader{}
	p := newProvider(t, fake)

	res, err := p.SubmitBatch(context.Background(), Batch{
		BatchNumber: "INV-000001",
		SubmittedAt: time.Date(2026, 1, 11, 14, 30, 52, 0, time.UTC),
		Invoices: []InvoiceLine{
			{DebtorName: "ABC Logistics, LLC", InvoiceNumber: "IN-000000", InvoiceDate: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC), PONumber: "5555555", AmountUSD: 1000},
			{DebtorName: "XYZ Brokers",        InvoiceNumber: "IN-000001", InvoiceDate: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC), PONumber: "6666666", AmountUSD: 500},
		},
		PDFs: []InvoicePDF{
			{InvoiceNumber: "IN-000000", Bytes: []byte("pdf1")},
			{InvoiceNumber: "IN-000001", Bytes: []byte("pdf2")},
		},
	})
	require.NoError(t, err)

	require.Len(t, fake.uploads, 3, "expected 2 PDFs + 1 CSV")
	assert.Equal(t, "IN-000000.pdf", fake.uploads[0].filename)
	assert.Equal(t, "IN-000001.pdf", fake.uploads[1].filename)
	// CSV MUST be last so Triumph's poller never sees a manifest without files.
	assert.True(t, strings.HasSuffix(fake.uploads[2].filename, ".csv"),
		"last upload should be CSV, got %q", fake.uploads[2].filename)
	assert.Equal(t, "invoices_20260111_143052.csv", fake.uploads[2].filename)
	assert.Equal(t, "invoices_20260111_143052.csv", res.CSVFileName)
	assert.Len(t, res.Uploaded, 3)
	assert.True(t, fake.closed, "client should be closed on success")
}

func TestSubmitBatch_UsesHardcodedInboundDir(t *testing.T) {
	fake := &fakeUploader{}
	p := NewTriumphSFTP(Credential{
		ProviderType: ProviderTriumphSFTP,
		Username:     "u",
		Password:     "p",
	})
	p.dialFn = func(ctx context.Context, d sftpDialer) (sftpUploader, error) { return fake, nil }
	p.now = func() time.Time { return time.Now() }

	_, err := p.SubmitBatch(context.Background(), Batch{
		Invoices: []InvoiceLine{{DebtorName: "X", InvoiceNumber: "INV-1", InvoiceDate: time.Now(), PONumber: "p", AmountUSD: 1}},
		PDFs:     []InvoicePDF{{InvoiceNumber: "INV-1", Bytes: []byte("x")}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, fake.uploads)
	// Triumph drops to TMS_INPUT inside the SSH user's chrooted home.
	assert.Equal(t, "TMS_INPUT", fake.uploads[0].dir)
}

func TestSubmitBatch_SanitizesPDFFilename(t *testing.T) {
	fake := &fakeUploader{}
	p := newProvider(t, fake)

	_, err := p.SubmitBatch(context.Background(), Batch{
		SubmittedAt: time.Now(),
		Invoices:    []InvoiceLine{{DebtorName: "X", InvoiceNumber: "INV/1 #A", InvoiceDate: time.Now(), PONumber: "p", AmountUSD: 1}},
		PDFs:        []InvoicePDF{{InvoiceNumber: "INV/1 #A", Bytes: []byte("x")}},
	})
	require.NoError(t, err)
	assert.Equal(t, "INV_1__A.pdf", fake.uploads[0].filename)
}

func TestSubmitBatch_ValidatesMismatchedCounts(t *testing.T) {
	fake := &fakeUploader{}
	p := newProvider(t, fake)
	_, err := p.SubmitBatch(context.Background(), Batch{
		Invoices: []InvoiceLine{{DebtorName: "X", InvoiceNumber: "INV-1", InvoiceDate: time.Now(), AmountUSD: 1}},
		PDFs:     nil,
	})
	require.Error(t, err)
	assert.Empty(t, fake.uploads, "no uploads should happen on validation failure")
}

func TestSubmitBatch_ValidatesEmptyBatch(t *testing.T) {
	fake := &fakeUploader{}
	p := newProvider(t, fake)
	_, err := p.SubmitBatch(context.Background(), Batch{})
	require.Error(t, err)
}

func TestSubmitBatch_PropagatesUploadError(t *testing.T) {
	fake := &fakeUploader{uploadErr: errors.New("disk full")}
	p := newProvider(t, fake)
	_, err := p.SubmitBatch(context.Background(), Batch{
		SubmittedAt: time.Now(),
		Invoices:    []InvoiceLine{{DebtorName: "X", InvoiceNumber: "INV-1", InvoiceDate: time.Now(), AmountUSD: 1}},
		PDFs:        []InvoicePDF{{InvoiceNumber: "INV-1", Bytes: []byte("x")}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestNewProviderFromCredential_ReturnsTriumphForKnownType(t *testing.T) {
	prov, err := NewProviderFromCredential(Credential{
		ProviderType: ProviderTriumphSFTP,
		Username:     "u",
		Password:     "p",
	})
	require.NoError(t, err)
	_, ok := prov.(*TriumphSFTPProvider)
	assert.True(t, ok, "expected *TriumphSFTPProvider")
}

func TestNewProviderFromCredential_RejectsUnknownType(t *testing.T) {
	_, err := NewProviderFromCredential(Credential{
		ProviderType: ProviderType("apex_api"),
		Username:     "u",
		Password:     "p",
	})
	require.Error(t, err)
}

func TestNewProviderFromCredential_RejectsMissingPassword(t *testing.T) {
	_, err := NewProviderFromCredential(Credential{
		ProviderType: ProviderTriumphSFTP,
		Username:     "u",
	})
	require.Error(t, err)
}

func TestNewProviderFromJSON_HappyPath(t *testing.T) {
	cred := []byte(`{"provider_type":"triumph_sftp","username":"u","password":"p"}`)
	prov, err := NewProviderFromJSON(cred)
	require.NoError(t, err)
	_, ok := prov.(*TriumphSFTPProvider)
	assert.True(t, ok)
}

func TestNewProviderFromJSON_RejectsEmpty(t *testing.T) {
	_, err := NewProviderFromJSON(nil)
	require.Error(t, err)
}

func TestProviderType_MarshalGQL(t *testing.T) {
	var buf strings.Builder
	ProviderTriumphSFTP.MarshalGQL(&buf)
	assert.Equal(t, `"triumph_sftp"`, buf.String())
}

func TestProviderType_UnmarshalGQL(t *testing.T) {
	var p ProviderType
	require.NoError(t, p.UnmarshalGQL("triumph_sftp"))
	assert.Equal(t, ProviderTriumphSFTP, p)

	require.Error(t, p.UnmarshalGQL("apex_api"), "unknown type must reject")
	require.Error(t, p.UnmarshalGQL(123), "non-string must reject")
}

func TestSanitizePDFName(t *testing.T) {
	cases := map[string]string{
		"INV-2026-00042":   "INV-2026-00042.pdf",
		"INV/1":            "INV_1.pdf",
		"INV 1":            "INV_1.pdf",
		"INV#1":            "INV_1.pdf",
		"abc_123.xyz":      "abc_123.xyz.pdf",
		"":                 ".pdf",
	}
	for input, want := range cases {
		got := sanitizePDFName(input)
		assert.Equal(t, want, got, "input=%q", input)
	}
}

func TestIsAuthError(t *testing.T) {
	authErr := &AuthError{ProviderType: ProviderTriumphSFTP, Cause: errors.New("bad password")}
	wrapped := errors.New("outer: " + authErr.Error())
	assert.True(t, IsAuthError(authErr))
	assert.True(t, IsAuthError(authErr))
	assert.False(t, IsAuthError(wrapped))
}
