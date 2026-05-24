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
	body, err := renderTemplate(s.templates, templateFile, data)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
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

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("resend: marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("resend: send email to %v: %w", to, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend: status %d: %s", resp.StatusCode, string(respBody))
	}

	fmt.Printf("Email sent via Resend to %v with subject '%s' (attachments=%d)\n", to, subject, len(attachments))
	return nil
}

type resendPayload struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

type resendAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
}
