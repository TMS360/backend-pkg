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
	ActorID       *uuid.UUID      `json:"actor_id"`
	CompanyID     *uuid.UUID      `json:"company_id"`
	EntityType    string          `json:"entity_type"` // users, orders, etc.
	EntityID      uuid.UUID       `json:"entity_id"`
	Action        string          `json:"action"`         // created, updated, deleted
	SourceService string          `json:"source_service"` // auth_service, order_service, etc.
	Timestamp     time.Time       `json:"timestamp"`
	Data          json.RawMessage `json:"data,omitempty"`    // {id: 123, name: "John Doe", ...}
	Changes       []Change        `json:"changes,omitempty"` // [{field: "name", old_value: "John", new_value: "John Doe"}, ...]

	// ActorIP and UserAgent record where the action came from. Stamped once by
	// tmsdb.writeEvent from the request context (middleware.ClientOrigin), so
	// producers never pass them explicitly. Both are omitted for events with no
	// HTTP request behind them — a Kafka-consumer or cron-triggered change
	// records nothing rather than the server's own address. UserAgent is
	// truncated to middleware.MaxUserAgentLen at capture time.
	ActorIP   *string `json:"actor_ip,omitempty"`
	UserAgent *string `json:"user_agent,omitempty"`

	// Reason is an optional human-supplied justification for the action (e.g. why a
	// dispatcher cleared a check-in). Most events omit it (nil); only actions that
	// require an explanation set it via EventBuilder.WithReason. Surfaced by
	// backend-audit as a nullable column so any entity can carry a reason.
	Reason *string `json:"reason,omitempty"`

	// Root entity context lets aggregate audit queries (e.g. "all activity for shipment X")
	// fan in nested events without a cross-service lookup at read time. When unset on the
	// wire, the consumer falls back to LeafToRoot[EntityType] / EntityID so legacy producers
	// keep working.
	RootEntityType string    `json:"root_entity_type,omitempty"`
	RootEntityID   uuid.UUID `json:"root_entity_id,omitempty"`

	// Sensitivity classifies what KIND of information this event carries so the
	// audit read path can seal events a reader may not see. Assigned by the
	// producing service at emit time (tmsdb.writeEvent auto-classifies via
	// events.Classify unless the producer set it with WithSensitivity). Empty on
	// the wire means unclassified — the read side treats that as the most
	// restrictive class and flags it, never as "open".
	Sensitivity Sensitivity `json:"sensitivity,omitempty"`

	// Participants are the entities that took part in the action and the role
	// each played (ACTOR / SUBJECT / ASSIGNED / MENTIONED / AFFECTED). The caller is stamped
	// as ACTOR automatically; producers add the rest — e.g. a trip assignment
	// expands the crew to its drivers (SUBJECT) plus the crew (ASSIGNED). This is
	// the join key that lets a driver page fan in a dispatch, a pay statement and
	// a reported issue about the same person from three different services.
	Participants []Participant `json:"participants,omitempty"`
}

type Change struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}
