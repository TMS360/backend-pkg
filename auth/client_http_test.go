package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/google/uuid"
)

func TestHTTPAuthClient_Retries5xxThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"perms":["accounting"]}`))
	}))
	t.Cleanup(srv.Close)

	client := NewHTTPAuthClient(srv.URL)
	uid := uuid.New()
	tok := "test-token"
	ctx := consts.WithActor(context.Background(), &consts.Actor{ID: uid, Token: &tok})

	perms, err := client.ResolveUserPerms(ctx, uid)
	if err != nil {
		t.Fatalf("ResolveUserPerms: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 3", hits.Load())
	}
	if len(perms) != 1 || perms[0] != "accounting" {
		t.Fatalf("perms = %v", perms)
	}
}

func TestHTTPAuthClient_DoesNotRetry403(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	client := NewHTTPAuthClient(srv.URL)
	uid := uuid.New()
	tok := "test-token"
	ctx := consts.WithActor(context.Background(), &consts.Actor{ID: uid, Token: &tok})

	_, err := client.ResolveUserPerms(ctx, uid)
	if err == nil {
		t.Fatal("expected error")
	}
	var se *authHTTPStatusError
	if !errors.As(err, &se) || se.Status != 403 {
		t.Fatalf("err = %v, want status 403", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (no retry on 403)", hits.Load())
	}
}

func TestHTTPAuthClient_DoesNotRetry401(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := NewHTTPAuthClient(srv.URL)
	uid := uuid.New()
	tok := "test-token"
	ctx := consts.WithActor(context.Background(), &consts.Actor{ID: uid, Token: &tok})

	_, err := client.ResolveUserPerms(ctx, uid)
	if err == nil {
		t.Fatal("expected error")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", hits.Load())
	}
}

func TestIsRetryablePermsError(t *testing.T) {
	if isRetryablePermsError(&authHTTPStatusError{Status: 403}) {
		t.Fatal("403 should not retry")
	}
	if isRetryablePermsError(&authHTTPStatusError{Status: 401}) {
		t.Fatal("401 should not retry")
	}
	if !isRetryablePermsError(&authHTTPStatusError{Status: 502}) {
		t.Fatal("502 should retry")
	}
	if !isRetryablePermsError(fmt.Errorf("call tms-auth: connection reset")) {
		t.Fatal("network wrap should retry")
	}
	if isRetryablePermsError(context.Canceled) {
		t.Fatal("canceled should not retry")
	}
}
