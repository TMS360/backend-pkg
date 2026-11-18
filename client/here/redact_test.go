package here

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const secretKey = "test-key-SECRET"

// newDeadClient points a Client at an httptest server that is already closed,
// so every call fails at transport level with a *url.Error carrying the URL.
func newDeadClient(t *testing.T) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	host := strings.TrimPrefix(srv.URL, "http://")
	c := &Client{
		httpClient:  srv.Client(),
		routerHost:  srv.URL,
		geocodeHost: srv.URL,
		lookupHost:  srv.URL,
		apiKey:      secretKey,
	}
	srv.Close()
	return c, host
}

func assertRedacted(t *testing.T, err error, host string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a transport error, got nil")
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("api key leaked in error: %s", err)
	}
	if !strings.Contains(err.Error(), host) {
		t.Fatalf("error lost the host %q: %s", host, err)
	}
	var ue *url.Error
	if !errors.As(err, &ue) {
		t.Fatalf("error is no longer a *url.Error: %T", err)
	}
	if ue.Err == nil {
		t.Fatal("inner Err dropped")
	}
}

func TestTransportError_DoRequestPath_RedactsAPIKey(t *testing.T) {
	c, host := newDeadClient(t)
	_, err := c.GetRoute(context.Background(), RouteRequest{
		Origin:        Coordinates{Latitude: 41.0, Longitude: -87.0},
		Destination:   Coordinates{Latitude: 42.0, Longitude: -88.0},
		TransportMode: "truck",
	})
	assertRedacted(t, err, host)
}

func TestTransportError_TestConnection_RedactsAPIKey(t *testing.T) {
	c, host := newDeadClient(t)
	err := c.TestConnection(context.Background())
	assertRedacted(t, err, host)
}

func TestTransportError_KeepsDeadline(t *testing.T) {
	c, _ := newDeadClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetRoute(ctx, RouteRequest{TransportMode: "truck"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(context.Canceled) lost through redaction: %v", err)
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("api key leaked in error: %s", err)
	}
}

func TestRedactAPIKey(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"key scrubbed, rest kept", "https://router.hereapi.com/v8/routes?apiKey=abc&origin=1%2C2", "https://router.hereapi.com/v8/routes?apiKey=REDACTED&origin=1%2C2"},
		{"no key untouched", "https://geocode.search.hereapi.com/v1/geocode?q=x", "https://geocode.search.hereapi.com/v1/geocode?q=x"},
		{"unparseable dropped", "http://[::1%zz/?apiKey=abc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactAPIKey(tc.in); got != tc.want {
				t.Fatalf("redactAPIKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
