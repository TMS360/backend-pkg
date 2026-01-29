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
	requestURL := s.baseURL + "/mcmx/snapshot/" + mcNumber
	req, err := s.prepareRequest(ctx, requestURL)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching MC data: %w", err)
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("external API returned error status: %d", resp.StatusCode)
	}

	// 6. Decode Response
	var result SaferCompanyDTO
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode Safer response: %w", err)
	}

	return &result, nil
}

func (s *saferAPIService) FetchByDOTNumber(ctx context.Context, dotNumber string) (*SaferCompanyDTO, error) {
	requestURL := s.baseURL + "/usdot/snapshot/" + dotNumber
	req, err := s.prepareRequest(ctx, requestURL)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching DOT data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("external API returned error status: %d", resp.StatusCode)
	}

	// 6. Decode Response
	var result SaferCompanyDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response JSON: %w", err)
	}

	return &result, nil
}

func (s *saferAPIService) prepareRequest(ctx context.Context, requestURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Accept", "application/json")

	return req, nil
}
