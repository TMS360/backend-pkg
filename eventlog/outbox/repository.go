package outbox

import (
	"context"
	"reflect"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"gorm.io/gorm/clause"
)

type Repository interface {
	// FetchPendingBatch locks and returns the next batch of events.
	FetchPendingBatch(ctx context.Context, limit int) ([]*model.OutboxEvent, error)
	// DeleteBatch removes processed events by ID.
	DeleteBatch(ctx context.Context, ids []string) error
}

type repo struct {
	tm tmsdb.TransactionManager
}

func NewOutboxEventRepository(tm tmsdb.TransactionManager) Repository {
	return &repo{tm}
}

// FetchPendingBatch locks and returns the next batch of events.
func (r *repo) FetchPendingBatch(ctx context.Context, limit int) ([]*model.OutboxEvent, error) {
	var eventsList []*model.OutboxEvent

	err := r.tm.GetDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ?", "PENDING").
		Order("created_at ASC").
		Limit(limit).
		Find(&eventsList).Error

	return eventsList, err
}

// DeleteBatch removes processed events by ID.
func (r *repo) DeleteBatch(ctx context.Context, ids []string) error {
	return r.tm.GetDB(ctx).
		Where("id IN ?", ids).
		Delete(&model.OutboxEvent{}).Error
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
