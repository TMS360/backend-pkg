package fmcsa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/TMS360/backend-pkg/middleware"
)

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

	if err := c.setAuthToken(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to set auth token: %w", err)
	}

	return req, nil
}

func (c *client) setAuthToken(ctx context.Context, req *http.Request) error {
	// Option A: Explicit System Override
	if isSystemAuth(ctx) {
		if c.systemAPIKey == "" {
			return fmt.Errorf("system API key is not configured on the client")
		}
		req.Header.Set("X-API-Key", c.systemAPIKey)
		return nil
	}

	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return fmt.Errorf("failed to get actor from context: %w", err)
	}

	if actor.Token == nil {
		return fmt.Errorf("no auth token found in context")
	}

	req.Header.Set("Authorization", "Bearer "+*actor.Token)
	return nil
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

func isSystemAuth(ctx context.Context) bool {
	val, ok := ctx.Value(authModeKey{}).(bool)
	return ok && val
}
