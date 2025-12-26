package tmsdb

import (
	"context"
	"encoding/json"
	"time"

	"github.com/TMS360/backend-pkg/events"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ctxTransactionKey struct{}

// GormTransactionManager TransactionManager implementation for GORM
type GormTransactionManager struct {
	db *gorm.DB
}

func NewGormTransactionManager(db *gorm.DB) *GormTransactionManager {
	return &GormTransactionManager{db: db}
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
		return tx
	}
	return m.db
}

// Publish implements the logic DIRECTLY here. No Repo.
func (m *GormTransactionManager) Publish(ctx context.Context, aggID uuid.UUID, aggType, evtType string, payload *events.EventPayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	event := &OutboxEvent{
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
