package relaypayments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

// TestListTransactions_HitsTransactionsHost is the DEV-1109 regression guard:
// GET /fuel/transactions/ must target the ".../api" base (no "/integrations"),
// per Relay's operation-level servers: override. Before the fix this hit
// ".../api/integrations/fuel/transactions/" and 404'd on every poll.
func TestListTransactions_HitsTransactionsHost(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"transaction_id":"txn_1","total_amount_paid":"42.50","created_at":"2026-07-01T09:00:00Z"}]`))
	}))
	defer srv.Close()

	// Host is the "/integrations"-suffixed base, exactly as configured in prod/QA.
	client, err := NewClient(config.RelayConfig{Host: srv.URL + "/api/integrations"}, "test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	txns, err := client.ListTransactions(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}

	if gotPath != "/api/fuel/transactions/" {
		t.Fatalf("request path = %q, want %q (must NOT contain /integrations)", gotPath, "/api/fuel/transactions/")
	}
	if strings.Contains(gotPath, "/integrations") {
		t.Fatalf("request path %q must not contain /integrations", gotPath)
	}
	if gotAuth != "test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "test-key")
	}
	if got := gotQuery.Get("dtstart"); got != from.Format(time.RFC3339) {
		t.Fatalf("dtstart = %q, want %q", got, from.Format(time.RFC3339))
	}
	if got := gotQuery.Get("dtend"); got != to.Format(time.RFC3339) {
		t.Fatalf("dtend = %q, want %q", got, to.Format(time.RFC3339))
	}
	if len(txns) != 1 || txns[0].TransactionID != "txn_1" {
		t.Fatalf("decoded txns = %+v, want one txn_1", txns)
	}
}

// TestOtherEndpointsStillUseIntegrationsHost guards that the fix is scoped to
// /fuel/transactions/ only — drivers/fuelcodes/policies must keep hitting the
// "/integrations"-suffixed host.
func TestOtherEndpointsStillUseIntegrationsHost(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client, err := NewClient(config.RelayConfig{Host: srv.URL + "/api/integrations"}, "test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// TestConnection issues GET /drivers/?limit=1 through the default host.
	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if gotPath != "/api/integrations/drivers/" {
		t.Fatalf("drivers path = %q, want %q (must keep /integrations)", gotPath, "/api/integrations/drivers/")
	}
}
