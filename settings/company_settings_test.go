package settings

import (
	"errors"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// DEV-1577. The empty-miles mode gates every automatic deadhead write in
// backend-load, so the fallback is load-bearing in one direction only: reading a
// deferred tenant as auto writes a mileage they did not ask for (recoverable —
// dispatch edits the origin), while reading an auto tenant as deferred stops
// empty miles from ever being written and stalls their driver pay (not
// recoverable without an operator noticing). Both a miss and a read failure
// therefore degrade to auto, and so does an unrecognized stored value.
func TestEmptyMilesWorkflowFromCache(t *testing.T) {
	cases := []struct {
		name  string
		value string
		err   error
		want  EmptyMilesWorkflow
	}{
		{name: "deferred", value: "deferred", want: EmptyMilesWorkflowDeferred},
		{name: "auto", value: "auto", want: EmptyMilesWorkflowAuto},
		{name: "mixed case and padding still parses", value: "  Deferred ", want: EmptyMilesWorkflowDeferred},
		{name: "cache miss defaults to auto", err: redis.Nil, want: EmptyMilesWorkflowAuto},
		{name: "read failure defaults to auto", err: errors.New("dial tcp: connection refused"), want: EmptyMilesWorkflowAuto},
		{name: "empty value defaults to auto", value: "", want: EmptyMilesWorkflowAuto},
		{name: "unrecognized value defaults to auto", value: "manual", want: EmptyMilesWorkflowAuto},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, emptyMilesWorkflowFromCache(tc.value, tc.err))
		})
	}
}

// The predicate the write paths branch on: only an explicitly stored "deferred"
// suppresses automatic writes. Anything else — including the zero value of the
// type, which is what a forgotten initialization would produce — leaves the
// historical behaviour in place.
func TestEmptyMilesWorkflow_IsDeferred(t *testing.T) {
	require.True(t, EmptyMilesWorkflowDeferred.IsDeferred())
	require.False(t, EmptyMilesWorkflowAuto.IsDeferred())
	require.False(t, DefaultEmptyMilesWorkflow.IsDeferred())
	require.False(t, EmptyMilesWorkflow("").IsDeferred())
}

func TestEmptyMilesWorkflow_IsValid(t *testing.T) {
	require.True(t, EmptyMilesWorkflowDeferred.IsValid())
	require.True(t, EmptyMilesWorkflowAuto.IsValid())
	require.False(t, EmptyMilesWorkflow("").IsValid())
	require.False(t, EmptyMilesWorkflow("Deferred").IsValid(), "IsValid is exact; normalization happens before it")
}

// The wire values are the contract shared with tms-auth's settingRules
// ("oneof=deferred auto") and with the stored rows. Renaming either constant
// silently desyncs validation from every reader.
func TestEmptyMilesWorkflow_WireValues(t *testing.T) {
	require.Equal(t, "deferred", EmptyMilesWorkflowDeferred.String())
	require.Equal(t, "auto", EmptyMilesWorkflowAuto.String())
}
