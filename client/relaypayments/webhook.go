package relaypayments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// WebhookHeaderSignature is the HTTP header Relay uses to sign webhook bodies.
const WebhookHeaderSignature = "X-Relay-Signature"

// WebhookEvent is the payload Relay POSTs to the customer's webhook URL.
type WebhookEvent struct {
	ID        string      `json:"id"`
	Category  string      `json:"category,omitempty"`
	Type      string      `json:"type"`   // "transaction"
	Action    string      `json:"action"` // "created"
	CreatedAt time.Time   `json:"created_at,omitempty"`
	Entity    Transaction `json:"entity"`
}

// VerifyWebhookSignature validates the X-Relay-Signature header against the
// raw request body using the company's Relay API key as the HMAC secret.
//
// The signature header format is "[timestamp]|[hex hmac-sha256]". The HMAC is
// computed over the concatenation of timestamp and body bytes. Comparison is
// constant-time. Returns false on any malformed input.
//
// See the "Webhooks (BETA)" section of the Relay TMS Fuel OpenAPI spec.
func VerifyWebhookSignature(body []byte, apiKey, signatureHeader string) bool {
	if apiKey == "" || signatureHeader == "" {
		return false
	}
	parts := strings.SplitN(signatureHeader, "|", 2)
	if len(parts) != 2 {
		return false
	}
	timestamp, provided := parts[0], parts[1]
	if timestamp == "" || provided == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected))
}
