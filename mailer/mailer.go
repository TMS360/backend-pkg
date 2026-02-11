package mailer

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"gopkg.in/gomail.v2"
)

// --- 1. Configuration ---

type Config struct {
	Host     string
	Port     int
	Username string // Leave empty for MailHog
	Password string // Leave empty for MailHog
	From     string
}

// --- 2. Interface (Best Practice for Testing) ---

type Sender interface {
	SendEmail(to []string, subject string, templateFile string, data interface{}) error
}

// --- 3. Implementation ---

type SMTPSender struct {
	dialer    *gomail.Dialer
	from      string
	templates embed.FS // Embedded filesystem
}

func NewSMTPSender(cfg Config, templates embed.FS) *SMTPSender {
	// For MailHog, we usually don't need authentication,
	// but this supports real SMTP servers (Gmail, SES, SendGrid) too.
	d := gomail.NewDialer(cfg.Host, cfg.Port, cfg.Username, cfg.Password)

	return &SMTPSender{
		dialer:    d,
		from:      cfg.From,
		templates: templates,
	}
}

func (s *SMTPSender) SendEmail(to []string, subject string, templateFile string, data interface{}) error {
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

	// C. Send
	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email to %v: %w", to, err)
	}

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
