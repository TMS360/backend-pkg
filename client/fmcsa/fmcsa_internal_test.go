package fmcsa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, status int, body string) FmcsaAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("expected X-API-Key header, got %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "test-key")
}

// TestGetCompanyNotFound pins the documented contract: a 404 from the FMCSA
// service means the DOT number is unknown, which is a nil result, not an error.
func TestGetCompanyNotFound(t *testing.T) {
	c := newTestClient(t, http.StatusNotFound, `{"detail":"Company not found"}`)
	res, err := c.GetCompany(WithSystemAuth(context.Background()), "2119076")
	if err != nil {
		t.Fatalf("expected no error for 404, got %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result for 404, got %+v", res)
	}
}

func TestGetCompanyServerErrorStillFails(t *testing.T) {
	c := newTestClient(t, http.StatusInternalServerError, `{"detail":"boom"}`)
	res, err := c.GetCompany(WithSystemAuth(context.Background()), "2119076")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if res != nil {
		t.Fatalf("expected nil result for 500, got %+v", res)
	}
}

func TestGetCompanyOK(t *testing.T) {
	c := newTestClient(t, http.StatusOK, `{}`)
	res, err := c.GetCompany(WithSystemAuth(context.Background()), "2119076")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result for 200")
	}
}

// TestVerifyCompanyNotFound ensures VerifyCompany still rejects unknown DOTs.
func TestVerifyCompanyNotFound(t *testing.T) {
	c := newTestClient(t, http.StatusNotFound, `{"detail":"Company not found"}`)
	if _, err := c.VerifyCompany(WithSystemAuth(context.Background()), "2119076", "broker"); err == nil {
		t.Fatal("expected VerifyCompany to return an error for unknown DOT")
	}
}
