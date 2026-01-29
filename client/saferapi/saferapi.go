package saferapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SaferApi interface {
	FetchByMCNumber(ctx context.Context, mcNumber string) (*SaferCompanyDTO, error)
	FetchByDOTNumber(ctx context.Context, dotNumber string) (*SaferCompanyDTO, error)
}

type saferAPIService struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewSaferAPIService(apiKey string) SaferApi {
	return &saferAPIService{
		baseURL: "https://saferwebapi.com/v2",
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *saferAPIService) FetchByMCNumber(ctx context.Context, mcNumber string) (*SaferCompanyDTO, error) {
	url := fmt.Sprintf("%s/mcmx/snapshot/%s", s.baseURL, mcNumber)
	return s.executeRequest(ctx, url, "MC")
}

func (s *saferAPIService) FetchByDOTNumber(ctx context.Context, dotNumber string) (*SaferCompanyDTO, error) {
	url := fmt.Sprintf("%s/usdot/snapshot/%s", s.baseURL, dotNumber)
	return s.executeRequest(ctx, url, "DOT")
}

// executeRequest handles the shared logic for performing the HTTP call and decoding the result
func (s *saferAPIService) executeRequest(ctx context.Context, url, entityType string) (*SaferCompanyDTO, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching %s data: %w", entityType, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// --- LOGGING ---
	fmt.Printf("SaferApi Status: %s\n", resp.Status)
	fmt.Printf("SaferApi Body: %s\n", string(bodyBytes))
	// ----------------

	// Handle Error Status Codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp ErrorResponse
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("safer api: %s", errResp.Message)
		}
		return nil, fmt.Errorf("external API returned error status: %d", resp.StatusCode)
	}

	// Decode Successful Response
	var result SaferCompanyDTO
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode Safer response: %w", err)
	}

	return &result, nil
}

type ErrorResponse struct {
	Message string `json:"message"`
}
