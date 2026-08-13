// Package mail publishes email send requests to backend-mail through the
// transactional outbox.
//
// It is the counterpart of backend-pkg/notify: services do not call an email
// provider directly, they emit an event inside their own database transaction
// and the relay ships it to Kafka topic "emails".
//
// That indirection buys three things a direct call cannot:
//
//   - A rollback un-sends the email. backend-broker currently sends its welcome
//     mail INSIDE the signup transaction, so a failed signup still emails the
//     user; publishing here makes the email share the transaction's fate.
//   - The email survives the process. tms360-backend sends from detached
//     goroutines, so a pod restart loses the mail with no trace.
//   - Delivery status becomes queryable, because backend-mail stores every
//     message and links it to whatever business entities the caller names.
//
// The package deliberately depends on nothing outside the standard library plus
// uuid: backend-pkg is linked into 13 modules, and none of them should inherit a
// provider SDK because they send email.
package mail

import (
	"context"
	"fmt"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
)

// Topic is the Kafka topic backend-mail consumes. It matches the outbox
// aggregate type, per the relay convention (topic defaults to EntityType).
const Topic = "emails"

// actionSend is the outbox event type for a send request.
const actionSend = "send"

// TransactionManager is the subset of tmsdb.TransactionManager needed here.
type TransactionManager interface {
	Publish(ctx context.Context, aggType, evtType string, aggID uuid.UUID, data interface{}, oldData ...interface{}) error
}

// Verifier optionally confirms that a published event actually reached the
// outbox table.
//
// This exists because tmsdb's Publish is non-fatal by design: when the outbox
// INSERT fails INSIDE a transaction it rolls back to a savepoint, logs, and
// returns nil, so the caller's business data still commits and the event is
// silently dropped (see backend-pkg/tmsdb/gorm_tm.go, "Non-fatal by design").
// For a password reset or an invoice that trade-off is wrong, so
// SendOptions.Guaranteed re-reads the row and turns a dropped event into an error.
type Verifier interface {
	// OutboxEventExists reports whether an outbox row exists for this aggregate id.
	OutboxEventExists(ctx context.Context, aggregateID uuid.UUID) (bool, error)
}

// Publisher enqueues emails through the transactional outbox.
type Publisher struct {
	tm       TransactionManager
	verifier Verifier
}

// NewPublisher creates a publisher. Pass your service's TransactionManager.
func NewPublisher(tm TransactionManager) *Publisher {
	return &Publisher{tm: tm}
}

// WithVerifier enables SendOptions.Guaranteed. Without one, Guaranteed is a
// no-op and Send stays best-effort.
func (p *Publisher) WithVerifier(v Verifier) *Publisher {
	p.verifier = v
	return p
}

// Address is one email participant.
type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// Relation describes how an entity relates to the email.
const (
	// RelationPrimary — what the email is about.
	RelationPrimary = "primary"
	// RelationSubject — who or what it concerns.
	RelationSubject = "subject"
	// RelationDerived — a parent or rollup, so the mail also shows on its page.
	RelationDerived = "derived"
	// RelationMentioned — referenced in passing.
	RelationMentioned = "mentioned"
)

// Link attaches the email to a business entity.
//
// Several links per email is the point, and the reason this replaces the
// single-entity driver_pay_email_log: a settlement email concerns the statement
// AND the driver AND the pay batch, and should appear on all three screens.
//
// EntityType uses the backend-pkg/eventlog/events vocabulary — "statements",
// "invoices", "users", "shipments" — so a mail link and an audit event agree on
// what to call a thing.
type Link struct {
	EntityType string    `json:"entity_type"`
	EntityID   uuid.UUID `json:"entity_id"`
	Relation   string    `json:"relation,omitempty"`
}

// Attachment is a file to send.
//
// Prefer FileID: the file is already in backend-files, deduplicated and
// tenant-scoped, and nothing is copied through Kafka. Content is the fallback
// for freshly rendered bytes and is capped — see MaxInlineAttachmentBytes.
type Attachment struct {
	FileID   *uuid.UUID `json:"file_id,omitempty"`
	Content  []byte     `json:"content,omitempty"`
	Filename string     `json:"filename"`
	MIMEType string     `json:"mime_type,omitempty"`
}

// MaxInlineAttachmentBytes bounds bytes carried inline through the Kafka
// payload. Above this, upload to backend-files first and pass a FileID — Kafka
// is a message bus, not a file transfer.
const MaxInlineAttachmentBytes = 1 << 20

// Message is one email to send.
type Message struct {
	// ID is the mail message id. Leave zero and Send generates one; the value is
	// returned so the caller can correlate later. It is stable across outbox
	// retries and seeds the provider's idempotency key, which is what makes an
	// at-least-once redelivery safe.
	ID uuid.UUID `json:"id"`

	// MailboxKey selects the sender identity per company: "billing" becomes
	// billing@zmile.com for one tenant and billing@acme.com for another.
	//
	// Never a literal address — that is precisely what makes per-company sending
	// domains work without branching at the call site.
	MailboxKey string `json:"mailbox_key,omitempty"`

	To  []Address `json:"to"`
	CC  []Address `json:"cc,omitempty"`
	BCC []Address `json:"bcc,omitempty"`

	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`

	// TemplateKey LABELS the message; it does not render it. The caller owns its
	// templates and publishes the rendered Subject and HTML/Text above — the mail
	// service stores this string so a batch of mail can be recognised later, and
	// has no renderer of its own.
	TemplateKey string `json:"template_key,omitempty"`
	// TemplateData travels with the message for the same diagnostic reason: it
	// records what the caller rendered from. Nothing downstream renders it.
	TemplateData map[string]any `json:"template_data,omitempty"`

	Attachments []Attachment      `json:"attachments,omitempty"`
	Links       []Link            `json:"links,omitempty"`
	ThreadID    *uuid.UUID        `json:"thread_id,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`

	// Origin is "system" by default; set "user" when a human composed it.
	Origin string `json:"origin,omitempty"`

	// CompanyID is normally taken from the JWT actor. Set it explicitly on
	// actor-less paths — Kafka consumers, cron jobs, detached goroutines — where
	// there is no JWT to read it from.
	CompanyID *uuid.UUID `json:"company_id,omitempty"`

	// SentBy records the acting user, when there is one.
	SentBy *uuid.UUID `json:"sent_by,omitempty"`
}

// SendOptions tunes delivery guarantees.
type SendOptions struct {
	// Guaranteed makes Send verify the outbox row landed, turning tmsdb's
	// deliberate non-fatal drop into an error. Requires WithVerifier.
	Guaranteed bool
}

// Send enqueues an email and returns its message id.
//
// Safe — and correct — to call inside tm.WithTransaction: a rollback un-sends it.
func (p *Publisher) Send(ctx context.Context, m Message) (uuid.UUID, error) {
	return p.SendWithOptions(ctx, m, SendOptions{})
}

// SendWithOptions is Send with explicit delivery guarantees.
func (p *Publisher) SendWithOptions(ctx context.Context, m Message, opts SendOptions) (uuid.UUID, error) {
	if err := m.validate(); err != nil {
		return uuid.Nil, err
	}

	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Origin == "" {
		m.Origin = "system"
	}
	if m.CompanyID == nil {
		if actor, _ := middleware.GetActor(ctx); actor != nil && actor.Claims != nil && actor.Claims.CompanyID != nil {
			m.CompanyID = actor.Claims.CompanyID
		}
	}
	if m.CompanyID == nil {
		// backend-mail cannot store a message without a tenant, and guessing one
		// would be worse than failing here.
		return uuid.Nil, fmt.Errorf("mail: company_id is required (no JWT actor in context — set Message.CompanyID explicitly)")
	}

	if err := p.tm.Publish(ctx, Topic, actionSend, m.ID, m); err != nil {
		return uuid.Nil, fmt.Errorf("mail: publish: %w", err)
	}

	if opts.Guaranteed && p.verifier != nil {
		ok, err := p.verifier.OutboxEventExists(ctx, m.ID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("mail: verify outbox: %w", err)
		}
		if !ok {
			// tmsdb swallowed the insert failure to protect the caller's
			// business data. For guaranteed mail that silence is the bug.
			return uuid.Nil, fmt.Errorf("mail: outbox event for %s was dropped (tmsdb publish is non-fatal by design)", m.ID)
		}
	}

	return m.ID, nil
}

// SendBatch enqueues several messages, returning their ids in order.
//
// It stops at the first failure: the caller is normally inside a transaction, so
// continuing past an error would commit a partial batch.
func (p *Publisher) SendBatch(ctx context.Context, ms []Message) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ms))
	for i, m := range ms {
		id, err := p.Send(ctx, m)
		if err != nil {
			return out, fmt.Errorf("mail: batch item %d: %w", i, err)
		}
		out = append(out, id)
	}
	return out, nil
}

func (m Message) validate() error {
	if len(m.To) == 0 {
		return fmt.Errorf("mail: at least one To recipient is required")
	}
	for _, a := range m.To {
		if a.Email == "" {
			return fmt.Errorf("mail: recipient email is empty")
		}
	}
	// TemplateKey does NOT satisfy either of these, despite its name.
	//
	// The mail service stores it as a label and renders nothing: the caller owns
	// its own templates and publishes the rendered result. Accepting a bare
	// template key here therefore produced a message with no subject and no body
	// — validated, published, stored, and delivered empty. The name promised a
	// renderer that does not exist on the other side.
	if m.Subject == "" {
		return fmt.Errorf("mail: subject is required")
	}
	if m.HTML == "" && m.Text == "" {
		return fmt.Errorf("mail: html or text is required (template_key is a label, not a renderer)")
	}

	for _, l := range m.Links {
		if l.EntityType == "" || l.EntityID == uuid.Nil {
			return fmt.Errorf("mail: link needs both entity_type and entity_id")
		}
	}

	for _, a := range m.Attachments {
		if a.Filename == "" {
			return fmt.Errorf("mail: attachment filename is required")
		}
		if a.FileID == nil && len(a.Content) == 0 {
			return fmt.Errorf("mail: attachment %q needs a file_id or content", a.Filename)
		}
		if len(a.Content) > MaxInlineAttachmentBytes {
			return fmt.Errorf(
				"mail: attachment %q is %d bytes inline, over the %d limit — upload it to backend-files and pass file_id",
				a.Filename, len(a.Content), MaxInlineAttachmentBytes,
			)
		}
	}

	return nil
}
