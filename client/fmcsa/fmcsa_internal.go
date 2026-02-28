package fmcsa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type FmcsaAPI interface {
	SearchBrokers(ctx context.Context, params SearchParams) (*SearchResponse, error)
	SearchCarriers(ctx context.Context, params SearchParams) (*SearchResponse, error)
}

type client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a clientExternal with a 10-second timeout
func NewClient(baseURL string) FmcsaAPI {
	return &client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SearchBrokers calls the FMCSA API to search for brokers based on the provided parameters
func (c *client) SearchBrokers(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	return c.executeSearch(ctx, "brokers", params)
}

// SearchCarriers calls the FMCSA API to search for carriers based on the provided parameters
func (c *client) SearchCarriers(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	return c.executeSearch(ctx, "carriers", params)
}

func (c *client) executeSearch(ctx context.Context, entityType string, params SearchParams) (*SearchResponse, error) {
	req, err := c.prepareReq(ctx, entityType, params)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fmcsa api call failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// --- LOGGING ---
	fmt.Printf("Fmcsa Status: %s\n", resp.Status)
	fmt.Printf("Fmcsa Body: %s\n", string(bodyBytes))

	if resp.StatusCode > 300 {
		return nil, c.handleAPIError(resp.StatusCode, bodyBytes)
	}

	var searchResp SearchResponse
	if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode fmcsa response: %w", err)
	}
	return &searchResp, nil
}

func (c *client) prepareReq(ctx context.Context, entityType string, params SearchParams) (*http.Request, error) {
	fmt.Println("Searching FMCSA for company: ", url.PathEscape(params.Query))
	reqURL, err := url.Parse(fmt.Sprintf("%s/api/v1/%s", c.baseURL, entityType))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := reqURL.Query()
	q.Add("q", params.Query)
	q.Add("limit", strconv.Itoa(params.Limit))
	q.Add("offset", strconv.Itoa(params.Offset))
	q.Add("active_only", strconv.FormatBool(params.ActiveOnly))
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return req, nil
}

func (c *client) handleAPIError(status int, bodyBytes []byte) error {
	switch status {
	case http.StatusUnprocessableEntity: // 422
		var errResp HTTPValidationError
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Detail != nil {
			return fmt.Errorf("422 status but decode failed (Body: %s)", string(bodyBytes))
		}
		if len(errResp.Detail) > 0 {
			return fmt.Errorf("validation error: %s", errResp.Detail[0].Msg)
		}
		return fmt.Errorf("validation error: unknown details")

	case http.StatusBadRequest: // 400
		return fmt.Errorf("bad request: %s", string(bodyBytes))

	default:
		return fmt.Errorf("api returned unexpected status %d (Body: %s)", status, string(bodyBytes))
	}
}
