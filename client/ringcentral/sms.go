package ringcentral

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// smsPathFormat is the SMS endpoint. It is scoped to an EXTENSION, not to the
	// account: that shape is the whole reason DEV-1895 exists. "~" means "the
	// extension the token belongs to".
	smsPathFormat = "/restapi/v1.0/account/~/extension/%s/sms"

	// SelfExtension is RingCentral's alias for the extension the access token was
	// minted for.
	SelfExtension = "~"

	// FeatureSMSSender marks a number RingCentral will accept in "from" for a
	// plain SMS. A number without it cannot be a text sender no matter who asks.
	FeatureSMSSender = "SmsSender"
)

// APIError is a platform (non-OAuth) rejection, kept verbatim. RingCentral's
// answer to a refused SMS carries the reason in errorCode plus a nested
// per-parameter list, and that code — not the HTTP status — is what tells one
// refusal apart from another ("this number is not yours" vs "your account has
// no texting at all"). Callers that flatten it to "SMS failed" throw away the
// only diagnostic there is.
type APIError struct {
	StatusCode int
	// Code is RingCentral's top-level errorCode (CMN-101, MSG-242, ...).
	Code    string
	Message string
	// Errors are the nested per-parameter errors, in the order returned.
	Errors []APISubError
	// Body is the raw response, truncated, for logs and for pasting into a
	// ticket when the code is one nobody has seen before.
	Body string
}

// APISubError is one entry of RingCentral's nested "errors" array.
type APISubError struct {
	Code      string `json:"errorCode"`
	Message   string `json:"message"`
	Parameter string `json:"parameterName"`
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("ringcentral: request failed (status %d, errorCode=%s): %s [%s: %s]",
			e.StatusCode, e.Code, e.Message, e.Errors[0].Code, e.Errors[0].Message)
	}
	return fmt.Sprintf("ringcentral: request failed (status %d, errorCode=%s): %s", e.StatusCode, e.Code, e.Message)
}

// SubCodes returns the nested error codes, so a caller can classify without
// walking the slice itself.
func (e *APIError) SubCodes() []string {
	out := make([]string, 0, len(e.Errors))
	for _, sub := range e.Errors {
		out = append(out, sub.Code)
	}
	return out
}

// AsAPIError extracts the *APIError from err, if there is one.
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// apiErrorResponse mirrors RingCentral's platform error payload.
type apiErrorResponse struct {
	ErrorCode string        `json:"errorCode"`
	Message   string        `json:"message"`
	Errors    []APISubError `json:"errors"`
}

// SMSRequest is one outbound text.
type SMSRequest struct {
	// ExtensionID selects the extension the message is sent through. Empty means
	// SelfExtension — the extension the credential belongs to.
	ExtensionID string
	// From must be one of that extension's own numbers, in E.164.
	From string
	// To is one or more recipients, in E.164.
	To []string
	// Text is the message body.
	Text string
}

// SMSResult is RingCentral's acknowledgement. MessageStatus is passed through
// verbatim ("Queued", "Sent", "SendingFailed", ...): a queued message is not a
// delivered message, and collapsing the two would let us tell a dispatcher a
// driver was texted when the carrier later refused it.
type SMSResult struct {
	ID            string
	Direction     string
	MessageStatus string
	From          string
	To            []string
}

// SendSMS sends one text. It is the first write this client performs against
// RingCentral; everything else here reads.
//
// ErrInvalidCredentials means the credential no longer authenticates. A refusal
// by the platform itself (wrong sender, feature not enabled, recipient not
// allowed) comes back as *APIError with RingCentral's own code and message
// intact — see AsAPIError.
func (c *Client) SendSMS(ctx context.Context, req SMSRequest) (*SMSResult, error) {
	ext := strings.TrimSpace(req.ExtensionID)
	if ext == "" {
		ext = SelfExtension
	}
	if strings.TrimSpace(req.From) == "" {
		return nil, fmt.Errorf("ringcentral: sms from is required")
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("ringcentral: sms to is required")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fmt.Errorf("ringcentral: sms text is required")
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	type phone struct {
		PhoneNumber string `json:"phoneNumber"`
	}
	to := make([]phone, 0, len(req.To))
	for _, n := range req.To {
		to = append(to, phone{PhoneNumber: n})
	}
	payload := struct {
		From phone   `json:"from"`
		To   []phone `json:"to"`
		Text string  `json:"text"`
	}{From: phone{PhoneNumber: req.From}, To: to, Text: req.Text}

	body, err := c.post(ctx, token, fmt.Sprintf(smsPathFormat, ext), payload)
	if err != nil {
		return nil, err
	}

	var decoded struct {
		ID            json.Number `json:"id"`
		Direction     string      `json:"direction"`
		MessageStatus string      `json:"messageStatus"`
		From          struct {
			PhoneNumber string `json:"phoneNumber"`
		} `json:"from"`
		To []struct {
			PhoneNumber string `json:"phoneNumber"`
		} `json:"to"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to decode sms response: %w", err)
	}

	res := &SMSResult{
		ID:            decoded.ID.String(),
		Direction:     decoded.Direction,
		MessageStatus: decoded.MessageStatus,
		From:          decoded.From.PhoneNumber,
	}
	for _, t := range decoded.To {
		res.To = append(res.To, t.PhoneNumber)
	}
	return res, nil
}

// TokenOwnerExtensionID reports which extension the stored credential belongs
// to. RingCentral returns it as owner_id on the token exchange, and it is the
// only reliable way to tell "our own number" from "somebody else's number" —
// the credential itself says nothing about which extension minted it.
func (c *Client) TokenOwnerExtensionID(ctx context.Context) (string, error) {
	form, err := c.tokenExchange(ctx)
	if err != nil {
		if IsAuthError(err) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	if form.OwnerID == "" {
		return "", fmt.Errorf("ringcentral: token response carried no owner_id")
	}
	return form.OwnerID, nil
}

// post performs an authenticated JSON POST.
func (c *Client) post(ctx context.Context, token, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.send(req, token)
}

// postMultipart is post for a body the endpoint wants as multipart. The fax
// endpoint packages its settings and each document as separate MIME parts, so
// the Content-Type carries a boundary and cannot be a constant.
func (c *Client) postMultipart(ctx context.Context, token, path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	return c.send(req, token)
}

// send performs an authenticated write. Unlike get, a platform rejection is
// preserved as *APIError instead of being flattened into a string: a write has
// far more ways to be refused than a read, and the difference between them is
// the whole answer to DEV-1895.
func (c *Client) send(req *http.Request, token string) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er apiErrorResponse
		_ = json.Unmarshal(body, &er)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Code:       er.ErrorCode,
			Message:    er.Message,
			Errors:     er.Errors,
			Body:       truncate(string(body)),
		}
	}
	return body, nil
}

func truncate(s string) string {
	const max = 2048
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}
