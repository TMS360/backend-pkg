package mailer

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

const resendEmailsEndpoint = "https://api.resend.com/emails"

// ResendSender delivers emails through the Resend HTTP API
// (https://resend.com/docs/api-reference/emails/send-email).
type ResendSender struct {
	apiKey    string
	from      string
	endpoint  string
	templates embed.FS
	http      *http.Client
}

func NewResendSender(cfg config.MailConfig, templates embed.FS) (*ResendSender, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("resend: MAIL_API_KEY is required")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("resend: MAIL_FROM is required")
	}

	return &ResendSender{
		apiKey:    cfg.APIKey,
		from:      cfg.From,
		endpoint:  resendEmailsEndpoint,
		templates: templates,
		http:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (s *ResendSender) SendEmail(to []string, subject, templateFile string, data interface{}) error {
	return s.SendEmailWithAttachments(to, subject, templateFile, data, nil)
}

func (s *ResendSender) SendEmailWithAttachments(to []string, subject, templateFile string, data interface{}, attachments []Attachment) error {
	_, err := s.send(to, subject, templateFile, data, attachments, nil)
	return err
}

// SendEmailWithTags renders + sends like SendEmailWithAttachments but also
// attaches Resend tags (echoed back on webhook events) and RETURNS the Resend
// email id parsed from the API response body ({"id":"…"}). The id is the handle
// for delivery-status tracking (webhook correlation + GET /emails/{id}).
//
// This is the method behind the optional SenderWithTracking capability. The
// signature deliberately uses only built-in types and the existing Attachment
// type so callers in other modules can detect it via a locally-declared
// interface (structural type assertion) without importing any new symbol —
// letting them keep building against an older release of this package and have
// tracking light up automatically once this version is deployed.
//
// tags keys/values must match Resend's charset (ASCII letters, digits, `_`,
// `-`); UUID strings qualify. A nil/empty map sends no tags.
func (s *ResendSender) SendEmailWithTags(to []string, subject, templateFile string, data interface{}, attachments []Attachment, tags map[string]string) (string, error) {
	return s.send(to, subject, templateFile, data, attachments, tags)
}

func (s *ResendSender) send(to []string, subject, templateFile string, data interface{}, attachments []Attachment, tags map[string]string) (string, error) {
	body, err := renderTemplate(s.templates, templateFile, data)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	payload := resendPayload{
		From:    s.from,
		To:      to,
		Subject: subject,
		HTML:    body,
	}

	for _, att := range attachments {
		if att.Filename == "" || len(att.Content) == 0 {
			continue
		}
		payload.Attachments = append(payload.Attachments, resendAttachment{
			Filename:    att.Filename,
			Content:     base64.StdEncoding.EncodeToString(att.Content),
			ContentType: att.MIMEType,
		})
	}

	for name, value := range tags {
		if name == "" {
			continue
		}
		payload.Tags = append(payload.Tags, resendTag{Name: name, Value: value})
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("resend: marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("resend: send email to %v: %w", to, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(respBody))
	}

	// Success body is {"id":"re_…"}. A decode failure is non-fatal: the email
	// was accepted, we just lose the tracking handle — return an empty id, not
	// an error, so the send still counts as delivered to the caller.
	var decoded struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBody, &decoded)

	fmt.Printf("Email sent via Resend to %v with subject '%s' (attachments=%d, id=%s)\n", to, subject, len(attachments), decoded.ID)
	return decoded.ID, nil
}

type resendPayload struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
	Tags        []resendTag        `json:"tags,omitempty"`
}

type resendAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
}

type resendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
