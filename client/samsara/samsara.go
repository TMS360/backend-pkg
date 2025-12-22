package samsara

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

type VehicleInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Vin  string `json:"vin"`
}

type VehicleListResponse struct {
	Data []VehicleInfo `json:"data"`
}

type GpsCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Time      string  `json:"time"`
}

type VehicleLocation struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Gps  *GpsCoordinates `json:"gps"`
}

type VehicleLocationResponse struct {
	Data []VehicleLocation `json:"data"`
}

type Client struct {
	httpClient *http.Client
	host       string
	apiKey     string
}

func NewClient(cfg config.SamsaraConfig, apiKey string) (*Client, error) {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		host:       cfg.Host,
		apiKey:     apiKey,
	}, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.host + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("received non-2xx status code: %d (and failed to read response body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("received non-2xx status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

func (c *Client) ListVehicles(ctx context.Context) (vehicles []VehicleInfo, err error) {
	path := "/fleet/vehicles"
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var listResponse VehicleListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return listResponse.Data, nil
}

func (c *Client) GetVehiclesStats(ctx context.Context, vehicleIDs []int64) (locations []VehicleLocation, err error) {
	if len(vehicleIDs) == 0 {
		return []VehicleLocation{}, nil
	}

	var stringIDs []string
	for _, id := range vehicleIDs {
		stringIDs = append(stringIDs, strconv.FormatInt(id, 10))
	}
	idsParam := strings.Join(stringIDs, ",")

	path := fmt.Sprintf("/fleet/vehicles/stats?types=gps&vehicleIds=%s", idsParam)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var locationResponse VehicleLocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&locationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return locationResponse.Data, nil
}

func (c *Client) GetVehicleCoordinates(ctx context.Context, vehicleID int64) (locations *VehicleLocation, err error) {
	path := fmt.Sprintf("/fleet/vehicles/%d/locations", vehicleID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var locationResponse VehicleLocation
	if err := json.NewDecoder(resp.Body).Decode(&locationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &locationResponse, nil
}

func (c *Client) GetVehicleByVIN(ctx context.Context, vin string) (vehicle *VehicleInfo, err error) {
	if vin == "" {
		return nil, fmt.Errorf("VIN cannot be empty")
	}

	path := fmt.Sprintf("/fleet/vehicles?vin=%s", vin)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var vehicleInfo VehicleInfo
	if err := json.NewDecoder(resp.Body).Decode(&vehicleInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response for VIN '%s': %w", vin, err)
	}

	return &vehicleInfo, nil
}
