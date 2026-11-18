package ringcentral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
)

const (
	// faxPathFormat is the fax endpoint. Like SMS it is scoped to an EXTENSION,
	// not to the account — but unlike SMS nothing here needs a per-person sender:
	// a fax leaves as the company, so SelfExtension is the whole answer and
	// DEV-1895's shared-number question does not reach this file.
	faxPathFormat = "/restapi/v1.0/account/~/extension/%s/fax"

	// messageStorePathFormat reads one stored message back. A fax that is still
	// being retried changes underneath us, so this is the only source of truth
	// for what happened to it.
	messageStorePathFormat = "/restapi/v1.0/account/~/extension/%s/message-store/%s"

	// The two service types RingCentral gives a number that can send a fax.
	numberTypeVoiceFax = "VoiceFax"
	numberTypeFaxOnly  = "FaxOnly"

	// Fax statuses, RingCentral's own words. Queued and Sent are NOT endings:
	// RingCentral redials a busy line for minutes, and a fax sitting in either of
	// them is still being tried. Calling that a failure is how a screen tells an
	// office person a broker never got a document that arrives two minutes later.
	FaxStatusQueued         = "Queued"
	FaxStatusSent           = "Sent"
	FaxStatusDelivered      = "Delivered"
	FaxStatusDeliveryFailed = "DeliveryFailed"
	FaxStatusSendingFailed  = "SendingFailed"
)

// FaxRequest is one fax: one document to one or more numbers.
type FaxRequest struct {
	// ExtensionID selects the extension the fax is sent through. Empty means
	// SelfExtension — the extension the credential belongs to.
	ExtensionID string
	// To is one or more recipients, in E.164.
	To []string
	// Filename is what RingCentral names the attachment. It ends up on the cover
	// page, so it is not cosmetic.
	Filename string
	// ContentType is the document's own MIME type. It has to be the truth:
	// RingCentral renders by it, and a PNG announced as application/pdf comes
	// back as faxErrorCode=RenderingFailed rather than as a decode error.
	ContentType string
	// Content is the document itself.
	Content []byte
	// Resolution is "High" or "Low". Empty leaves RingCentral's default.
	Resolution string
	// CoverPageText is the note on the cover page. Empty sends no cover page.
	CoverPageText string
}

// FaxRecipientResult is what happened to ONE number.
//
// This is the whole reason fax is modelled differently from SMS here: a fax to
// three numbers comes back as three answers, and RingCentral keeps them apart
// for us. Collapsing them into one status would mean a retry after a partial
// failure re-sends to the brokers who already received the document.
type FaxRecipientResult struct {
	PhoneNumber string
	// MessageStatus is RingCentral's own word for this recipient.
	MessageStatus string
	// FaxErrorCode is the reason this recipient failed, empty otherwise
	// (RenderingFailed, NoAnswer, LineBusy, ...).
	FaxErrorCode string
}

// FaxResult is one stored fax message.
type FaxResult struct {
	ID string
	// MessageStatus is the message-level status. Per-recipient truth lives in
	// Recipients; this is only useful when there is exactly one of them.
	MessageStatus string
	// PageCount is what the document rendered to. Zero until RingCentral has
	// rendered it.
	PageCount  int
	Recipients []FaxRecipientResult
}

// FaxStatusIsTerminal answers whether RingCentral has stopped trying.
func FaxStatusIsTerminal(status string) bool {
	switch status {
	case FaxStatusDelivered, FaxStatusDeliveryFailed, FaxStatusSendingFailed:
		return true
	default:
		return false
	}
}

// CanFax reports whether RingCentral will accept this number as a fax sender.
func (n PhoneNumber) CanFax() bool {
	return n.Type == numberTypeVoiceFax || n.Type == numberTypeFaxOnly
}

// faxMessageRecord mirrors the RingCentral payload. Shared by the send response
// and the message-store read, because they are the same resource.
type faxMessageRecord struct {
	ID            json.Number `json:"id"`
	MessageStatus string      `json:"messageStatus"`
	FaxPageCount  int         `json:"faxPageCount"`
	To            []struct {
		PhoneNumber   string `json:"phoneNumber"`
		MessageStatus string `json:"messageStatus"`
		FaxErrorCode  string `json:"faxErrorCode"`
	} `json:"to"`
}

func (r faxMessageRecord) toResult() *FaxResult {
	res := &FaxResult{
		ID:            r.ID.String(),
		MessageStatus: r.MessageStatus,
		PageCount:     r.FaxPageCount,
	}
	for _, t := range r.To {
		// A recipient RingCentral has not judged yet inherits the message-level
		// status, so a caller never has to treat "" as a fourth state.
		status := t.MessageStatus
		if status == "" {
			status = r.MessageStatus
		}
		res.Recipients = append(res.Recipients, FaxRecipientResult{
			PhoneNumber:   t.PhoneNumber,
			MessageStatus: status,
			FaxErrorCode:  t.FaxErrorCode,
		})
	}
	return res
}

// SendFax sends one document to one or more fax numbers.
//
// Everything a caller can get wrong is refused here, before the network: this
// call costs money per recipient and prints paper on somebody's machine.
// ErrInvalidCredentials means the credential no longer authenticates; a refusal
// by the platform comes back as *APIError with RingCentral's own code intact.
func (c *Client) SendFax(ctx context.Context, req FaxRequest) (*FaxResult, error) {
	ext := strings.TrimSpace(req.ExtensionID)
	if ext == "" {
		ext = SelfExtension
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("ringcentral: fax to is required")
	}
	for _, n := range req.To {
		if strings.TrimSpace(n) == "" {
			return nil, fmt.Errorf("ringcentral: fax to contains an empty number")
		}
	}
	if len(req.Content) == 0 {
		return nil, fmt.Errorf("ringcentral: fax content is empty")
	}
	if strings.TrimSpace(req.Filename) == "" {
		return nil, fmt.Errorf("ringcentral: fax filename is required")
	}
	if strings.TrimSpace(req.ContentType) == "" {
		return nil, fmt.Errorf("ringcentral: fax content type is required")
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	body, contentType, err := buildFaxBody(req)
	if err != nil {
		return nil, err
	}

	raw, err := c.postMultipart(ctx, token, fmt.Sprintf(faxPathFormat, ext), body, contentType)
	if err != nil {
		return nil, err
	}

	var decoded faxMessageRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to decode fax response: %w", err)
	}
	return decoded.toResult(), nil
}

// FaxMessage reads one fax back from the message store. This is how a fax that
// was Queued becomes Delivered or names its reason for failing.
func (c *Client) FaxMessage(ctx context.Context, extensionID, messageID string) (*FaxResult, error) {
	ext := strings.TrimSpace(extensionID)
	if ext == "" {
		ext = SelfExtension
	}
	if strings.TrimSpace(messageID) == "" {
		return nil, fmt.Errorf("ringcentral: fax message id is required")
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	body, err := c.get(ctx, token, fmt.Sprintf(messageStorePathFormat, ext, messageID))
	if err != nil {
		return nil, err
	}

	var decoded faxMessageRecord
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to decode fax message: %w", err)
	}
	return decoded.toResult(), nil
}

// buildFaxBody assembles the two-part body the fax endpoint wants: a JSON part
// named "json" carrying the recipients, then one part named "attachment" per
// document carrying its own Content-Type.
//
// CreatePart rather than CreateFormFile: CreateFormFile hardcodes
// application/octet-stream, and an attachment whose type RingCentral cannot see
// is an attachment it cannot render.
func buildFaxBody(req FaxRequest) (*bytes.Buffer, string, error) {
	type phone struct {
		PhoneNumber string `json:"phoneNumber"`
	}
	to := make([]phone, 0, len(req.To))
	for _, n := range req.To {
		to = append(to, phone{PhoneNumber: n})
	}
	settings := struct {
		To            []phone `json:"to"`
		FaxResolution string  `json:"faxResolution,omitempty"`
		CoverPageText string  `json:"coverPageText,omitempty"`
	}{To: to, FaxResolution: req.Resolution, CoverPageText: req.CoverPageText}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to encode fax settings: %w", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	jsonHeader := make(textproto.MIMEHeader)
	jsonHeader.Set("Content-Disposition", `form-data; name="json"`)
	jsonHeader.Set("Content-Type", "application/json")
	jsonPart, err := writer.CreatePart(jsonHeader)
	if err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to create fax settings part: %w", err)
	}
	if _, err := jsonPart.Write(settingsJSON); err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to write fax settings part: %w", err)
	}

	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="attachment"; filename=%q`, req.Filename))
	fileHeader.Set("Content-Type", req.ContentType)
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to create fax attachment part: %w", err)
	}
	if _, err := filePart.Write(req.Content); err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to write fax attachment part: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to close fax body: %w", err)
	}
	return body, writer.FormDataContentType(), nil
}
