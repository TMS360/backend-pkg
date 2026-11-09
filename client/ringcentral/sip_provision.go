package ringcentral

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// sipProvisionPath hands out a SIP registration for a browser phone (DEV-3980).
// It takes no extension parameter: the registration is always for the
// extension the access token belongs to ("~"). A company credential therefore
// provisions its own owner's extension, never some other extension's number.
const sipProvisionPath = "/restapi/v1.0/client-info/sip-provision"

// ErrSIPPhoneLimit means RingCentral refused because the extension already has
// as many phones registered as it allows (5 active instances). RingCentral
// usually says this at SIP REGISTER time ("603 Too Many Contacts"), which only
// the browser sees; this is the rarer case where the REST call says it first.
var ErrSIPPhoneLimit = errors.New("ringcentral: too many phones registered on this extension")

// ErrSIPCallingNotAllowed means the extension or the app may not register a
// phone — no VoIP calling permission, or the app lacks the scope.
var ErrSIPCallingNotAllowed = errors.New("ringcentral: this extension may not register a phone")

// sipRefusal wraps the platform's own answer so a caller can match the
// sentinel with errors.Is and still reach the *APIError for its logs.
type sipRefusal struct {
	kind error
	api  *APIError
}

func (e *sipRefusal) Error() string        { return e.kind.Error() + ": " + e.api.Error() }
func (e *sipRefusal) Is(target error) bool { return target == e.kind }
func (e *sipRefusal) Unwrap() error        { return e.api }

type sipProvisionRequest struct {
	SipInfo []sipTransport `json:"sipInfo"`
}

type sipTransport struct {
	Transport string `json:"transport"`
}

// sipProvisionAnswer keeps only what a web phone needs, in RingCentral's own
// shape and nesting, so the browser can pass it to the web-phone SDK as is.
type sipProvisionAnswer struct {
	SipInfo  []json.RawMessage `json:"sipInfo"`
	SipFlags json.RawMessage   `json:"sipFlags,omitempty"`
	Device   *sipDevice        `json:"device,omitempty"`
}

type sipDevice struct {
	ID flexID `json:"id"`
}

// ProvisionSIP asks RingCentral for a WebSocket (WSS) SIP registration for the
// token's own extension and returns it as opaque JSON:
// {"sipInfo":[<first entry>],"sipFlags":{...},"device":{"id":"..."}}.
//
// accessToken is the token to provision under. Pass the token you are about to
// hand out, so the registration and the token share one lifetime and one token
// exchange; empty means "mint one", like every other call in this package.
//
// The answer holds SIP credentials. Hand it to the user it was made for and
// never store it.
func (c *Client) ProvisionSIP(ctx context.Context, accessToken string) (json.RawMessage, error) {
	token := accessToken
	if token == "" {
		var err error
		token, _, err = c.AccessToken(ctx)
		if err != nil {
			if IsAuthError(err) {
				return nil, ErrInvalidCredentials
			}
			return nil, err
		}
	}

	body, err := c.post(ctx, token, sipProvisionPath, sipProvisionRequest{
		SipInfo: []sipTransport{{Transport: "WSS"}},
	})
	if err != nil {
		return nil, classifySIPRefusal(err)
	}

	var answer sipProvisionAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("ringcentral: failed to decode sip provision: %w", err)
	}
	if len(answer.SipInfo) == 0 {
		return nil, fmt.Errorf("ringcentral: sip provision returned no sipInfo")
	}
	answer.SipInfo = answer.SipInfo[:1]

	out, err := json.Marshal(answer)
	if err != nil {
		return nil, fmt.Errorf("ringcentral: failed to encode sip provision: %w", err)
	}
	return out, nil
}

// classifySIPRefusal turns a platform refusal into one of the two sentinels
// when it is one; anything else (outage, bad credential) passes through.
func classifySIPRefusal(err error) error {
	api, ok := AsAPIError(err)
	if !ok {
		return err
	}
	text := strings.ToLower(api.Message + " " + api.Body)
	switch {
	case strings.Contains(text, "too many contacts"),
		strings.Contains(text, "too many devices"),
		strings.Contains(text, "device limit"):
		return &sipRefusal{kind: ErrSIPPhoneLimit, api: api}
	case api.StatusCode == http.StatusForbidden:
		// CMN-408 and its relatives: "user needs to have [VoipCalling]
		// permission", or the app was never granted the scope.
		return &sipRefusal{kind: ErrSIPCallingNotAllowed, api: api}
	}
	return err
}
