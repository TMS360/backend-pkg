package outbox

import (
	"context"
	"errors"

	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	// FetchPendingBatch locks and returns the next batch of events.
	FetchPendingBatch(ctx context.Context, limit int) ([]*model.OutboxEvent, error)
	// DeleteBatch removes processed events by ID.
	DeleteBatch(ctx context.Context, ids []string) error
	// MarkParked moves PENDING events to a parked status, out of the fetch window.
	MarkParked(ctx context.Context, ids []string, status string) error
}

// ParkedRepository is the admin side of parked events.
type ParkedRepository interface {
	// FindParked returns a parked event by ID, or (nil, nil) when there is no
	// parked event with that ID.
	FindParked(ctx context.Context, id string) (*model.OutboxEvent, error)
	// Requeue puts a parked event back to PENDING. False when it was not parked.
	Requeue(ctx context.Context, id string) (bool, error)
	// DeleteParked removes a parked event. False when it was not parked.
	DeleteParked(ctx context.Context, id string) (bool, error)
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
		Where("status = ?", StatusPending).
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

// MarkParked moves PENDING events to a parked status, out of the fetch window.
func (r *repo) MarkParked(ctx context.Context, ids []string, status string) error {
	return r.tm.GetDB(ctx).
		Model(&model.OutboxEvent{}).
		Where("id IN ? AND status = ?", ids, StatusPending).
		Update("status", status).Error
}

// NewParkedRepository returns the admin repository for parked events.
func NewParkedRepository(tm tmsdb.TransactionManager) ParkedRepository {
	return &repo{tm}
}

func (r *repo) FindParked(ctx context.Context, id string) (*model.OutboxEvent, error) {
	var event model.OutboxEvent
	err := r.tm.GetDB(ctx).
		Where("id = ? AND status = ?", id, StatusParkedTooLarge).
		Take(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *repo) Requeue(ctx context.Context, id string) (bool, error) {
	res := r.tm.GetDB(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ?", id, StatusParkedTooLarge).
		Update("status", StatusPending)
	return res.RowsAffected > 0, res.Error
}

func (r *repo) DeleteParked(ctx context.Context, id string) (bool, error) {
	res := r.tm.GetDB(ctx).
		Where("id = ? AND status = ?", id, StatusParkedTooLarge).
		Delete(&model.OutboxEvent{})
	return res.RowsAffected > 0, res.Error
}
