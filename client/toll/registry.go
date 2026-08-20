package toll

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NewProviderFromCredential returns the concrete Provider implementation that
// matches cred.ProviderType. This is the only entry point callers need.
//
// Adding a new toll aggregator: declare the constant in toll.go, add its entry
// to RulesFor, add the impl file, then add a case here.
//
// Required-field validation is driven by RulesFor rather than hardcoded.
// factoring's registry demands a username and a password from every provider,
// which would reject an API-key-only vendor outright; toll asks each type what
// it actually needs, so the first HTTP provider does not require surgery here.
func NewProviderFromCredential(cred Credential) (Provider, error) {
	if !cred.ProviderType.IsValid() {
		return nil, fmt.Errorf("toll: unknown provider_type %q", cred.ProviderType)
	}
	if err := validateCredential(cred); err != nil {
		return nil, err
	}
	switch cred.ProviderType {
	case ProviderPrePassSFTP:
		return NewPrePassSFTP(cred), nil
	default:
		return nil, fmt.Errorf("toll: provider %q has no implementation yet", cred.ProviderType)
	}
}

// validateCredential checks only what the provider's own rules demand.
func validateCredential(cred Credential) error {
	r := RulesFor(cred.ProviderType)
	if r.RequiresUserPassword {
		if strings.TrimSpace(cred.Username) == "" || cred.Password == "" {
			return fmt.Errorf("toll: %s credential missing username/password", cred.ProviderType)
		}
	}
	if r.RequiresAPIKey && strings.TrimSpace(cred.APIKey) == "" {
		return fmt.Errorf("toll: %s credential missing api key", cred.ProviderType)
	}
	// The host requirement is waived on non-production deployments, where the
	// TEST_* override supplies our catcher folder instead of the real one.
	if r.RequiresHost && strings.TrimSpace(cred.Host) == "" && !isNonProdAppEnv() {
		return fmt.Errorf("toll: %s credential missing host", cred.ProviderType)
	}
	return nil
}

// NewProviderFromJSON parses a stored credential blob and dispatches.
func NewProviderFromJSON(credentialJSON []byte) (Provider, error) {
	if len(credentialJSON) == 0 {
		return nil, fmt.Errorf("toll: empty credentials")
	}
	var cred Credential
	if err := json.Unmarshal(credentialJSON, &cred); err != nil {
		return nil, fmt.Errorf("toll: parse credentials: %w", err)
	}
	return NewProviderFromCredential(cred)
}
