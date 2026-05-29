package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
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

// NewHTTPAuthClient takes the tms-auth base URL (e.g. "http://tms-auth:8080")
// without any trailing path. Hits /api/me/permissions under the hood.
func NewHTTPAuthClient(baseURL string) *HTTPAuthClient {
	return &HTTPAuthClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ResolveUserPerms ignores userID — the resolution happens on tms-auth's
// side based on the forwarded JWT. The userID arg is retained on the
// interface so callers don't need to know transport details; we still
// double-check that the JWT we forward belongs to the userID asked.
func (c *HTTPAuthClient) ResolveUserPerms(ctx context.Context, userID uuid.UUID) ([]string, error) {
	actor, err := middleware.GetActor(ctx)
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

	url := c.baseURL + "/api/me/permissions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+*actor.Token)

	slog.Info("resolving perms", "baseURL", c.baseURL, "fullURL", url)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call tms-auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tms-auth returned status %d", resp.StatusCode)
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
