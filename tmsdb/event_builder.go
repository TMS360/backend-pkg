package tmsdb

import (
	"context"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/google/uuid"
)

// EventBuilder is the fluent surface for emitting an outbox event. It is
// constructed via TransactionManager.Event(aggType, evtType, aggID) and
// terminated with Publish(ctx).
//
// Typical usage for a nested leaf that rolls up to a parent aggregate:
//
//	tm.Event("trip_events", "drivers_changed", trip.ID).
//	    WithRoot(events.RootShipment, trip.ShipmentID).
//	    WithData(payload).
//	    Publish(ctx)
//
// For self-rooted events the legacy shorthand tm.Publish(...) is equivalent
// and shorter.
type EventBuilder struct {
	tm       *GormTransactionManager
	aggType  string
	evtType  string
	aggID    uuid.UUID
	rootType string
	rootID   uuid.UUID
	topic    string
	data     interface{}
	oldData  interface{}
}

// WithRoot attaches aggregate-root context so the event is discoverable via
// aggregate-root audit queries (e.g. all activity for a shipment).
func (b *EventBuilder) WithRoot(rootType events.RootEntity, rootID uuid.UUID) *EventBuilder {
	b.rootType = string(rootType)
	b.rootID = rootID
	return b
}

// WithTopic overrides the Kafka topic this event publishes to. By default the
// relay uses EntityType as the topic. Use WithTopic when a child entity type
// should route onto its parent's topic (e.g. customer_comments published onto
// the "customers" topic while keeping entity_type = "customer_comments" for
// downstream filtering). The entity_type stays distinct, only the Kafka
// destination changes.
func (b *EventBuilder) WithTopic(topic string) *EventBuilder {
	b.topic = topic
	return b
}

// WithData sets the event payload.
func (b *EventBuilder) WithData(data interface{}) *EventBuilder {
	b.data = data
	return b
}

// WithOldData supplies the pre-change snapshot so the platform can compute a
// field-level Changes diff. Pass the original struct (not a pointer to it is
// fine; both pointers and values are accepted via reflection).
func (b *EventBuilder) WithOldData(oldData interface{}) *EventBuilder {
	b.oldData = oldData
	return b
}

// Publish writes the event to the outbox in the current transaction (if any).
func (b *EventBuilder) Publish(ctx context.Context) error {
	return b.tm.writeEvent(ctx, b)
}
