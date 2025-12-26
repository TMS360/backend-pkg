package events

import (
	"time"

	"github.com/google/uuid"
)

// EventPayload defines the standard message format for all Kafka events
type EventPayload struct {
	EventID       uuid.UUID   `json:"event_id"`
	RequestID     string      `json:"request_id"`
	ActorID       uuid.UUID   `json:"actor_id"`
	ActorRole     string      `json:"actor_role"`
	EntityType    string      `json:"entity_type"` // TEAM, LOAD, TRIP
	EntityID      uuid.UUID   `json:"entity_id"`
	Action        string      `json:"action"` // CREATE, UPDATE, DELETE
	SourceService string      `json:"source_service"`
	Timestamp     time.Time   `json:"timestamp"`
	Data          interface{} `json:"data,omitempty"`
	Changes       []Change    `json:"changes,omitempty"`
}

type Change struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}
