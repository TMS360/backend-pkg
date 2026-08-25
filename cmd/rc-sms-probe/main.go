// Command rc-sms-probe answers DEV-1895 with a real request: can one
// company-level RingCentral login send a text FROM a number that belongs to a
// different extension (our shared-number rule)?
//
// It prints a markdown report — verdict, the request that was sent, the answer
// that came back verbatim — meant to be pasted straight into the ticket.
//
// Run it against the RingCentral developer sandbox, never a customer account:
//
//	RC_CLIENT_ID=... RC_CLIENT_SECRET=... RC_JWT=... RC_TO=+15551230000 \
//	  go run ./cmd/rc-sms-probe
//
// Environment:
//
//	RC_CLIENT_ID, RC_CLIENT_SECRET, RC_JWT   the sandbox app credential (required)
//	RC_TO                                    recipient, E.164 (required)
//	RC_SERVER_URL                            default: the sandbox host
//	RC_TEXT                                  message body
//	RC_OWN_FROM, RC_SHARED_FROM,
//	RC_SHARED_EXTENSION_ID                   override the automatic number pick
//	RC_DRY_RUN=1                             read only, send nothing
//
// Credentials are read from the environment and never printed.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/client/ringcentral"
	"github.com/TMS360/backend-pkg/client/ringcentral/smsprobe"
)

func main() {
	cfg := smsprobe.Config{
		Cred: ringcentral.Cred{
			ClientID:     os.Getenv("RC_CLIENT_ID"),
			ClientSecret: os.Getenv("RC_CLIENT_SECRET"),
			JWT:          os.Getenv("RC_JWT"),
			ServerURL:    envOr("RC_SERVER_URL", ringcentral.SandboxServerURL),
		},
		To:                os.Getenv("RC_TO"),
		Text:              os.Getenv("RC_TEXT"),
		OwnFrom:           os.Getenv("RC_OWN_FROM"),
		SharedFrom:        os.Getenv("RC_SHARED_FROM"),
		SharedExtensionID: os.Getenv("RC_SHARED_EXTENSION_ID"),
		DryRun:            truthy(os.Getenv("RC_DRY_RUN")),
	}

	if err := cfg.Cred.Validate(); err != nil {
		fail("%v — set RC_CLIENT_ID, RC_CLIENT_SECRET and RC_JWT from the RingCentral sandbox app", err)
	}
	if strings.TrimSpace(cfg.To) == "" {
		fail("RC_TO is required: the recipient the probe texts, in E.164 (+15551230000)")
	}
	if cfg.Cred.ServerURL == ringcentral.DefaultServerURL {
		fmt.Fprintln(os.Stderr, "warning: RC_SERVER_URL points at PRODUCTION — this sends real texts from a customer account")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	report, err := smsprobe.Run(ctx, cfg)
	if err != nil {
		fail("%v", err)
	}
	fmt.Print(smsprobe.Render(report))
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y":
		return true
	}
	return false
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "rc-sms-probe: "+format+"\n", args...)
	os.Exit(1)
}
