package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/middleware"
)

// ClientProvider manages per-company client instances with sync.Map caching.
// API keys are fetched from Redis using pattern: {company_id}:setting:{settingKey}
//
// Usage:
//
//	hereProvider := provider.New("here_api_key", func(apiKey string) (here.Service, error) {
//	    client, err := here.NewClientWithToken(apiKey)
//	    if err != nil { return nil, err }
//	    return here.NewService(client), nil
//	})
//
//	// In HTTP handler (actor in context):
//	svc, err := hereProvider.Get(ctx)
//
//	// In background worker (no actor):
//	svc, err := hereProvider.GetByCompanyID(ctx, companyID)
type ClientProvider[T any] struct {
	clients    sync.Map // companyID string → T
	settingKey string   // e.g. "here_api_key", "samsara_api_key"
	factory    func(apiKey string) (T, error)
}

// New creates a ClientProvider.
//   - settingKey: Redis setting suffix (e.g. "here_api_key")
//   - factory: creates client T from an API key string
func New[T any](settingKey string, factory func(apiKey string) (T, error)) *ClientProvider[T] {
	return &ClientProvider[T]{
		settingKey: settingKey,
		factory:    factory,
	}
}

// Get returns a cached or new client for the company extracted from JWT context.
func (p *ClientProvider[T]) Get(ctx context.Context) (T, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("provider: no actor in context: %w", err)
	}
	companyID := actor.GetCompanyID()
	if companyID == nil {
		var zero T
		return zero, fmt.Errorf("provider: no company_id in token")
	}
	return p.GetByCompanyID(ctx, companyID.String())
}

// GetByCompanyID returns a cached or new client for the given company.
// Use this in background workers where there is no actor in context.
func (p *ClientProvider[T]) GetByCompanyID(ctx context.Context, companyID string) (T, error) {
	if val, ok := p.clients.Load(companyID); ok {
		return val.(T), nil
	}

	apiKey, err := p.fetchAPIKey(ctx, companyID)
	if err != nil {
		var zero T
		return zero, err
	}

	client, err := p.factory(apiKey)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("provider: failed to create client for company %s: %w", companyID, err)
	}

	actual, _ := p.clients.LoadOrStore(companyID, client)
	return actual.(T), nil
}

// GetAPIKey returns just the API key for a company without creating a client.
func (p *ClientProvider[T]) GetAPIKey(ctx context.Context, companyID string) (string, error) {
	return p.fetchAPIKey(ctx, companyID)
}

// Invalidate removes a cached client, forcing re-creation on next Get.
func (p *ClientProvider[T]) Invalidate(companyID string) {
	p.clients.Delete(companyID)
}

// InvalidateAll clears all cached clients.
func (p *ClientProvider[T]) InvalidateAll() {
	p.clients.Range(func(key, _ any) bool {
		p.clients.Delete(key)
		return true
	})
}

// fetchAPIKey reads the API key from Redis directly (bypasses cache.buildKey auto-prefix).
func (p *ClientProvider[T]) fetchAPIKey(ctx context.Context, companyID string) (string, error) {
	redisKey := fmt.Sprintf("%s:setting:%s", companyID, p.settingKey)

	data, err := cache.Client().Get(ctx, redisKey).Bytes()
	if err != nil {
		return "", fmt.Errorf("provider: %s not found for company %s: %w", p.settingKey, companyID, err)
	}

	var apiKey string
	if err := json.Unmarshal(data, &apiKey); err != nil {
		return "", fmt.Errorf("provider: failed to unmarshal %s for company %s: %w", p.settingKey, companyID, err)
	}

	if apiKey == "" {
		return "", fmt.Errorf("provider: %s is empty for company %s", p.settingKey, companyID)
	}

	return apiKey, nil
}
