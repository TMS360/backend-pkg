package smsprobe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/TMS360/backend-pkg/client/ringcentral"
)

// Report is everything one probe run learned.
type Report struct {
	ServerURL        string
	OwnerExtensionID string
	Numbers          []ringcentral.PhoneNumber
	Own              Attempt
	Shared           []Attempt
	Verdict          Verdict
}

// HighVolumeNote is the "if the answer is no" half of the ticket: what the
// separate high-volume (A2P) product costs and what it drags in. The figures
// are RingCentral's published ones and are printed as published — a quote for
// our own account has to come from sales in writing before anybody promises a
// price to a customer.
const HighVolumeNote = `### High Volume SMS (A2P) — what the fallback costs

- Different product, different endpoint: ` + "`POST /restapi/v1.0/account/~/a2p-sms/messages`" + ` (plain SMS is ` + "`/account/~/extension/{id}/sms`" + `). Separate opt-in with RingCentral, not a flag we can turn on ourselves.
- Per message (published): about **$0.007 per SMS** outbound and inbound on a US 10DLC number, toll-free the same; current third-party trackers quote **$0.0085–$0.01**. Segments count separately — a text over 160 GSM-7 characters is billed as more than one message.
- Per month: no separate platform subscription is published for the API itself. The recurring cost is **A2P 10DLC carrier registration** — one brand registration plus a per-campaign monthly carrier fee — which is billed through RingCentral and priced by the carriers, not by us.
- Also required before the first message: brand + campaign registration (business details, sample messages, opt-in wording), and throughput is capped per number (10DLC ~1 message/second, toll-free ~8).
- Sources: RingCentral High Volume SMS guide (developers.ringcentral.com/guide/messaging/sms/high-volume) and its pricing announcement thread. **Confirm both numbers with RingCentral sales in writing before this goes into a plan or a customer promise.**`

// Render turns a report into the markdown the ticket comment needs: the
// verdict, the request that was sent, the answer that came back verbatim, and —
// when the answer is anything but "sharing works" — what the fallback costs.
func Render(r Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Answer: %s\n\n%s\n\n", r.Verdict.Answer, r.Verdict.Reason)
	fmt.Fprintf(&b, "- Server: `%s`\n", r.ServerURL)
	fmt.Fprintf(&b, "- Credential's own extension (token `owner_id`): `%s`\n\n", r.OwnerExtensionID)

	b.WriteString("### Account numbers\n\n")
	b.WriteString("| Number | Usage | Type | Features | Extension |\n|---|---|---|---|---|\n")
	for _, n := range r.Numbers {
		ext := "—"
		if n.ExtensionID != "" {
			ext = fmt.Sprintf("%s (%s, id %s)", n.ExtensionNumber, n.ExtensionName, n.ExtensionID)
		}
		features := strings.Join(sortedCopy(n.Features), ", ")
		if features == "" {
			features = "—"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", n.PhoneNumber, n.UsageType, n.Type, features, ext)
	}
	b.WriteString("\n### Attempts\n")

	for _, a := range append([]Attempt{r.Own}, r.Shared...) {
		b.WriteString(renderAttempt(a))
	}

	if r.Verdict.Answer != AnswerSharedAllowed {
		b.WriteString("\n")
		b.WriteString(HighVolumeNote)
		b.WriteString("\n")
	}
	return b.String()
}

func renderAttempt(a Attempt) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n**%s**\n\n", a.Label)
	fmt.Fprintf(&b, "Request:\n```http\nPOST /restapi/v1.0/account/~/extension/%s/sms\n\n%s\n```\n",
		a.ExtensionID, fmt.Sprintf("{\"from\":{\"phoneNumber\":%q},\"to\":[{\"phoneNumber\":%q}],\"text\":%q}", a.From, a.To, a.Text))

	switch {
	case a.Transport != "":
		fmt.Fprintf(&b, "Result: **never reached RingCentral** — %s\n", a.Transport)
	case a.Sent:
		fmt.Fprintf(&b, "Result: **accepted** — id `%s`, messageStatus `%s`\n", a.MessageID, a.MessageStatus)
	default:
		fmt.Fprintf(&b, "Result: **refused** — HTTP %d, errorCode `%s`", a.StatusCode, a.ErrorCode)
		if len(a.SubCodes) > 0 {
			fmt.Fprintf(&b, " (nested: `%s`)", strings.Join(a.SubCodes, "`, `"))
		}
		fmt.Fprintf(&b, "\n\nResponse verbatim:\n```json\n%s\n```\n", a.Raw)
	}
	return b.String()
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
