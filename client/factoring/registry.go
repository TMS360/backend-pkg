package factoring

import (
	"encoding/json"
	"fmt"
)

// NewProviderFromCredential returns the concrete Provider implementation that
// matches cred.ProviderType. This is the only entry point callers need: parse
// the company's stored credential JSON, pass it in, get a ready-to-use
// Provider.
//
// Adding a new factoring backend: declare the constant in factoring.go, add
// the impl file (e.g. apex.go), then add a case here.
func NewProviderFromCredential(cred Credential) (Provider, error) {
	if !cred.ProviderType.IsValid() {
		return nil, fmt.Errorf("factoring: unknown provider_type %q", cred.ProviderType)
	}
	if cred.Username == "" || cred.Password == "" {
		return nil, fmt.Errorf("factoring: %s credential missing username/password", cred.ProviderType)
	}
	switch cred.ProviderType {
	case ProviderTriumphSFTP:
		return NewTriumphSFTP(cred), nil
	case ProviderRTSSFTP:
		return NewRTSSFTP(cred), nil
	default:
		return nil, fmt.Errorf("factoring: provider %q has no implementation yet", cred.ProviderType)
	}
}

// NewProviderFromJSON is a small convenience: parses the raw bytes from
// Redis/DB into Credential and dispatches. tms360-backend already validates
// shape on save, so unmarshal errors here would mean tampering or a stale
// schema.
func NewProviderFromJSON(credentialJSON []byte) (Provider, error) {
	if len(credentialJSON) == 0 {
		return nil, fmt.Errorf("factoring: empty credentials")
	}
	var cred Credential
	if err := json.Unmarshal(credentialJSON, &cred); err != nil {
		return nil, fmt.Errorf("factoring: parse credentials: %w", err)
	}
	return NewProviderFromCredential(cred)
}
