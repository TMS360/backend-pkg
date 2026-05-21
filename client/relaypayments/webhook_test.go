package relaypayments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func signRelay(t *testing.T, apiKey, timestamp string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	return timestamp + "|" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	apiKey := "test-api-key"
	body := []byte(`{"id":"whe_abc","type":"transaction","action":"created","entity":{}}`)
	header := signRelay(t, apiKey, "137458791", body)

	if !VerifyWebhookSignature(body, apiKey, header) {
		t.Fatal("expected signature to verify")
	}
}

func TestVerifyWebhookSignature_TamperedBody(t *testing.T) {
	apiKey := "test-api-key"
	body := []byte(`{"id":"whe_abc"}`)
	header := signRelay(t, apiKey, "100", body)

	tampered := []byte(`{"id":"whe_xyz"}`)
	if VerifyWebhookSignature(tampered, apiKey, header) {
		t.Fatal("expected tampered body to fail verification")
	}
}

func TestVerifyWebhookSignature_WrongKey(t *testing.T) {
	body := []byte(`{}`)
	header := signRelay(t, "key-a", "1", body)

	if VerifyWebhookSignature(body, "key-b", header) {
		t.Fatal("expected wrong key to fail verification")
	}
}

func TestVerifyWebhookSignature_TimestampNotInHMAC(t *testing.T) {
	apiKey := "test-api-key"
	body := []byte(`{}`)

	// Build a header where the HMAC was computed over body only (no timestamp).
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write(body)
	header := "999|" + hex.EncodeToString(mac.Sum(nil))

	if VerifyWebhookSignature(body, apiKey, header) {
		t.Fatal("signature without timestamp prefix in HMAC must not verify")
	}
}

func TestVerifyWebhookSignature_MalformedHeader(t *testing.T) {
	apiKey := "test-api-key"
	body := []byte(`{}`)
	cases := map[string]string{
		"no pipe":         "abcdef",
		"empty timestamp": "|abcdef",
		"empty hmac":      "1|",
		"empty header":    "",
	}
	for name, hdr := range cases {
		t.Run(name, func(t *testing.T) {
			if VerifyWebhookSignature(body, apiKey, hdr) {
				t.Fatalf("expected malformed header %q to fail", hdr)
			}
		})
	}
}

func TestVerifyWebhookSignature_EmptyKey(t *testing.T) {
	body := []byte(`{}`)
	header := signRelay(t, "k", "1", body)
	if VerifyWebhookSignature(body, "", header) {
		t.Fatal("expected empty apiKey to fail verification")
	}
}
