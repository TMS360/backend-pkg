package saferapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SaferApi interface {
	FetchByMCNumber(ctx context.Context, mcNumber string) (*SaferCompanyDTO, error)
}

type saferAPIService struct {
	apiKey string
	client *http.Client
}

func NewSaferAPIService(apiKey string) SaferApi {
	return &saferAPIService{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *saferAPIService) FetchByMCNumber(ctx context.Context, mcNumber string) (*SaferCompanyDTO, error) {
	requestURL := fmt.Sprintf("https://saferwebapi.com/v2/mcmx/snapshot/%s", mcNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching MC data: %w", err)
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
