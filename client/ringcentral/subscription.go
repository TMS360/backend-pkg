package ringcentral

// Webhook subscriptions (DEV-1898).
//
// An inbound text does not ring. Polling alone would show a driver's "blew a
// tyre" minutes late, which is most of the value gone, so the fast path is a
// RingCentral subscription that posts an event to us within seconds and the
// poller stays as the safety net for whatever the webhook loses.
//
// Two things about subscriptions decide the shape of everything here:
//
//   - They EXPIRE. The platform gives roughly a week and stops delivering
//     silently — no error, no event, nothing. Renewal is therefore not a nicety
//     but the difference between a working feature and one that dies quietly a
//     week after release, which is why ExpiresAt is exposed rather than hidden.
//   - The delivery address must be reachable and is validated at creation: on
//     the first POST RingCentral sends a Validation-Token header that has to be
//     echoed back, or the subscription is never created.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	subscriptionPath = "/restapi/v1.0/subscription"

	// EventFilterInstantSMS is the event that carries an inbound text seconds
	// after it lands. "instant" is the fast lane of the message store; the plain
	// message-store event batches and is not what a waiting dispatcher needs.
	EventFilterInstantSMS = "/restapi/v1.0/account/~/extension/~/message-store/instant?type=SMS"

	// TransportWebhook is the only delivery mode this client supports. The
	// alternative (PubNub) needs a long-lived client-side connection, which a
	// horizontally scaled service does not have.
	TransportWebhook = "WebHook"

	// SubscriptionActive is the only status that delivers events. Anything else
	// (Blacklisted, Suspended) means events are NOT arriving, and that has to be
	// visible rather than assumed.
	SubscriptionActive = "Active"
)

// Subscription is one webhook registration, as RingCentral reports it.
type Subscription struct {
	ID           string
	Status       string
	CreationTime time.Time
	// ExpiresAt is when deliveries stop. Renewal has to happen before it, and a
	// subscription already past it cannot be renewed — only replaced.
	ExpiresAt    time.Time
	EventFilters []string
	// Address is the URL RingCentral posts to; kept so a caller can tell its own
	// subscriptions apart from ones left behind by another environment sharing
	// the same RingCentral account.
	Address string
}

// Expired reports whether this subscription can no longer deliver anything.
func (s Subscription) Expired() bool {
	return !s.ExpiresAt.IsZero() && time.Now().After(s.ExpiresAt)
}

// Deliverable reports whether events are actually arriving right now.
func (s Subscription) Deliverable() bool {
	return strings.EqualFold(s.Status, SubscriptionActive) && !s.Expired()
}

// SubscriptionRequest creates one webhook subscription.
type SubscriptionRequest struct {
	// EventFilters defaults to EventFilterInstantSMS when empty.
	EventFilters []string
	// Address is our own HTTPS endpoint. RingCentral validates it during this
	// call, so it must already be deployed and answering.
	Address string
	// VerificationToken comes back on every delivered event and is how the
	// receiver tells a real RingCentral post from anybody who guessed the URL.
	VerificationToken string
}

type subscriptionPayload struct {
	EventFilters []string `json:"eventFilters"`
	DeliveryMode struct {
		Transport         string `json:"transport"`
		Address           string `json:"address"`
		VerificationToken string `json:"verificationToken,omitempty"`
	} `json:"deliveryMode"`
}

type subscriptionRecord struct {
	ID             string   `json:"id"`
	Status         string   `json:"status"`
	CreationTime   string   `json:"creationTime"`
	ExpirationTime string   `json:"expirationTime"`
	ExpiresIn      int      `json:"expiresIn"`
	EventFilters   []string `json:"eventFilters"`
	DeliveryMode   *struct {
		Transport string `json:"transport"`
		Address   string `json:"address"`
	} `json:"deliveryMode"`
}

func (r subscriptionRecord) toSubscription() Subscription {
	s := Subscription{
		ID:           r.ID,
		Status:       r.Status,
		EventFilters: r.EventFilters,
	}
	if t, err := time.Parse(time.RFC3339, r.CreationTime); err == nil {
		s.CreationTime = t
	}
	if t, err := time.Parse(time.RFC3339, r.ExpirationTime); err == nil {
		s.ExpiresAt = t
	} else if r.ExpiresIn > 0 {
		// Older responses carry only the remaining seconds. Deriving the moment
		// keeps every caller off "how long is left" arithmetic.
		s.ExpiresAt = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	if r.DeliveryMode != nil {
		s.Address = r.DeliveryMode.Address
	}
	return s
}

// CreateSubscription registers our webhook for inbound texts.
//
// RingCentral calls the address during this request and refuses the whole
// subscription if the echo of Validation-Token does not come back, so a failure
// here usually means the receiver is not deployed — not that the credential is
// wrong. The two stay distinguishable: a credential problem is
// ErrInvalidCredentials, a refusal is *APIError with RingCentral's own words.
func (c *Client) CreateSubscription(ctx context.Context, req SubscriptionRequest) (*Subscription, error) {
	address := strings.TrimSpace(req.Address)
	if address == "" {
		return nil, fmt.Errorf("ringcentral: a subscription needs a delivery address")
	}
	filters := req.EventFilters
	if len(filters) == 0 {
		filters = []string{EventFilterInstantSMS}
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	var payload subscriptionPayload
	payload.EventFilters = filters
	payload.DeliveryMode.Transport = TransportWebhook
	payload.DeliveryMode.Address = address
	payload.DeliveryMode.VerificationToken = req.VerificationToken

	body, err := c.post(ctx, token, subscriptionPath, payload)
	if err != nil {
		return nil, err
	}
	return decodeSubscription(body)
}

// RenewSubscription pushes the expiry out. It is the whole reason a renewal
// worker exists: an unrenewed subscription stops delivering without saying so.
//
// A subscription that has already expired cannot be renewed — RingCentral
// answers 404/400 — so a caller that gets an error here should create a new one
// rather than retry the renewal.
func (c *Client) RenewSubscription(ctx context.Context, id string) (*Subscription, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("ringcentral: subscription id is required")
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	body, err := c.write(ctx, http.MethodPost, token, subscriptionPath+"/"+id+"/renew", nil)
	if err != nil {
		return nil, err
	}
	return decodeSubscription(body)
}

// ListSubscriptions returns the subscriptions this credential owns. Used to
// adopt one that already exists instead of piling up a new subscription on every
// service restart.
func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	body, err := c.get(ctx, token, subscriptionPath)
	if err != nil {
		return nil, err
	}

	var page struct {
		Records []subscriptionRecord `json:"records"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to parse subscriptions: %w", err)
	}
	out := make([]Subscription, 0, len(page.Records))
	for _, r := range page.Records {
		out = append(out, r.toSubscription())
	}
	return out, nil
}

// DeleteSubscription removes one. Called when a tenant disconnects RingCentral,
// so we stop asking the platform to post events nobody will read.
func (c *Client) DeleteSubscription(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("ringcentral: subscription id is required")
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return ErrInvalidCredentials
		}
		return err
	}

	_, err = c.write(ctx, http.MethodDelete, token, subscriptionPath+"/"+id, nil)
	return err
}

func decodeSubscription(body []byte) (*Subscription, error) {
	var rec subscriptionRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to parse subscription: %w", err)
	}
	out := rec.toSubscription()
	return &out, nil
}

// write performs an authenticated request with an optional JSON body, for the
// verbs post does not cover. It goes through send, so a platform refusal stays
// an *APIError with RingCentral's own code — the same reason send exists.
func (c *Client) write(ctx context.Context, method string, token, path string, payload any) ([]byte, error) {
	var body *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("ringcentral: failed to encode request: %w", err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.serverURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to create request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, token)
}
