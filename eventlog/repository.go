package eventlog

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/TMS360/backend-pkg/events"
	"github.com/TMS360/backend-pkg/tmsdb"
	"gorm.io/gorm/clause"
)

type OutboxEventRepository interface {
	// FetchPendingBatch locks and returns the next batch of events.
	FetchPendingBatch(ctx context.Context, limit int) ([]*OutboxEvent, error)
	// DeleteBatch removes processed events by ID.
	DeleteBatch(ctx context.Context, ids []string) error
	// CreateEvent writes the event to the DB
	CreateEvent(ctx context.Context, topic string, payload *events.EventPayload) error
}

type outboxEventRepo struct {
	tm tmsdb.TransactionManager
}

func NewOutboxEventRepository(tm tmsdb.TransactionManager) OutboxEventRepository {
	return &outboxEventRepo{tm}
}

// FetchPendingBatch locks and returns the next batch of events.
func (r *outboxEventRepo) FetchPendingBatch(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	var eventsList []*OutboxEvent

	err := r.tm.GetDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ?", "PENDING").
		Order("created_at ASC").
		Limit(limit).
		Find(&eventsList).Error

	return eventsList, err
}

// DeleteBatch removes processed events by ID.
func (r *outboxEventRepo) DeleteBatch(ctx context.Context, ids []string) error {
	return r.tm.GetDB(ctx).
		Where("id IN ?", ids).
		Delete(&OutboxEvent{}).Error
}

// CreateEvent writes the event to the DB
func (r *outboxEventRepo) CreateEvent(ctx context.Context, topic string, payload *events.EventPayload) error {
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

// CalculateChanges compares two structs and returns a list of changes.
func CalculateChanges(oldVal, newVal interface{}) []events.Change {
	var changes []events.Change

	vOld := reflect.ValueOf(oldVal).Elem()
	vNew := reflect.ValueOf(newVal).Elem()
	typeOf := vOld.Type()

	for i := 0; i < vOld.NumField(); i++ {
		field := typeOf.Field(i)

		// Skip unexported fields or fields tagged to be ignored
		if field.PkgPath != "" {
			continue
		}

		valOld := vOld.Field(i).Interface()
		valNew := vNew.Field(i).Interface()

		if !reflect.DeepEqual(valOld, valNew) {
			changes = append(changes, events.Change{
				Field:    field.Name, // Or use field.Tag.Get("json")
				OldValue: valOld,
				NewValue: valNew,
			})
		}
	}

	return changes
}
