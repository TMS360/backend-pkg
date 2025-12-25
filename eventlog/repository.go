package eventlog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TMS360/backend-pkg/tmsdb"
	"gorm.io/gorm/clause"
)

type OutboxEventRepository interface {
	// FetchPendingBatch locks and returns the next batch of events.
	FetchPendingBatch(ctx context.Context, limit int) ([]OutboxEvent, error)
	// DeleteBatch removes processed events by ID.
	DeleteBatch(ctx context.Context, ids []string) error
	// CreateEvent writes the event to the DB
	CreateEvent(ctx context.Context, topic string, payload EventPayload) error
}

type outboxEventRepo struct {
	tm tmsdb.TransactionManager
}

func NewOutboxEventRepository(tm tmsdb.TransactionManager) OutboxEventRepository {
	return &outboxEventRepo{tm}
}

// FetchPendingBatch locks and returns the next batch of events.
func (r *outboxEventRepo) FetchPendingBatch(ctx context.Context, limit int) ([]OutboxEvent, error) {
	var events []OutboxEvent

	err := r.tm.GetDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ?", "PENDING").
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error

	return events, err
}

// DeleteBatch removes processed events by ID.
func (r *outboxEventRepo) DeleteBatch(ctx context.Context, ids []string) error {
	return r.tm.GetDB(ctx).
		Where("id IN ?", ids).
		Delete(&OutboxEvent{}).Error
}

// CreateEvent writes the event to the DB
func (r *outboxEventRepo) CreateEvent(ctx context.Context, topic string, payload EventPayload) error {
	// 1. Marshal the payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	// 2. Prepare the model
	event := OutboxEvent{
		AggregateID:   payload.EntityID,
		AggregateType: payload.EntityType,
		EventType:     payload.Action,
		Payload:       payloadBytes,
		Topic:         topic,
		Status:        "PENDING",
	}

	// 3. Create using the passed transaction
	// WithContext ensures we respect cancellations/timeouts
	if err := r.tm.GetDB(ctx).WithContext(ctx).Create(&event).Error; err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}

	return nil
}
