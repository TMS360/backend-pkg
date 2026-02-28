package fmcsa_external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type FmcsaExternalApi interface {
	SearchCompaniesByName(ctx context.Context, name string) ([]Carrier, error)
}

type clientExternal struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewClientExternal creates a clientExternal with a 10-second timeout
func NewClientExternal(apiKey string) FmcsaExternalApi {
	return &clientExternal{
		apiKey:  apiKey,
		baseURL: "https://mobile.fmcsa.dot.gov/qc/services/carriers/",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SearchCompaniesByName calls the FMCSA API
func (c *clientExternal) SearchCompaniesByName(ctx context.Context, name string) ([]Carrier, error) {
	fmt.Println("Searching FMCSA for company name: ", url.PathEscape(name))
	req, err := c.prepareReq(ctx, "name/"+url.PathEscape(name))
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fmcsa returned non-200 status: %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode fmcsa response: %w", err)
	}

	var carriers []Carrier
	for _, item := range searchResp.Content {
		carriers = append(carriers, item.Carrier)
	}

	return carriers, nil
}

func (c *clientExternal) prepareReq(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("webKey", c.apiKey)
	req.URL.RawQuery = q.Encode()

	return req, nil
}
