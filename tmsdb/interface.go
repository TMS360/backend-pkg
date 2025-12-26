package tmsdb

import (
	"context"

	"github.com/TMS360/backend-pkg/events"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionManager interface {
	// WithTransaction executes the fn function within a transaction.
	// If fn returns an error, Rollback occurs; if nil, Commit occurs.
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	GetDB(ctx context.Context) *gorm.DB
	Publish(ctx context.Context, aggID uuid.UUID, aggType, evtType string, payload *events.EventPayload) error
}
