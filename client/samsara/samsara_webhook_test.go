package samsara

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// Samsara answers a DELETE for a webhook it no longer has with a 400, not a
// 404. It must come back as ErrNotFound so callers treat it as already removed.
func TestDeleteWebhook_MissingWebhook400IsNotFound(t *testing.T) {
	client := newAlertTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/webhooks/585138643877137" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Webhook ID does not exist 585138643877137","requestId":"x"}`))
	})

	err := client.DeleteWebhook(context.Background(), "585138643877137")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteWebhook_404IsNotFound(t *testing.T) {
	client := newAlertTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := client.DeleteWebhook(context.Background(), "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteWebhook_Other400StillFails(t *testing.T) {
	client := newAlertTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Invalid webhook id format"}`))
	})

	err := client.DeleteWebhook(context.Background(), "bad")
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("want a real error, got %v", err)
	}
}

func TestDeleteWebhook_Success(t *testing.T) {
	client := newAlertTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeleteWebhook(context.Background(), "ok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
