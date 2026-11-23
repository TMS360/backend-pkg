package relaypayments

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// ListTransactions returns fuel transactions in the inclusive [dtstart, dtend]
// window. Both bounds are required by Relay.
func (c *Client) ListTransactions(ctx context.Context, dtstart, dtend time.Time) ([]Transaction, error) {
	return c.ListTransactionsForDriver(ctx, dtstart, dtend, "")
}

// ListTransactionsForDriver is ListTransactions scoped to one Relay driver id
// (dr_…). Empty driverID lists the whole account. Fuel-codes already take
// driver_id; transactions accepts the same query param.
func (c *Client) ListTransactionsForDriver(ctx context.Context, dtstart, dtend time.Time, driverID string) ([]Transaction, error) {
	if dtstart.IsZero() || dtend.IsZero() {
		return nil, errors.New("relaypayments: dtstart and dtend are required")
	}
	q := url.Values{}
	q.Set("dtstart", dtstart.UTC().Format(time.RFC3339))
	q.Set("dtend", dtend.UTC().Format(time.RFC3339))
	if driverID != "" {
		q.Set("driver_id", driverID)
	}

	// /fuel/transactions/ lives under a servers: override at ".../api" (no
	// "/integrations") per Relay's OpenAPI spec — use transactionsHost, not host.
	resp, err := c.doRequestTo(ctx, c.transactionsHost, http.MethodGet, "/fuel/transactions/", q, nil)
	if err != nil {
		return nil, err
	}
	var out []Transaction
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}
