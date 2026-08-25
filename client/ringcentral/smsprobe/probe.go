// Package smsprobe answers one question with a real request instead of a guess:
// can ONE company-level RingCentral login send a text FROM a number that
// belongs to a DIFFERENT extension?
//
// It matters because RingCentral's SMS endpoint is scoped to an extension
// (/account/~/extension/{id}/sms) while our integration stores one credential
// per company, and our own rule lets a small office share a direct number
// between several people. If the platform refuses the cross-extension sender,
// per-person texting needs per-person credentials — or a different RingCentral
// product — and the whole texting design changes shape.
//
// The package is split on purpose: Classify, SelectNumbers and Render are pure
// and unit-tested, Run is the thin live layer that needs a real sandbox
// credential. The verdict is derived from RingCentral's own error code, never
// from a guess, and an unrecognised refusal stays INCONCLUSIVE rather than
// being rounded to the nearest familiar answer.
package smsprobe

import (
	"fmt"
	"strings"

	"github.com/TMS360/backend-pkg/client/ringcentral"
)

// Answer is one of the three outcomes the ticket asks us to choose between,
// plus the honest fourth one.
type Answer string

const (
	// AnswerSharedAllowed — the company login may text as any extension's
	// number. Our number-sharing rule survives untouched.
	AnswerSharedAllowed Answer = "SHARED_NUMBER_ALLOWED"
	// AnswerOwnNumberOnly — only the credential's own number may text. A whole
	// company then texts from one number unless every person gets a credential.
	AnswerOwnNumberOnly Answer = "OWN_NUMBER_ONLY"
	// AnswerHighVolumeOnly — the platform points at the separate high-volume
	// (A2P) product, which has its own sign-up, registration and price.
	AnswerHighVolumeOnly Answer = "HIGH_VOLUME_ONLY"
	// AnswerInconclusive — the run proved nothing. Either the control send
	// failed too, or RingCentral answered with something nobody has classified
	// yet. Both mean "read the verbatim body", not "pick the likely one".
	AnswerInconclusive Answer = "INCONCLUSIVE"
)

// Attempt is one send and exactly what came back, kept verbatim. It is the
// evidence the ticket asks for, so nothing here is normalised or prettified.
type Attempt struct {
	// Label names the variant: own number, shared number through our own
	// extension, shared number through the owning extension.
	Label string
	// ExtensionID is the extension the request was addressed to ("~" = ours).
	ExtensionID string
	From        string
	To          string
	Text        string

	// Sent is true only when RingCentral accepted the message.
	Sent          bool
	MessageID     string
	MessageStatus string

	// StatusCode / ErrorCode / SubCodes / Message / Raw are RingCentral's own
	// refusal, untouched.
	StatusCode int
	ErrorCode  string
	SubCodes   []string
	Message    string
	Raw        string

	// Transport records a failure that never reached RingCentral (DNS, timeout,
	// revoked credential). Kept apart from a platform refusal so a broken
	// network is never read as a platform answer.
	Transport string
}

// Verdict is the classified answer plus the reason it was chosen, so a reader
// of the ticket comment can check the reasoning instead of trusting a label.
type Verdict struct {
	Answer Answer
	Reason string
}

// Classify turns the attempts into one of the four answers.
//
// The control send (own number, own extension) is checked first and on purpose:
// if our own number cannot text, the cross-extension attempts carry no
// information at all — they would fail for a reason that has nothing to do with
// sharing, and calling that "no" would be exactly the wrong-for-two-weeks
// conclusion the ticket is trying to avoid.
func Classify(own Attempt, shared []Attempt) Verdict {
	switch {
	case own.Transport != "":
		return Verdict{AnswerInconclusive, fmt.Sprintf(
			"the control send never reached RingCentral (%s); nothing was proved", own.Transport)}
	case !own.Sent && own.StatusCode == 0 && own.ErrorCode == "":
		return Verdict{AnswerInconclusive, "the control send was not performed; nothing was proved"}
	case !own.Sent && isFeatureUnavailable(own):
		return Verdict{AnswerInconclusive, fmt.Sprintf(
			"texting is not enabled for the login's own extension at all (%s: %s) — enable SMS on the test account, or price the high-volume product; the shared-number attempts prove nothing until the control send passes",
			own.ErrorCode, own.Message)}
	case !own.Sent:
		return Verdict{AnswerInconclusive, fmt.Sprintf(
			"the control send from our own number was refused (%s: %s), so the shared-number attempts prove nothing",
			own.ErrorCode, own.Message)}
	}

	if len(shared) == 0 {
		return Verdict{AnswerInconclusive, "the control send passed but no shared-number attempt was performed"}
	}

	for _, a := range shared {
		if a.Sent {
			return Verdict{AnswerSharedAllowed, fmt.Sprintf(
				"%s was accepted (message %s, status %s): one company login can text as another extension's number",
				a.Label, a.MessageID, a.MessageStatus)}
		}
	}

	for _, a := range shared {
		if a.Transport != "" {
			return Verdict{AnswerInconclusive, fmt.Sprintf(
				"%s never reached RingCentral (%s); rerun before concluding anything", a.Label, a.Transport)}
		}
	}

	for _, a := range shared {
		if isHighVolumeHint(a) {
			return Verdict{AnswerHighVolumeOnly, fmt.Sprintf(
				"%s was refused and RingCentral pointed at the high-volume product (%s: %s)",
				a.Label, a.ErrorCode, a.Message)}
		}
	}

	allOwnership := true
	for _, a := range shared {
		if !isOwnershipRejection(a) {
			allOwnership = false
			break
		}
	}
	if allOwnership {
		first := shared[0]
		return Verdict{AnswerOwnNumberOnly, fmt.Sprintf(
			"every shared-number attempt was refused as a sender-ownership problem (%s HTTP %d %s: %s): a text can only go out from a number of the extension the credential belongs to",
			first.Label, first.StatusCode, first.ErrorCode, first.Message)}
	}

	unknown := shared[0]
	for _, a := range shared {
		if !isOwnershipRejection(a) {
			unknown = a
			break
		}
	}
	return Verdict{AnswerInconclusive, fmt.Sprintf(
		"%s was refused with a code we have not classified (HTTP %d %s: %s) — read the verbatim body below before deciding",
		unknown.Label, unknown.StatusCode, unknown.ErrorCode, unknown.Message)}
}

// isOwnershipRejection reports whether RingCentral refused the send because the
// sender number is not this extension's. Two shapes are known: HTTP 400
// CMN-101 "Parameter [from] value is invalid" and the older message "Phone
// number doesn't belong to extension". A 403 on another extension's SMS path is
// the same wall from the other side — the credential may not act as that
// extension — and is counted here for the same reason.
func isOwnershipRejection(a Attempt) bool {
	if a.StatusCode == 403 {
		return true
	}
	msg := strings.ToLower(a.Message + " " + a.Raw)
	if strings.Contains(msg, "belong to extension") || strings.Contains(msg, "belong to the extension") {
		return true
	}
	if hasCode(a, "CMN-101") && (strings.Contains(msg, "[from]") || strings.Contains(msg, "from")) {
		return true
	}
	return false
}

// isFeatureUnavailable reports the "your account cannot text at all" refusal
// (MSG-242 and friends), which is a plan problem, not a sharing answer.
func isFeatureUnavailable(a Attempt) bool {
	if hasCode(a, "MSG-242") {
		return true
	}
	return strings.Contains(strings.ToLower(a.Message), "feature is not available")
}

// isHighVolumeHint reports whether the refusal named the separate A2P /
// high-volume product.
func isHighVolumeHint(a Attempt) bool {
	hay := strings.ToLower(a.Message + " " + a.Raw)
	for _, marker := range []string{"high volume", "high-volume", "highvolume", "a2p", "10dlc"} {
		if strings.Contains(hay, marker) {
			return true
		}
	}
	return false
}

func hasCode(a Attempt, code string) bool {
	if strings.EqualFold(a.ErrorCode, code) {
		return true
	}
	for _, c := range a.SubCodes {
		if strings.EqualFold(c, code) {
			return true
		}
	}
	return false
}

// Pick is the pair of numbers the probe needs: one that belongs to the
// credential's own extension (the control) and one that belongs to somebody
// else (the actual question).
type Pick struct {
	Own    ringcentral.PhoneNumber
	Shared ringcentral.PhoneNumber
}

// SelectNumbers chooses the control and the cross-extension sender from the
// account's numbers. An SMS-capable number is preferred on both sides, because
// a number without the SmsSender feature would be refused for a reason that has
// nothing to do with who owns it.
//
// It refuses rather than improvises: a test account with only one extension
// cannot answer this question, and saying so is more useful than running a
// meaningless send.
func SelectNumbers(numbers []ringcentral.PhoneNumber, ownerExtensionID string) (Pick, error) {
	owner := strings.TrimSpace(ownerExtensionID)
	if owner == "" {
		return Pick{}, fmt.Errorf("smsprobe: owner extension id is required to tell our own number from somebody else's")
	}

	var own, ownFallback, shared, sharedFallback *ringcentral.PhoneNumber
	for i := range numbers {
		n := &numbers[i]
		if n.ExtensionID == "" {
			continue // main company number routed by an IVR: no owner to compare
		}
		smsCapable := hasFeature(*n, ringcentral.FeatureSMSSender)
		if n.ExtensionID == owner {
			if smsCapable && own == nil {
				own = n
			} else if ownFallback == nil {
				ownFallback = n
			}
			continue
		}
		if smsCapable && shared == nil {
			shared = n
		} else if sharedFallback == nil {
			sharedFallback = n
		}
	}

	if own == nil {
		own = ownFallback
	}
	if shared == nil {
		shared = sharedFallback
	}
	if own == nil {
		return Pick{}, fmt.Errorf("smsprobe: no number is assigned to extension %s (the credential's own extension) — assign one in the RingCentral console", owner)
	}
	if shared == nil {
		return Pick{}, fmt.Errorf("smsprobe: no number is assigned to a second extension — add a second user and give it a direct number, otherwise there is nothing to share")
	}
	return Pick{Own: *own, Shared: *shared}, nil
}

func hasFeature(n ringcentral.PhoneNumber, feature string) bool {
	for _, f := range n.Features {
		if strings.EqualFold(f, feature) {
			return true
		}
	}
	return false
}
