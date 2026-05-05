package tmsdb

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionManager interface {
	// WithTransaction executes the fn function within a transaction.
	// If fn returns an error, Rollback occurs; if nil, Commit occurs.
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	GetDB(ctx context.Context) *gorm.DB
	// Publish is the legacy shorthand: emits an event whose root_entity defaults
	// to itself (root_entity_type = aggType, root_entity_id = aggID). Use Event(...)
	// when the event is a nested leaf that should roll up to a parent aggregate
	// (e.g. trip_events → shipments).
	Publish(ctx context.Context, aggType, evtType string, aggID uuid.UUID, data interface{}, oldData ...interface{}) error
	// Event opens a fluent builder for emitting an event. Chain WithRoot / WithData /
	// WithOldData and finish with .Publish(ctx).
	Event(aggType, evtType string, aggID uuid.UUID) *EventBuilder
	Filter(ctx context.Context, model interface{}) *FilterBuilder
}
