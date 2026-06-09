package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/middleware"
)

// JSONClientProvider is the multi-field analogue of ClientProvider: instead of
// a single API-key string in Redis, the value at {company_id}:setting:{settingKey}
// is a JSON object that unmarshals into the Cred type. The factory then builds
// the client from the typed credential struct.
//
// Used by integrations that need more than one secret per company — e.g. SFTP
// factoring providers (host/port/username/password/inbound_directory).
//
// Usage:
//
//	type TriumphCred struct {
//	    Host             string `json:"host"`
//	    Port             int    `json:"port"`
//	    Username         string `json:"username"`
//	    Password         string `json:"password"`
//	    InboundDirectory string `json:"inbound_directory"`
//	}
//
//	triumphProvider := provider.NewJSON[TriumphCred, factoring.Provider](
//	    "triumph_sftp_credentials",
//	    func(cred TriumphCred) (factoring.Provider, error) {
//	        return factoring.NewTriumphSFTP(cred), nil
//	    },
//	)
//
//	prov, err := triumphProvider.Get(ctx)
type JSONClientProvider[Cred any, T any] struct {
	settingKey string
	factory    func(cred Cred) (T, error)
}

// NewJSON creates a JSONClientProvider.
//   - settingKey: Redis setting suffix (e.g. "triumph_sftp_credentials")
//   - factory: builds client T from a typed credential struct
func NewJSON[Cred any, T any](settingKey string, factory func(cred Cred) (T, error)) *JSONClientProvider[Cred, T] {
	return &JSONClientProvider[Cred, T]{
		settingKey: settingKey,
		factory:    factory,
	}
}

// Get returns a fresh client for the company extracted from JWT context.
func (p *JSONClientProvider[Cred, T]) Get(ctx context.Context) (T, error) {
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

// GetByCompanyID returns a fresh client for the given company by reading the
// credential JSON from Redis on every call. If the key is absent or empty
// (e.g. integration deactivated), an error is returned and the caller should
// skip the polling tick silently.
func (p *JSONClientProvider[Cred, T]) GetByCompanyID(ctx context.Context, companyID string) (T, error) {
	cred, err := p.fetchCredential(ctx, companyID)
	if err != nil {
		var zero T
		return zero, err
	}

	client, err := p.factory(cred)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("provider: failed to create client for company %s: %w", companyID, err)
	}

	return client, nil
}

// GetCredential returns the unmarshaled credential struct for a company without
// creating a client.
func (p *JSONClientProvider[Cred, T]) GetCredential(ctx context.Context, companyID string) (Cred, error) {
	return p.fetchCredential(ctx, companyID)
}

func (p *JSONClientProvider[Cred, T]) fetchCredential(ctx context.Context, companyID string) (Cred, error) {
	redisKey := fmt.Sprintf("%s:setting:%s", companyID, p.settingKey)

	data, err := cache.Client().Get(ctx, redisKey).Bytes()
	if err != nil {
		var zero Cred
		return zero, fmt.Errorf("provider: %s not found for company %s: %w", p.settingKey, companyID, err)
	}

	var cred Cred

	// Setting values are written to Redis by tms360-backend via cache.Set,
	// which always json.Marshals the value. For string-typed settings (API
	// keys) that produces a JSON-quoted string in Redis. For our credential
	// case the DB column is a JSON object encoded as TEXT, so cache.Set ends
	// up wrapping that text in another set of quotes — i.e. the Redis blob is
	// a JSON string whose contents are the real JSON object.
	//
	// Try the direct decode first (in case some future write path stores raw
	// JSON), then fall back to the double-decode that matches tms360-backend's
	// current behavior.
	if err := json.Unmarshal(data, &cred); err == nil {
		return cred, nil
	}

	var jsonStr string
	if err := json.Unmarshal(data, &jsonStr); err != nil {
		var zero Cred
		return zero, fmt.Errorf("provider: failed to unmarshal %s for company %s: %w", p.settingKey, companyID, err)
	}
	if err := json.Unmarshal([]byte(jsonStr), &cred); err != nil {
		var zero Cred
		return zero, fmt.Errorf("provider: failed to parse %s json for company %s: %w", p.settingKey, companyID, err)
	}
	return cred, nil
}
