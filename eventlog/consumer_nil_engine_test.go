package eventlog

import (
	"context"
	"testing"

	"github.com/TMS360/backend-pkg/eventlog/events"
)

// A nil engine must not panic and must still run the system handlers.
func TestDispatch_NilEngineRunsSystemHandlers(t *testing.T) {
	ran := 0
	c := &Consumer{systemHandlers: map[string][]SystemHandlerFunc{
		"loads.created": {func(context.Context, events.EventPayload) error { ran++; return nil }},
	}}
	for _, ev := range []events.EventPayload{
		{EntityType: "loads", Action: "created"},
		{EntityType: "loads", Action: "deleted"},
	} {
		if err := c.dispatch(context.Background(), ev); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	if ran != 1 {
		t.Fatalf("system handler ran %d times, want 1", ran)
	}
}
