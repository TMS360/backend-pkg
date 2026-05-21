package relaypayments

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ListDriversParams filters and paginates ListDrivers. Zero-valued fields are
// omitted from the query string.
type ListDriversParams struct {
	IntegrationID string
	Offset        int
	Limit         int
	Q             string
}

func (p ListDriversParams) values() url.Values {
	q := url.Values{}
	if p.IntegrationID != "" {
		q.Set("integration_id", p.IntegrationID)
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Q != "" {
		q.Set("q", p.Q)
	}
	return q
}

// CreateDriver creates a driver in Relay for the authenticated organization.
func (c *Client) CreateDriver(ctx context.Context, driver Driver) (*Driver, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/drivers/", nil, driver)
	if err != nil {
		return nil, err
	}
	var out Driver
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListDrivers returns drivers for the authenticated organization.
func (c *Client) ListDrivers(ctx context.Context, params ListDriversParams) ([]Driver, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/drivers/", params.values(), nil)
	if err != nil {
		return nil, err
	}
	var out []Driver
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDriver returns a single driver by Relay external ID.
func (c *Client) GetDriver(ctx context.Context, id string) (*Driver, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/drivers/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return nil, err
	}
	var out Driver
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateDriver updates an existing driver. Caller is responsible for setting
// the desired fields on the Driver payload.
func (c *Client) UpdateDriver(ctx context.Context, id string, driver Driver) (*Driver, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, "/drivers/"+url.PathEscape(id), nil, driver)
	if err != nil {
		return nil, err
	}
	var out Driver
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDriver removes a driver by Relay external ID.
func (c *Client) DeleteDriver(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/drivers/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}
