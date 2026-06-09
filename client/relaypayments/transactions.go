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
	if dtstart.IsZero() || dtend.IsZero() {
		return nil, errors.New("relaypayments: dtstart and dtend are required")
	}
	q := url.Values{}
	q.Set("dtstart", dtstart.UTC().Format(time.RFC3339))
	q.Set("dtend", dtend.UTC().Format(time.RFC3339))

	resp, err := c.doRequest(ctx, http.MethodGet, "/fuel/transactions/", q, nil)
	if err != nil {
		return nil, err
	}
	var out []Transaction
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}
