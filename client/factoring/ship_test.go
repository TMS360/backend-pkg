package factoring

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-3299: a factor gets a batch once and whole. These run through both SFTP
// providers because both ship via shipBatch.

const shipCSV = "invoices_20260111_143052.csv"

func submitBoth(t *testing.T, setup func(*fakeUploader)) map[string]struct {
	fake *fakeUploader
	res  SubmitResult
	err  error
} {
	t.Helper()
	out := map[string]struct {
		fake *fakeUploader
		res  SubmitResult
		err  error
	}{}
	for _, name := range []string{"triumph", "rts"} {
		fake := &fakeUploader{}
		setup(fake)
		var prov Provider = newProvider(t, fake)
		if name == "rts" {
			prov = newRTSProvider(t, fake)
		}
		res, err := prov.SubmitBatch(context.Background(), rtsBatch(), nil)
		out[name] = struct {
			fake *fakeUploader
			res  SubmitResult
			err  error
		}{fake, res, err}
	}
	return out
}

func TestShipBatch_PDFFailsMidway_LeavesNothing(t *testing.T) {
	for name, r := range submitBoth(t, func(f *fakeUploader) { f.failUploadOf = "IN-000001.pdf" }) {
		require.Error(t, r.err, name)
		assert.NotErrorIs(t, r.err, ErrFilesMayRemain, name)
		assert.Empty(t, r.fake.remoteFiles(), "%s: no file of the attempt may stay on the server", name)
		assert.Empty(t, r.fake.renames, "%s: manifest must never be activated", name)
	}
}

func TestShipBatch_ManifestUploadFails_LeavesNothing(t *testing.T) {
	for name, r := range submitBoth(t, func(f *fakeUploader) { f.failUploadOf = shipCSV + manifestTempSuffix }) {
		require.Error(t, r.err, name)
		assert.Empty(t, r.fake.remoteFiles(), "%s: PDFs and the half-written temp CSV are removed", name)
	}
}

func TestShipBatch_RenameErrorButManifestLanded_IsSent(t *testing.T) {
	for name, r := range submitBoth(t, func(f *fakeUploader) {
		f.renameErr = errors.New("connection lost")
		f.renameErrAfterMove = true
	}) {
		require.NoError(t, r.err, "%s: a whole CSV on the server means the batch is sent", name)
		assert.Equal(t, shipCSV, r.res.CSVFileName, name)
		assert.Contains(t, r.fake.remoteFiles(), r.res.Uploaded[len(r.res.Uploaded)-1], name)
		assert.Empty(t, r.fake.removed, "%s: nothing is removed once the CSV landed", name)
	}
}

func TestShipBatch_RenameErrorManifestCollected_IsSent(t *testing.T) {
	// RTS grabs and clears the folder the moment the CSV appears: final and temp
	// are both gone, which can only mean the rename went through.
	fake := &fakeUploader{renameErr: errors.New("connection lost"), renameErrAfterMove: true}
	p := newRTSProvider(t, fake)
	p.dialFn = func(ctx context.Context, d sftpDialer) (sftpUploader, error) {
		return &collectingUploader{fakeUploader: fake}, nil
	}
	_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
	require.NoError(t, err)
	assert.Empty(t, fake.removed)
}

func TestShipBatch_RenameErrorManifestAbsent_CleansUp(t *testing.T) {
	for name, r := range submitBoth(t, func(f *fakeUploader) { f.renameErr = errors.New("permission denied") }) {
		require.Error(t, r.err, name)
		assert.Contains(t, r.err.Error(), "activate csv manifest", name)
		assert.Empty(t, r.fake.remoteFiles(), "%s: temp CSV and PDFs are removed", name)
	}
}

func TestShipBatch_CleanupFails_SaysFilesMayRemain(t *testing.T) {
	for name, r := range submitBoth(t, func(f *fakeUploader) {
		f.failUploadOf = "IN-000001.pdf"
		f.removeErr = errors.New("host unreachable")
	}) {
		require.ErrorIs(t, r.err, ErrFilesMayRemain, name)
		assert.Contains(t, r.err.Error(), "check the folder", name)
	}
}

func TestShipBatch_ManifestStateUnreadable_RemovesNothing(t *testing.T) {
	for name, r := range submitBoth(t, func(f *fakeUploader) {
		f.renameErr = errors.New("connection lost")
		f.sizeErr = errors.New("host unreachable")
	}) {
		require.ErrorIs(t, r.err, ErrFilesMayRemain, name)
		assert.Empty(t, r.fake.removed, "%s: the CSV may be live, so its PDFs must stay", name)
	}
}

// collectingUploader plays a trigger-and-clear factor: a manifest renamed into
// place is collected (removed with the PDFs) at once.
type collectingUploader struct{ *fakeUploader }

func (c *collectingUploader) Rename(dir, from, to string) error {
	err := c.fakeUploader.Rename(dir, from, to)
	c.fakeUploader.mu.Lock()
	c.fakeUploader.files = map[string]int64{}
	c.fakeUploader.mu.Unlock()
	return err
}
