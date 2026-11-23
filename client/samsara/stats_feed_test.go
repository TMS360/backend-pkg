package samsara

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// feedServer always answers hasNextPage=true; after failAfter pages it
// answers 429 on every call (failAfter=0 never fails).
func feedServer(t *testing.T, failAfter int) (*Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if failAfter > 0 && calls > failAfter {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprintf(w, `{"data":[{"id":"v%d","gps":[{"time":"2026-01-01T00:00:00Z","latitude":1,"longitude":2}]}],
			"pagination":{"endCursor":"c%d","hasNextPage":true}}`, calls, calls)
	}))
	t.Cleanup(srv.Close)
	c, _ := NewClientWithToken("k")
	c.host = srv.URL
	return c, &calls
}

func TestGetVehicleStatsFeed_CapsPagesAndKeepsCursor(t *testing.T) {
	c, calls := feedServer(t, 0)

	res, err := c.GetVehicleStatsFeed(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *calls != MaxStatsFeedPagesPerCall {
		t.Fatalf("calls = %d, want %d", *calls, MaxStatsFeedPagesPerCall)
	}
	if len(res.Data) != MaxStatsFeedPagesPerCall {
		t.Fatalf("data = %d, want %d", len(res.Data), MaxStatsFeedPagesPerCall)
	}
	if want := fmt.Sprintf("c%d", MaxStatsFeedPagesPerCall); res.EndCursor != want || !res.HasMore {
		t.Fatalf("cursor = %q hasMore = %v, want %q true", res.EndCursor, res.HasMore, want)
	}
}

func TestGetVehicleStatsFeed_ErrorAfterPagesReturnsPartial(t *testing.T) {
	c, _ := feedServer(t, 3)

	res, err := c.GetVehicleStatsFeed(context.Background(), "")
	if err == nil {
		t.Fatal("want 429 error")
	}
	if res == nil || len(res.Data) != 3 || res.EndCursor != "c3" {
		t.Fatalf("partial result = %+v", res)
	}
}

func TestGetVehicleStatsFeed_ErrorOnFirstPageReturnsNil(t *testing.T) {
	c, _ := feedServer(t, 0)
	c.host = "http://127.0.0.1:1"

	res, err := c.GetVehicleStatsFeed(context.Background(), "")
	if err == nil || res != nil {
		t.Fatalf("res = %v err = %v", res, err)
	}
	var ae *AuthError
	if errors.As(err, &ae) {
		t.Fatal("unexpected auth error")
	}
}
