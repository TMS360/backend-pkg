package mailer

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"strconv"

	"github.com/TMS360/backend-pkg/config"
	"gopkg.in/gomail.v2"
)

// Attachment is an in-memory file to attach to an outgoing email. Use this when
// the file is rendered on the fly (e.g. a freshly-generated PDF) and should not
// touch the local filesystem.
type Attachment struct {
	// Filename is the name shown to the email recipient (e.g. "invoice.pdf").
	Filename string
	// Content is the raw bytes of the attachment.
	Content []byte
	// MIMEType is the Content-Type header for the part (e.g. "application/pdf").
	// If empty, defaults to "application/octet-stream".
	MIMEType string
}

// --- Interface (Best Practice for Testing) ---

type Sender interface {
	SendEmail(to []string, subject string, templateFile string, data interface{}) error
	// SendEmailWithAttachments sends an email and attaches one or more in-memory
	// files. Passing a nil/empty attachments slice is equivalent to SendEmail.
	SendEmailWithAttachments(to []string, subject string, templateFile string, data interface{}, attachments []Attachment) error
}

// --- 3. Implementation ---

type SMTPSender struct {
	dialer    *gomail.Dialer
	from      string
	templates embed.FS // Embedded filesystem
}

func NewSMTPSender(cfg config.MailConfig, templates embed.FS) (*SMTPSender, error) {
	// For MailHog, we usually don't need authentication,
	// but this supports real SMTP servers (Gmail, SES, SendGrid) too.
	d, err := NewEmailDialer(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create email dialer: %v", err)
	}

	return &SMTPSender{
		dialer:    d,
		from:      cfg.From,
		templates: templates,
	}, nil
}

func (s *SMTPSender) SendEmail(to []string, subject string, templateFile string, data interface{}) error {
	return s.SendEmailWithAttachments(to, subject, templateFile, data, nil)
}

// SendEmailWithAttachments renders the template body and dispatches the
// message with zero or more in-memory attachments. Attachments are streamed
// via gomail's AttachReader, so the bytes never hit disk.
func (s *SMTPSender) SendEmailWithAttachments(to []string, subject string, templateFile string, data interface{}, attachments []Attachment) error {
	// A. Parse the HTML Template
	body, err := s.parseTemplate(templateFile, data)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// B. Construct the Message
	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", to...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	// C. Attach in-memory files (if any).
	for _, att := range attachments {
		if att.Filename == "" || len(att.Content) == 0 {
			continue
		}
		mimeType := att.MIMEType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		content := att.Content
		m.Attach(att.Filename,
			gomail.SetCopyFunc(func(w io.Writer) error {
				_, werr := w.Write(content)
				return werr
			}),
			gomail.SetHeader(map[string][]string{
				"Content-Type": {mimeType + `; name="` + att.Filename + `"`},
			}),
		)
	}

	// D. Send
	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email to %v: %w", to, err)
	}

	fmt.Printf("Email sent successfully to %v with subject '%s' (attachments=%d)\n", to, subject, len(attachments))

	return nil
}

// Helper to parse templates from the embedded FS
func (s *SMTPSender) parseTemplate(templateName string, data interface{}) (string, error) {
	t, err := template.ParseFS(s.templates, "templates/"+templateName)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func NewEmailDialer(cfg config.MailConfig) (*gomail.Dialer, error) {
	// 1. Sensible Default
	port := 25

	// 2. Override if provided
	if cfg.Port != "" {
		var err error
		port, err = strconv.Atoi(cfg.Port)
		if err != nil {
			// High-load tip: Wrap errors with context so you know WHERE it failed
			return nil, fmt.Errorf("invalid SMTP port '%s': %w", cfg.Port, err)
		}
	}

	// 3. Optional: Validate port range
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("SMTP port %d is out of valid range (1-65535)", port)
	}

	return gomail.NewDialer(cfg.Host, port, cfg.Username, cfg.Password), nil
}
