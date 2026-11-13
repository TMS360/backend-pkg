package factoring

import (
	"context"
	"errors"
	"fmt"
	"path"
)

// ErrFilesMayRemain marks a failed send whose own uploads could not be
// cleaned up (or whose manifest state could not be read back): the factor's
// folder may still hold files of this attempt. The text is shown to the
// accountant as is, so it says what to do.
var ErrFilesMayRemain = errors.New("some files may remain on the factor's server; check the folder before retrying")

// manifestTempSuffix is the inert name a manifest is written under before it
// is renamed into place — no factor poller treats *.uploading as a trigger.
const manifestTempSuffix = ".uploading"

// shipBatch drops PDFs first and the manifest last. The manifest is the
// "sent" point: before it lands every file this attempt touched is removed,
// so a retry starts from a clean folder; once it has landed the batch is sent
// and never undone (DEV-3299).
func shipBatch(ctx context.Context, client sftpUploader, dir string, pdfs []InvoicePDF, csvName string, csvBytes []byte, onProgress ProgressFunc) (SubmitResult, error) {
	total := len(pdfs) + 1
	touched, err := uploadPDFs(ctx, client, dir, pdfs, total, onProgress)
	if err != nil {
		return SubmitResult{CSVFileName: csvName}, discardUploads(client, touched, err)
	}
	tempUploaded, err := dropManifest(client, dir, csvName, csvBytes)
	if err != nil {
		return settleManifestFailure(client, dir, csvName, int64(len(csvBytes)), tempUploaded, touched, err)
	}
	reportUpload(onProgress, total, total, csvName)
	return SubmitResult{CSVFileName: csvName, Uploaded: append(touched, path.Join(dir, csvName))}, nil
}

// uploadPDFs uploads every PDF in order and returns the remote paths it
// touched — including the one whose upload failed, which may exist half
// written.
func uploadPDFs(ctx context.Context, client sftpUploader, dir string, pdfs []InvoicePDF, total int, onProgress ProgressFunc) ([]string, error) {
	touched := make([]string, 0, len(pdfs))
	for i, pdf := range pdfs {
		if err := ctx.Err(); err != nil {
			return touched, err
		}
		name := sanitizePDFName(pdf.InvoiceNumber)
		touched = append(touched, path.Join(dir, name))
		if _, err := client.Upload(dir, name, pdf.Bytes); err != nil {
			return touched, fmt.Errorf("upload pdf[%d] %s: %w", i, pdf.InvoiceNumber, err)
		}
		reportUpload(onProgress, i+1, total, name)
	}
	return touched, nil
}

// dropManifest writes the manifest under its temp name, then renames it into
// place, so the factor never sees a half-written CSV. tempUploaded reports
// whether the temp file was fully written before the error.
func dropManifest(client sftpUploader, dir, csvName string, csvBytes []byte) (tempUploaded bool, err error) {
	tempName := csvName + manifestTempSuffix
	if _, err := client.Upload(dir, tempName, csvBytes); err != nil {
		return false, fmt.Errorf("upload csv manifest (temp): %w", err)
	}
	if err := client.Rename(dir, tempName, csvName); err != nil {
		return true, fmt.Errorf("activate csv manifest: %w", err)
	}
	return true, nil
}

// settleManifestFailure asks the server what a manifest error left behind.
// Landed → the batch is sent; not landed → this attempt's files are removed;
// unreadable → nothing is removed (the manifest may be live) and the error
// says files may remain.
func settleManifestFailure(client sftpUploader, dir, csvName string, csvSize int64, tempUploaded bool, touched []string, cause error) (SubmitResult, error) {
	landed, err := manifestLanded(client, dir, csvName, csvSize, tempUploaded)
	if err != nil {
		return SubmitResult{CSVFileName: csvName}, fmt.Errorf("%w: %w (manifest check: %v)", ErrFilesMayRemain, cause, err)
	}
	if landed {
		return SubmitResult{CSVFileName: csvName, Uploaded: append(touched, path.Join(dir, csvName))}, nil
	}
	return SubmitResult{CSVFileName: csvName}, discardUploads(client, append(touched, path.Join(dir, csvName+manifestTempSuffix)), cause)
}

// manifestLanded reports whether the manifest reached the factor: the final
// file is there whole, or the fully written temp file is gone — the rename
// went through and the factor may already have collected the drop.
func manifestLanded(client sftpUploader, dir, csvName string, csvSize int64, tempUploaded bool) (bool, error) {
	size, exists, err := client.Size(dir, csvName)
	if err != nil {
		return false, err
	}
	if exists {
		return size == csvSize, nil
	}
	if !tempUploaded {
		return false, nil
	}
	_, tempExists, err := client.Size(dir, csvName+manifestTempSuffix)
	if err != nil {
		return false, err
	}
	return !tempExists, nil
}

// discardUploads removes every path this attempt touched and returns cause,
// marked ErrFilesMayRemain when any removal failed.
func discardUploads(client sftpUploader, paths []string, cause error) error {
	var failed []error
	for _, p := range paths {
		if err := client.Remove(p); err != nil {
			failed = append(failed, err)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%w: %w (cleanup: %v)", ErrFilesMayRemain, cause, errors.Join(failed...))
	}
	return cause
}

func reportUpload(onProgress ProgressFunc, done, total int, detail string) {
	if onProgress != nil {
		onProgress(Progress{Phase: "uploading", Done: done, Total: total, Detail: detail})
	}
}
