package fmcsa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a client with a 10-second timeout
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: "https://mobile.fmcsa.dot.gov/qc/services/carriers/",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SearchCompaniesByName calls the FMCSA API
func (c *Client) SearchCompaniesByName(ctx context.Context, name string) ([]Carrier, error) {
	req, err := c.prepareReq(ctx, "name/"+url.PathEscape(name))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fmcsa api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fmcsa returned non-200 status: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode fmcsa response: %w", err)
	}

	var carriers []Carrier
	for _, item := range searchResp.Content {
		carriers = append(carriers, item.Carrier)
	}

	return carriers, nil
}

func (c *Client) prepareReq(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("webKey", c.apiKey)
	req.URL.RawQuery = q.Encode()

	return req, nil
}
