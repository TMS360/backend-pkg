package fmcsa

import "testing"

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// TestVerificationUnavailable pins the distinction that fixes the false-reject:
// only a nil operating status WITH live data explicitly unavailable counts as
// "couldn't verify" — everything else stays a normal validity decision.
func TestVerificationUnavailable(t *testing.T) {
	cases := []struct {
		name      string
		opStatus  *string
		liveAvail *bool
		want      bool
	}{
		{"live-unavailable + nil status (the incident)", nil, boolPtr(false), true},
		{"live-available + nil status (genuine no-auth)", nil, boolPtr(true), false},
		{"live-unknown + nil status (unchanged reject)", nil, nil, false},
		{"live-unavailable but status present", strPtr("AUTHORIZED FOR HIRE"), boolPtr(false), false},
		{"authorized + live-available", strPtr("AUTHORIZED FOR HIRE"), boolPtr(true), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Result{OperatingStatus: c.opStatus, LiveDataAvailable: c.liveAvail}
			if got := r.VerificationUnavailable(); got != c.want {
				t.Fatalf("VerificationUnavailable() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestIsValidUnchanged guards that the fix did not alter the existing authority
// checks — genuine rejects and passes must behave exactly as before.
func TestIsValidUnchanged(t *testing.T) {
	cases := []struct {
		name     string
		opStatus *string
		allowed  *bool
		want     bool
	}{
		{"authorized", strPtr("AUTHORIZED FOR HIRE"), nil, true},
		{"not authorized", strPtr("NOT AUTHORIZED"), nil, false},
		{"nil operating status", nil, nil, false},
		{"authorized but not allowed to operate", strPtr("AUTHORIZED FOR HIRE"), boolPtr(false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Result{OperatingStatus: c.opStatus, AllowedToOperate: c.allowed}
			if got := r.IsValid(); got != c.want {
				t.Fatalf("IsValid() = %v, want %v", got, c.want)
			}
		})
	}
}
