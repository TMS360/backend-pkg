package fmcsa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var mcRegex = regexp.MustCompile(`^[0-9]+$`)

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

func (c *client) SetAuthToken(ctx context.Context, req *http.Request) error {
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

// SearchBrokers calls the FMCSA API to search for brokers based on the provided parameters
func (c *client) SearchBrokers(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	return c.executeSearch(ctx, "brokers", params)
}

// SearchCarriers calls the FMCSA API to search for carriers based on the provided parameters
func (c *client) SearchCarriers(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	return c.executeSearch(ctx, "carriers", params)
}

// SearchByDOT searches the FMCSA API and strictly filters in-memory for an exact DOT match.
func (c *client) SearchByDOT(ctx context.Context, dot string, entityType *string) (*Result, error) {
	dot = strings.TrimSpace(dot)
	if dot == "" {
		return nil, status.Error(codes.InvalidArgument, "DOT number cannot be empty")
	}

	if !mcRegex.MatchString(dot) {
		return nil, errors.New("MC number must contain only integers")
	}

	results, err := c.FetchFMCSAResults(ctx, dot, entityType)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil // No match found, return nil without error
	}

	return utils.Pointer(results[0]), nil
}

// SearchByMC searches the FMCSA API and strictly filters in-memory for an exact MC match.
func (c *client) SearchByMC(ctx context.Context, mc string, entityType *string) (*Result, error) {
	mc = strings.TrimSpace(mc)
	if mc == "" {
		return nil, status.Error(codes.InvalidArgument, "MC number cannot be empty")
	}

	if !mcRegex.MatchString(mc) {
		return nil, errors.New("MC number must contain only integers")
	}

	// Clean the input to purely numeric (e.g., "MC-12345" becomes "12345")
	cleanInputMC := strings.ReplaceAll(strings.ToUpper(mc), "MC-", "")
	cleanInputMC = strings.ReplaceAll(cleanInputMC, "FF-", "")
	cleanInputMC = strings.ReplaceAll(cleanInputMC, "MX-", "")

	results, err := c.FetchFMCSAResults(ctx, cleanInputMC, entityType)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil // No match found, return nil without error
	}

	return utils.Pointer(results[0]), nil
}

// FetchFMCSAResults handles the core FMCSA API invocation and routing.
func (c *client) FetchFMCSAResults(ctx context.Context, query string, entityType *string) ([]Result, error) {
	if entityType == nil || strings.TrimSpace(*entityType) == "" {
		return nil, status.Error(codes.InvalidArgument, "entity type is required")
	}

	params := SearchParams{
		Query:      query,
		Limit:      20,
		Offset:     0,
		ActiveOnly: true,
	}

	var searchResults *SearchResponse
	var err error

	eType := utils.ValOrEmpty(entityType)
	if eType == "carrier" {
		searchResults, err = c.SearchCarriers(ctx, params)
	} else if eType == "broker" {
		searchResults, err = c.SearchBrokers(ctx, params)
	} else {
		return nil, status.Error(codes.InvalidArgument, "entity type must be either 'carrier' or 'broker'")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search FMCSA for %s: %w", eType, err)
	}

	if searchResults == nil || len(searchResults.Results) == 0 {
		return []Result{}, nil
	}

	return searchResults.Results, nil
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

	if err := c.SetAuthToken(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to set auth token: %w", err)
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
