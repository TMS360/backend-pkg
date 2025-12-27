package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventPayload defines the standard message format for all Kafka events
type EventPayload struct {
	//RequestID     string      `json:"request_id"`
	EventID       uuid.UUID       `json:"event_id"`
	ActorID       uuid.UUID       `json:"actor_id"`
	EntityType    string          `json:"entity_type"` // users, orders, etc.
	EntityID      uuid.UUID       `json:"entity_id"`
	Action        string          `json:"action"`         // created, updated, deleted
	SourceService string          `json:"source_service"` // auth_service, order_service, etc.
	Timestamp     time.Time       `json:"timestamp"`
	Data          json.RawMessage `json:"data,omitempty"`    // {id: 123, name: "John Doe", ...}
	Changes       []Change        `json:"changes,omitempty"` // [{field: "name", old_value: "John", new_value: "John Doe"}, ...]
}

type Change struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}
