package smsprobe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TMS360/backend-pkg/client/ringcentral"
)

// Config is one probe run.
type Config struct {
	// Cred is the RingCentral app + JWT credential. Point ServerURL at
	// ringcentral.SandboxServerURL: this sends real texts, and a real customer
	// account is the wrong place to experiment.
	Cred ringcentral.Cred
	// To is the recipient, E.164. On the sandbox it must be a number the sandbox
	// account is allowed to text (a sandbox extension's own number is safest).
	To string
	// Text is the message body. A recognisable one makes the run easy to find in
	// the RingCentral message log afterwards.
	Text string

	// OwnFrom / SharedFrom / SharedExtensionID override the automatic pick when
	// the account's numbers are laid out in a way the heuristic gets wrong.
	OwnFrom           string
	SharedFrom        string
	SharedExtensionID string

	// DryRun performs the reads (token, numbers, pick) and skips every send, so
	// the wiring can be checked without spending a message.
	DryRun bool
}

const defaultText = "DEV-1895 probe: can a company login text from a shared number?"

// Run performs the probe: read the account's numbers, then try to send the same
// text three ways — from our own number (the control), from a second
// extension's number through our own extension, and from a second extension's
// number through that extension. The three results together are what answers
// the ticket.
func Run(ctx context.Context, cfg Config) (Report, error) {
	if strings.TrimSpace(cfg.To) == "" {
		return Report{}, errors.New("smsprobe: a recipient (To) is required")
	}
	text := strings.TrimSpace(cfg.Text)
	if text == "" {
		text = defaultText
	}

	client, err := ringcentral.NewClientWithCred(cfg.Cred)
	if err != nil {
		return Report{}, err
	}

	server := strings.TrimSpace(cfg.Cred.ServerURL)
	if server == "" {
		server = ringcentral.DefaultServerURL
	}

	owner, err := client.TokenOwnerExtensionID(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("smsprobe: token exchange failed: %w", err)
	}

	smsOK, smsKnown, smsReason, smsErr := client.SMSSendingAvailable(ctx)
	ownerSMS := FeatureCheck{Known: smsKnown, Available: smsOK, Reason: smsReason}
	if smsErr != nil {
		ownerSMS = FeatureCheck{Error: smsErr.Error()}
	}

	numbers, err := client.ListPhoneNumbers(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("smsprobe: listing phone numbers failed: %w", err)
	}

	pick, err := SelectNumbers(numbers, owner)
	if err != nil && (cfg.OwnFrom == "" || cfg.SharedFrom == "") {
		return Report{}, err
	}
	ownFrom := firstNonEmpty(cfg.OwnFrom, pick.Own.PhoneNumber)
	sharedFrom := firstNonEmpty(cfg.SharedFrom, pick.Shared.PhoneNumber)
	sharedExt := firstNonEmpty(cfg.SharedExtensionID, pick.Shared.ExtensionID)

	report := Report{
		ServerURL:        server,
		OwnerExtensionID: owner,
		OwnerSMS:         ownerSMS,
		Numbers:          numbers,
	}

	report.Own = Attempt{
		Label:       "control — our own number, our own extension",
		ExtensionID: ringcentral.SelfExtension,
		From:        ownFrom,
		To:          cfg.To,
		Text:        text,
	}
	report.Shared = []Attempt{
		{
			Label:       "shared number, sent through OUR extension (the question)",
			ExtensionID: ringcentral.SelfExtension,
			From:        sharedFrom,
			To:          cfg.To,
			Text:        text,
		},
	}
	if sharedExt != "" {
		report.Shared = append(report.Shared, Attempt{
			Label:       fmt.Sprintf("shared number, sent through ITS OWN extension %s (admin acting for a user)", sharedExt),
			ExtensionID: sharedExt,
			From:        sharedFrom,
			To:          cfg.To,
			Text:        text,
		})
	}

	if cfg.DryRun {
		report.Verdict = Verdict{AnswerInconclusive, "dry run: no message was sent"}
		return report, nil
	}

	// Refusing to send when our own extension cannot text is the same rule the
	// verdict already applies, moved one step earlier: the control send would
	// fail for a reason that has nothing to do with sharing, and three real
	// messages would be spent to learn it.
	if ownerSMS.Known && !ownerSMS.Available {
		report.Verdict = Verdict{AnswerInconclusive, fmt.Sprintf(
			"RingCentral reports that our own extension (%s) may not send SMS at all (%s) — nothing was sent; enable texting for this extension, then rerun",
			owner, ownerSMS.Reason)}
		return report, nil
	}

	send(ctx, client, &report.Own)
	for i := range report.Shared {
		send(ctx, client, &report.Shared[i])
	}
	report.Verdict = Classify(report.Own, report.Shared)
	return report, nil
}

// send performs one attempt and records the outcome on it. RingCentral's own
// refusal is copied in verbatim; anything that never reached the platform lands
// in Transport so it cannot be mistaken for an answer.
func send(ctx context.Context, client *ringcentral.Client, a *Attempt) {
	a.Attempted = true
	res, err := client.SendSMS(ctx, ringcentral.SMSRequest{
		ExtensionID: a.ExtensionID,
		From:        a.From,
		To:          []string{a.To},
		Text:        a.Text,
	})
	if err == nil {
		a.Sent = true
		a.MessageID = res.ID
		a.MessageStatus = res.MessageStatus
		return
	}
	if apiErr, ok := ringcentral.AsAPIError(err); ok {
		a.StatusCode = apiErr.StatusCode
		a.ErrorCode = apiErr.Code
		a.SubCodes = apiErr.SubCodes()
		a.Message = apiErr.Message
		a.Raw = apiErr.Body
		return
	}
	a.Transport = err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
