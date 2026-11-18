package samsara

import (
	"encoding/json"
	"fmt"
	"strings"
)

// A company can hold more than one Samsara organization under a single MC
// (DEV-2056): the tenant's primary token stays in the `samsara_api_key`
// setting — that one is "org 1" and is what every existing reader already
// uses — while any ADDITIONAL organizations live as a JSON list under the
// `samsara_extra_orgs` setting key. Keeping the primary token where it always
// was is what lets the old Settings screen and every current consumer keep
// working untouched.
const (
	// MinTokenLength mirrors the `min=10` validation rule tms-auth applies to
	// the primary samsara_api_key, so an extra organization can never be saved
	// with a token that the primary field would have rejected.
	MinTokenLength = 10

	// MaxExtraOrgs caps the list. The value is stored as one JSON blob in a
	// single settings row and cached whole in Redis; a bound keeps a
	// misbehaving client from growing that row without limit. No real tenant is
	// anywhere near it — Martian, the reason this exists, has two.
	MaxExtraOrgs = 20
)

// ExtraOrg is one additional Samsara organization of a company, beyond the
// primary token stored in `samsara_api_key`.
//
// ID is server-assigned and stable for the life of the row: it is how a caller
// updates one organization without resending every token, and how a later
// per-organization 401 handler can disable exactly the organization that was
// rejected instead of the whole integration.
type ExtraOrg struct {
	ID string `json:"id"`
	// Name is the tenant's own label for the organization. It may be empty —
	// callers render DisplayName instead of assuming a name is present.
	Name string `json:"name"`
	// Token is the Samsara API token for this organization. Stored in full;
	// every read path that leaves the service masks it to its last 4.
	Token string `json:"token"`
	// IsActive is the per-organization on/off switch. An inactive organization
	// keeps its token but must not be polled.
	IsActive bool `json:"is_active"`
}

// DisplayName is the label to show for an organization whose Name the tenant
// left empty. index is the organization's 0-based position in the extras list,
// so the first extra reads "Org 2" — the primary samsara_api_key is org 1.
func (o ExtraOrg) DisplayName(index int) string {
	if name := strings.TrimSpace(o.Name); name != "" {
		return name
	}
	return fmt.Sprintf("Org %d", index+2)
}

// ParseExtraOrgs decodes the raw `samsara_extra_orgs` setting value. A missing
// or blank value is not an error — it is the normal state of a company that
// has only the primary token — and yields no organizations.
func ParseExtraOrgs(raw string) ([]ExtraOrg, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var orgs []ExtraOrg
	if err := json.Unmarshal([]byte(raw), &orgs); err != nil {
		return nil, fmt.Errorf("samsara: parse extra orgs: %w", err)
	}
	return orgs, nil
}

// ActiveExtraOrgs returns only the organizations that are switched on. This is
// the list a poller should walk; the primary token is always polled separately.
func ActiveExtraOrgs(orgs []ExtraOrg) []ExtraOrg {
	active := make([]ExtraOrg, 0, len(orgs))
	for _, o := range orgs {
		if o.IsActive {
			active = append(active, o)
		}
	}
	return active
}
