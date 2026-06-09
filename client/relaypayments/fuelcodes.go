package relaypayments

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ListOneTimeCodesParams filters and paginates ListOneTimeCodes. DTStart and
// DTEnd are required by Relay; zero values are not sent so callers will see a
// 400 if they forget to set them.
type ListOneTimeCodesParams struct {
	Offset   int
	Limit    int
	DriverID string
	DTStart  time.Time
	DTEnd    time.Time
}

func (p ListOneTimeCodesParams) values() url.Values {
	q := url.Values{}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.DriverID != "" {
		q.Set("driver_id", p.DriverID)
	}
	if !p.DTStart.IsZero() {
		q.Set("dtstart", p.DTStart.UTC().Format(time.RFC3339))
	}
	if !p.DTEnd.IsZero() {
		q.Set("dtend", p.DTEnd.UTC().Format(time.RFC3339))
	}
	return q
}

// CreateOneTimeCode creates a one-time fuel or cash code.
func (c *Client) CreateOneTimeCode(ctx context.Context, code OneTimeCode) (*OneTimeCode, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/fuelcodes/", nil, code)
	if err != nil {
		return nil, err
	}
	var out OneTimeCode
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOneTimeCodes returns one-time codes in `active` or `locked` status for
// the given date range.
func (c *Client) ListOneTimeCodes(ctx context.Context, params ListOneTimeCodesParams) ([]OneTimeCode, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/fuelcodes/", params.values(), nil)
	if err != nil {
		return nil, err
	}
	var out []OneTimeCode
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOneTimeCode returns details about a single one-time code by Relay external ID.
func (c *Client) GetOneTimeCode(ctx context.Context, id string) (*OneTimeCode, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/fuelcodes/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return nil, err
	}
	var out OneTimeCode
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateOneTimeCode updates a one-time code; only allowed in `created` status.
func (c *Client) UpdateOneTimeCode(ctx context.Context, id string, code OneTimeCode) (*OneTimeCode, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, "/fuelcodes/"+url.PathEscape(id), nil, code)
	if err != nil {
		return nil, err
	}
	var out OneTimeCode
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteOneTimeCode soft-deletes a one-time code; only allowed in `created` status.
func (c *Client) DeleteOneTimeCode(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/fuelcodes/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}
