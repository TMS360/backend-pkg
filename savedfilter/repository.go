package savedfilter

import (
	"context"

	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/google/uuid"
)

type Repository interface {
	tmsdb.BaseRepository[SavedFilter]
	ListByUserAndEntity(ctx context.Context, userID uuid.UUID, entityType string) ([]*SavedFilter, error)
	CountByUserAndEntity(ctx context.Context, userID uuid.UUID, entityType string) (int64, error)
}

type repository struct {
	tmsdb.BaseRepository[SavedFilter]
}

func NewRepository(tm tmsdb.TransactionManager) Repository {
	return &repository{
		BaseRepository: tmsdb.NewBaseRepository[SavedFilter](tm),
	}
}

func (r *repository) ListByUserAndEntity(ctx context.Context, userID uuid.UUID, entityType string) ([]*SavedFilter, error) {
	var filters []*SavedFilter
	err := r.TM().GetDB(ctx).
		Where("user_id = ? AND entity_type = ?", userID, entityType).
		Order("created_at DESC").
		Find(&filters).Error
	if err != nil {
		return nil, err
	}
	return filters, nil
}

func (r *repository) CountByUserAndEntity(ctx context.Context, userID uuid.UUID, entityType string) (int64, error) {
	var count int64
	err := r.TM().GetDB(ctx).
		Model(&SavedFilter{}).
		Where("user_id = ? AND entity_type = ?", userID, entityType).
		Count(&count).Error
	return count, err
}
