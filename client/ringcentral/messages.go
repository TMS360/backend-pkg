package ringcentral

// Reading texts back out of the message store (DEV-1898).
//
// The webhook is the fast path for an inbound text; this is the safety net. A
// webhook that was never delivered, arrived while we were deploying, or stopped
// because a subscription expired leaves a gap, and the only way to notice is to
// re-read the store with an overlapping window — the same trick the call log
// already lives on, and it is free for the same reason: the message id is the
// idempotency key.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	messageStoreListPathFormat = "/restapi/v1.0/account/~/extension/%s/message-store"

	// messagesPerPage is RingCentral's maximum for this endpoint.
	messagesPerPage = 1000
	// maxMessagePages bounds the loop so a broken paging response cannot spin.
	maxMessagePages = 5

	// DirectionInbound / DirectionOutbound are RingCentral's own words for the
	// direction field, exposed so callers stop spelling them out.
	DirectionInbound  = "Inbound"
	DirectionOutbound = "Outbound"
)

// SMSMessage is one text as the message store holds it — enough to store an
// inbound one we have never seen: who wrote it, to which of our numbers, what
// it said, and what state it is in.
type SMSMessage struct {
	ID        string
	Direction string
	// From / To are E.164 as RingCentral reports them. For an inbound text From
	// is the driver and To is the company number that received it.
	From string
	To   []string
	// Text is the message body. RingCentral calls it "subject" on this endpoint,
	// which is a historical name and not a subject line.
	Text          string
	MessageStatus string
	ErrorCode     string
	CreationTime  time.Time
	// ReadStatus is RingCentral's own read flag. Deliberately NOT used as our
	// unread mark: ours is per conversation and belongs to the office, while this
	// one belongs to the RingCentral extension and would flip when somebody opens
	// the RingCentral app.
	ReadStatus string
}

// SMSQuery is one page of the store. Zero values are fine: they mean "whatever
// RingCentral defaults to", which for a sweep is the recent window.
type SMSQuery struct {
	// ExtensionID defaults to SelfExtension.
	ExtensionID string
	// Direction filters to Inbound or Outbound. Empty means both.
	Direction string
	// DateFrom / DateTo bound the window. A sweep should overlap the previous
	// one — re-reading a message it already stored costs nothing.
	DateFrom time.Time
	DateTo   time.Time
	// PerPage defaults to messagesPerPage.
	PerPage int
}

type smsStoreRecord struct {
	ID            json.Number `json:"id"`
	Direction     string      `json:"direction"`
	Type          string      `json:"type"`
	Subject       string      `json:"subject"`
	MessageStatus string      `json:"messageStatus"`
	ErrorCode     string      `json:"errorCode"`
	CreationTime  time.Time   `json:"creationTime"`
	ReadStatus    string      `json:"readStatus"`
	From          *struct {
		PhoneNumber string `json:"phoneNumber"`
	} `json:"from"`
	To []struct {
		PhoneNumber string `json:"phoneNumber"`
		ErrorCode   string `json:"errorCode"`
	} `json:"to"`
}

type smsStorePage struct {
	Records []smsStoreRecord `json:"records"`
	Paging  struct {
		Page       int `json:"page"`
		TotalPages int `json:"totalPages"`
	} `json:"paging"`
}

// ListSMSMessages reads texts from one extension's message store, following
// pagination. Only SMS is returned: the same store holds voicemail, faxes and
// pager messages, and a sweep that stored those as texts would put a fax cover
// page into a driver's conversation.
func (c *Client) ListSMSMessages(ctx context.Context, q SMSQuery) ([]SMSMessage, error) {
	ext := strings.TrimSpace(q.ExtensionID)
	if ext == "" {
		ext = SelfExtension
	}
	perPage := q.PerPage
	if perPage <= 0 || perPage > messagesPerPage {
		perPage = messagesPerPage
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	path := fmt.Sprintf(messageStoreListPathFormat, ext)
	var out []SMSMessage
	for page := 1; page <= maxMessagePages; page++ {
		v := url.Values{}
		v.Set("messageType", "SMS")
		v.Set("perPage", strconv.Itoa(perPage))
		v.Set("page", strconv.Itoa(page))
		if d := strings.TrimSpace(q.Direction); d != "" {
			v.Set("direction", d)
		}
		if !q.DateFrom.IsZero() {
			v.Set("dateFrom", q.DateFrom.UTC().Format(time.RFC3339))
		}
		if !q.DateTo.IsZero() {
			v.Set("dateTo", q.DateTo.UTC().Format(time.RFC3339))
		}

		body, err := c.get(ctx, token, path+"?"+v.Encode())
		if err != nil {
			return nil, err
		}

		var decoded smsStorePage
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("ringcentral: failed to decode messages: %w", err)
		}
		for _, r := range decoded.Records {
			if r.Type != "" && !strings.EqualFold(r.Type, "SMS") {
				continue
			}
			out = append(out, r.toSMSMessage())
		}
		if decoded.Paging.TotalPages <= page || len(decoded.Records) == 0 {
			break
		}
	}
	return out, nil
}

func (r smsStoreRecord) toSMSMessage() SMSMessage {
	m := SMSMessage{
		ID:            r.ID.String(),
		Direction:     r.Direction,
		Text:          r.Subject,
		MessageStatus: r.MessageStatus,
		ErrorCode:     strings.TrimSpace(r.ErrorCode),
		CreationTime:  r.CreationTime,
		ReadStatus:    r.ReadStatus,
	}
	if r.From != nil {
		m.From = strings.TrimSpace(r.From.PhoneNumber)
	}
	for _, t := range r.To {
		if n := strings.TrimSpace(t.PhoneNumber); n != "" {
			m.To = append(m.To, n)
		}
		if m.ErrorCode == "" {
			m.ErrorCode = strings.TrimSpace(t.ErrorCode)
		}
	}
	return m
}
