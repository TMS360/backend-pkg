package tmsdb

import (
	"context"
	"encoding/json"
	"time"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ctxTransactionKey struct{}

// GormTransactionManager TransactionManager implementation for GORM
type GormTransactionManager struct {
	db            *gorm.DB
	sourceService string
}

func NewGormTransactionManager(db *gorm.DB, sourceService string) *GormTransactionManager {
	return &GormTransactionManager{db, sourceService}
}

// WithTransaction implement interface service.TransactionManager
func (m *GormTransactionManager) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, ctxTransactionKey{}, tx)
		return fn(txCtx)
	})
}

// GetDB извлекает транзакцию или возвращает fallback DB.
func (m *GormTransactionManager) GetDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(ctxTransactionKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return m.db.WithContext(ctx)
}

// Publish implements the logic DIRECTLY here. No Repo.
func (m *GormTransactionManager) Publish(ctx context.Context, aggType, evtType string, aggID uuid.UUID, data interface{}) error {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return err
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	eventPayload := events.EventPayload{
		SourceService: m.sourceService,
		EventID:       uuid.New(),
		ActorID:       actor.ID,
		EntityType:    aggType,
		EntityID:      aggID,
		Action:        evtType,
		Data:          json.RawMessage(dataBytes),
		Timestamp:     time.Now(),
	}

	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return err
	}

	event := &model.OutboxEvent{
		ID:            uuid.New(),
		AggregateID:   aggID,
		AggregateType: aggType,
		EventType:     evtType,
		Payload:       payloadBytes,
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Uses the active transaction from context automatically
	return m.GetDB(ctx).Create(event).Error
}
