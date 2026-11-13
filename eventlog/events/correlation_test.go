package events

import (
	"encoding/json"
	"strings"
	"testing"
)

// RMS reads the request id from the event under this exact name; renaming the
// tag would silently break the decision -> fire -> result chain.
func TestEventPayloadCarriesCorrelationID(t *testing.T) {
	b, err := json.Marshal(EventPayload{CorrelationID: "req-7"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"correlation_id":"req-7"`) {
		t.Fatalf("correlation_id missing on the wire: %s", b)
	}
	b, _ = json.Marshal(EventPayload{})
	if strings.Contains(string(b), "correlation_id") {
		t.Fatalf("empty id must be omitted: %s", b)
	}
}
