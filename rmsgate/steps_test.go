package rmsgate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequiredSteps_FormatParseRoundtrip(t *testing.T) {
	cases := []struct {
		kind, arg string
		encoded   string
	}{
		{StepManualApproval, "", "manual_approval"},
		{StepOverrideMarker, "weight_exceeded", "override:weight_exceeded"},
		{StepPostMessage, "welcome_template", "post_message:welcome_template"},
		{StepNotify, "sms:dispatch", "notify:sms:dispatch"}, // arg сам содержит ':'
	}
	for _, c := range cases {
		t.Run(c.encoded, func(t *testing.T) {
			got := FormatStep(c.kind, c.arg)
			assert.Equal(t, c.encoded, got)
			step := ParseStep(got)
			assert.Equal(t, c.kind, step.Kind)
			assert.Equal(t, c.arg, step.Arg)
		})
	}
}

func TestRequiredSteps_ParseSteps(t *testing.T) {
	steps := ParseSteps([]string{"manual_approval", " override:credit ", "", "post_message:po_copy"})
	assert.Equal(t, []Step{
		{Kind: "manual_approval", Arg: ""},
		{Kind: "override", Arg: "credit"},
		{Kind: "post_message", Arg: "po_copy"},
	}, steps)
}
