package ringcentral

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	phoneNumberPath = "/restapi/v1.0/account/~/phone-number"

	// numbersPerPage is RingCentral's maximum page size for the phone-number
	// listing. Fewer round trips for a large account, and small accounts still
	// finish in one call.
	numbersPerPage = 1000
	// maxNumberPages bounds the pagination loop so a broken paging response can
	// never spin forever.
	maxNumberPages = 10
)

// PhoneNumber is one number owned by the company's RingCentral account, in the
// shape an admin screen needs: the number to show, and the extension it belongs
// to so a softphone can be started as that extension.
type PhoneNumber struct {
	// ID is RingCentral's phone-number id.
	ID string
	// PhoneNumber is E.164, e.g. "+15551230000".
	PhoneNumber string
	// UsageType tells a direct line apart from the main company number
	// (DirectNumber, MainCompanyNumber, CompanyNumber, ...).
	UsageType string
	// Type is the service type (VoiceFax, FaxOnly, ...). A fax-only number
	// cannot place calls, so callers may want to filter on it.
	Type string
	// Status is RingCentral's own status ("Normal", "Pending", ...).
	Status string
	// Label is the free-text label set in the RingCentral console, if any.
	Label string
	// Features lists what RingCentral will let this number do
	// (SmsSender, MmsSender, CallerId, ...). A number without FeatureSMSSender
	// cannot be the sender of a text, whoever asks — checking it is cheaper than
	// discovering it from a rejected send.
	Features []string
	// ExtensionID / ExtensionNumber / ExtensionName describe the extension the
	// number rings. Empty when the number is not attached to an extension (e.g.
	// a main company number routed by an IVR).
	ExtensionID     string
	ExtensionNumber string
	ExtensionName   string
}

// phoneNumberRecord mirrors the RingCentral payload. Ids come back as JSON
// numbers, so they are decoded loosely and normalised to strings — the rest of
// the system treats them as opaque identifiers.
type phoneNumberRecord struct {
	ID          json.Number `json:"id"`
	PhoneNumber string      `json:"phoneNumber"`
	UsageType   string      `json:"usageType"`
	Type        string      `json:"type"`
	Status      string      `json:"status"`
	Label       string      `json:"label"`
	Features    []string    `json:"features"`
	Extension   *struct {
		ID              json.Number `json:"id"`
		ExtensionNumber string      `json:"extensionNumber"`
		Name            string      `json:"name"`
	} `json:"extension"`
}

type phoneNumberPage struct {
	Records []phoneNumberRecord `json:"records"`
	Paging  struct {
		Page       int `json:"page"`
		TotalPages int `json:"totalPages"`
	} `json:"paging"`
}

// ListPhoneNumbers returns every phone number on the company's RingCentral
// account, following pagination.
//
// ErrInvalidCredentials means the stored credential no longer authenticates —
// the same signal TestConnection gives — so a caller can show "reconnect"
// instead of "no numbers". Any other error is a transport or platform problem
// and says nothing about the credential.
func (c *Client) ListPhoneNumbers(ctx context.Context) ([]PhoneNumber, error) {
	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	var out []PhoneNumber
	for page := 1; page <= maxNumberPages; page++ {
		q := url.Values{}
		q.Set("perPage", strconv.Itoa(numbersPerPage))
		q.Set("page", strconv.Itoa(page))

		body, err := c.get(ctx, token, phoneNumberPath+"?"+q.Encode())
		if err != nil {
			return nil, err
		}

		var decoded phoneNumberPage
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("ringcentral: failed to decode phone numbers: %w", err)
		}
		for _, r := range decoded.Records {
			out = append(out, r.toPhoneNumber())
		}

		if decoded.Paging.TotalPages <= page || len(decoded.Records) == 0 {
			break
		}
	}
	return out, nil
}

func (r phoneNumberRecord) toPhoneNumber() PhoneNumber {
	n := PhoneNumber{
		ID:          r.ID.String(),
		PhoneNumber: strings.TrimSpace(r.PhoneNumber),
		UsageType:   r.UsageType,
		Type:        r.Type,
		Status:      r.Status,
		Label:       r.Label,
		Features:    r.Features,
	}
	if r.Extension != nil {
		n.ExtensionID = r.Extension.ID.String()
		n.ExtensionNumber = r.Extension.ExtensionNumber
		n.ExtensionName = r.Extension.Name
	}
	return n
}

// get performs an authenticated GET and returns the raw body. A 401/403 on a
// token we just minted means the credential was revoked between the two calls,
// so it maps to ErrInvalidCredentials like every other credential failure.
func (c *Client) get(ctx context.Context, token, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrInvalidCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er errorResponse
		_ = json.Unmarshal(body, &er)
		return nil, fmt.Errorf("ringcentral: request failed with status %d: %s", resp.StatusCode, describe(er, body))
	}
	return body, nil
}
