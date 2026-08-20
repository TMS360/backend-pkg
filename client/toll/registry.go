package toll

import (
	"encoding/json"
	"fmt"
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
	if err := cred.Validate(); err != nil {
		return nil, err
	}
	switch cred.ProviderType {
	case ProviderPrePassSFTP:
		return NewPrePassSFTP(cred), nil
	default:
		return nil, fmt.Errorf("toll: provider %q has no implementation yet", cred.ProviderType)
	}
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
