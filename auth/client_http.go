package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/google/uuid"
)

// HTTPAuthClient implements AuthServiceClient by calling tms-auth's
// `/api/me/permissions` endpoint with the caller's JWT forwarded in the
// Authorization header. Microservices don't need their own credentials for
// this lookup — the user's identity in their existing token is enough.
type HTTPAuthClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPAuthClient takes the tms-auth base URL (e.g. "http://auth.railway.internal:8080")
// without any trailing path. Hits /api/me/permissions under the hood.
// Prefer the private network URL in every env — public hostnames (auth.tms360.io /
// *.up.railway.app) add edge/WAF failure modes that surface as intermittent 403s
// (DEV-1556).
func NewHTTPAuthClient(baseURL string) *HTTPAuthClient {
	return &HTTPAuthClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// authHTTPStatusError is returned when tms-auth answers with a non-200 status.
// Callers and the retry loop branch on Status rather than parsing the message.
type authHTTPStatusError struct {
	Status int
}

func (e *authHTTPStatusError) Error() string {
	return fmt.Sprintf("tms-auth returned status %d", e.Status)
}

// ResolveUserPerms ignores userID — the resolution happens on tms-auth's
// side based on the forwarded JWT. The userID arg is retained on the
// interface so callers don't need to know transport details; we still
// double-check that the JWT we forward belongs to the userID asked.
//
// Retries: up to 3 attempts total on network errors and 5xx only. Never retries
// 401/403 (token dead or forbidden — surface as unresolved upstream).
func (c *HTTPAuthClient) ResolveUserPerms(ctx context.Context, userID uuid.UUID) ([]string, error) {
	actor, err := consts.GetActor(ctx)
	if err != nil || actor == nil {
		return nil, errors.New("no actor in context — cannot forward token to tms-auth")
	}
	if actor.Token == nil || *actor.Token == "" {
		return nil, errors.New("actor missing JWT — cannot forward to tms-auth")
	}
	if actor.ID != userID {
		// Defense-in-depth: if a caller ever asks for a different user's
		// perms we'd silently leak the wrong identity. Catch it.
		return nil, fmt.Errorf("HTTPAuthClient can only resolve the caller's own perms (actor=%s, requested=%s)", actor.ID, userID)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := 50 * time.Millisecond
			if attempt >= 2 {
				delay = 150 * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		perms, err := c.doResolve(ctx, *actor.Token)
		if err == nil {
			return perms, nil
		}
		lastErr = err
		if !isRetryablePermsError(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *HTTPAuthClient) doResolve(ctx context.Context, token string) ([]string, error) {
	url := c.baseURL + "/api/me/permissions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	slog.Debug("resolving perms", "baseURL", c.baseURL, "fullURL", url)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call tms-auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &authHTTPStatusError{Status: resp.StatusCode}
	}

	var body struct {
		Perms []string `json:"perms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if body.Perms == nil {
		body.Perms = []string{}
	}
	return body.Perms, nil
}

func isRetryablePermsError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var se *authHTTPStatusError
	if errors.As(err, &se) {
		// Retry only 5xx. 401/403 and other 4xx must not be retried.
		return se.Status >= 500 && se.Status <= 599
	}
	// Network / dial failures from client.Do are wrapped as "call tms-auth: …".
	return true
}
