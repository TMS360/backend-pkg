package outbox

import (
	"context"
	"reflect"
	"strings"

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
