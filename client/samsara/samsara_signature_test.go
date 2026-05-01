package samsara

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	secret := "test-secret-key"
	body := []byte(`{"eventId":"abc","eventMs":1700000000000,"eventType":"EngineFaultOn"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	if !VerifyWebhookSignature(body, secret, sig) {
		t.Fatal("expected signature to verify with raw hex")
	}
	if !VerifyWebhookSignature(body, secret, "v1="+sig) {
		t.Fatal("expected signature to verify with v1= prefix")
	}
	if !VerifyWebhookSignature(body, secret, "sha256="+sig) {
		t.Fatal("expected signature to verify with sha256= prefix")
	}
}

func TestVerifyWebhookSignature_Invalid(t *testing.T) {
	secret := "test-secret-key"
	body := []byte(`{"a":1}`)

	if VerifyWebhookSignature(body, secret, "deadbeef") {
		t.Fatal("expected invalid signature to fail")
	}
	if VerifyWebhookSignature(body, "", "deadbeef") {
		t.Fatal("expected empty secret to fail")
	}
	if VerifyWebhookSignature(body, secret, "") {
		t.Fatal("expected empty signature to fail")
	}

	// Tampered body
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(`{"a":2}`))
	wrongSig := hex.EncodeToString(mac.Sum(nil))
	if VerifyWebhookSignature(body, secret, wrongSig) {
		t.Fatal("expected mismatched signature to fail")
	}
}
