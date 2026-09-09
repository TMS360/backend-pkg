package ringcentral

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// extensionFeaturesPathFormat lists what one extension is allowed to do.
	// RingCentral reports capabilities relative to the asking credential, so this
	// is the only reliable answer to "may we text at all".
	extensionFeaturesPathFormat = "/restapi/v1.0/account/~/extension/%s/features"

	// FeatureSMSSending is the extension feature that gates plain outbound SMS.
	// It is NOT the same as PhoneNumber.Features[SmsSender]: a real account
	// returns no per-number features at all from the account-level number
	// listing, so the extension feature is what has to be checked.
	FeatureSMSSending = "SMSSending"
)

// ExtensionFeature is one capability RingCentral reports for an extension,
// with the reason it is unavailable kept intact — "the feature is turned off
// for the current account" and "no permission granted" are different problems
// and lead to different fixes.
type ExtensionFeature struct {
	ID            string
	Available     bool
	ReasonCode    string
	ReasonMessage string
}

type extensionFeatureRecord struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
	Reason    *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"reason"`
}

type extensionFeaturePage struct {
	Records []extensionFeatureRecord `json:"records"`
}

// SelfExtensionFeatures lists the capabilities of the extension the credential
// belongs to.
//
// Only the credential's own extension is asked for on purpose: a GET on another
// extension answers 403 for a plain (non-admin) user, and this client maps
// 401/403 to ErrInvalidCredentials — a missing permission would read as a
// revoked credential, which is a far more alarming and entirely wrong story.
func (c *Client) SelfExtensionFeatures(ctx context.Context) ([]ExtensionFeature, error) {
	token, _, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, token, fmt.Sprintf(extensionFeaturesPathFormat, SelfExtension))
	if err != nil {
		return nil, err
	}

	var page extensionFeaturePage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to parse extension features: %w", err)
	}

	out := make([]ExtensionFeature, 0, len(page.Records))
	for _, r := range page.Records {
		f := ExtensionFeature{ID: r.ID, Available: r.Available}
		if r.Reason != nil {
			f.ReasonCode = r.Reason.Code
			f.ReasonMessage = r.Reason.Message
		}
		out = append(out, f)
	}
	return out, nil
}

// SMSSendingAvailable answers "may this credential's own extension send a plain
// text at all", which has to be settled before any cross-extension experiment:
// a refusal on a shared number says nothing while our own number cannot text
// either.
//
// The returned reason is RingCentral's own wording when the answer is no. An
// account that does not report the feature at all yields found=false — unknown,
// not "no", because refusing to try on a missing field would be a guess.
func (c *Client) SMSSendingAvailable(ctx context.Context) (available, found bool, reason string, err error) {
	features, err := c.SelfExtensionFeatures(ctx)
	if err != nil {
		return false, false, "", err
	}
	for _, f := range features {
		if !strings.EqualFold(f.ID, FeatureSMSSending) {
			continue
		}
		reason = strings.TrimSpace(f.ReasonCode + " " + f.ReasonMessage)
		return f.Available, true, reason, nil
	}
	return false, false, "", nil
}

// selfExtensionPath is the credential's own extension record. It is the one
// call that answers "which RingCentral ACCOUNT is this token in", which the
// token exchange itself never says: owner_id names the extension, not the
// account it lives in.
const selfExtensionPath = "/restapi/v1.0/account/~/extension/~"

// ExtensionInfo is who a token belongs to, as RingCentral sees it.
type ExtensionInfo struct {
	ID              string
	AccountID       string
	ExtensionNumber string
	Name            string
	Email           string
	Type            string
	Status          string
}

// SelfExtensionInfo identifies the holder of this credential.
//
// Added for DEV-2111, where a person authorizes TMS against their own
// RingCentral login: the account id is what tells "this dispatcher connected
// the company's phone system" apart from "this dispatcher connected their
// personal RingCentral", and the name and extension number are what the admin
// screen shows instead of a bare id.
func (c *Client) SelfExtensionInfo(ctx context.Context) (*ExtensionInfo, error) {
	token, _, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body, err := c.get(ctx, token, selfExtensionPath)
	if err != nil {
		return nil, err
	}

	var decoded struct {
		ID      json.Number `json:"id"`
		Account struct {
			ID json.Number `json:"id"`
		} `json:"account"`
		ExtensionNumber string `json:"extensionNumber"`
		Name            string `json:"name"`
		Contact         struct {
			Email     string `json:"email"`
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"contact"`
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to decode the extension record: %w", err)
	}

	out := &ExtensionInfo{
		ID:              decoded.ID.String(),
		AccountID:       decoded.Account.ID.String(),
		ExtensionNumber: decoded.ExtensionNumber,
		Name:            strings.TrimSpace(decoded.Name),
		Email:           decoded.Contact.Email,
		Type:            decoded.Type,
		Status:          decoded.Status,
	}
	if out.Name == "" {
		out.Name = strings.TrimSpace(decoded.Contact.FirstName + " " + decoded.Contact.LastName)
	}
	return out, nil
}
