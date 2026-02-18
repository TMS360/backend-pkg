package tmsdb

import (
	"context"
	"errors"

	"github.com/TMS360/backend-pkg/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a record is not found.
var ErrNotFound = errors.New("record not found")

// BaseRepository определяет стандартные CRUD операции и query helpers для любой сущности.
type BaseRepository[T any] interface {
	// CRUD
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, entity *T) error
	UpdateFields(ctx context.Context, id uuid.UUID, input any) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*T, error)

	// Query helpers
	GetByColumn(ctx context.Context, column string, value any) ([]*T, error)
	GetFirstByColumn(ctx context.Context, column string, value any) (*T, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*T, error)

	// FilterBuilder
	Filter(ctx context.Context) *FilterBuilder

	// List с пагинацией — принимает callback для применения фильтров
	List(ctx context.Context, applyFilters func(*FilterBuilder), pagination *PaginationInput) ([]*T, *Pagination, error)

	// Count — подсчёт записей с фильтрами без загрузки данных
	Count(ctx context.Context, applyFilters func(*FilterBuilder)) (int64, error)

	// Доступ к TransactionManager для кастомных запросов
	TM() TransactionManager
}

type gormBaseRepository[T any] struct {
	tm TransactionManager
}

// NewBaseRepository создает новый экземпляр базового репозитория.
func NewBaseRepository[T any](tm TransactionManager) BaseRepository[T] {
	return &gormBaseRepository[T]{tm: tm}
}

func (r *gormBaseRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.tm.GetDB(ctx).Create(entity).Error
}

func (r *gormBaseRepository[T]) Update(ctx context.Context, entity *T) error {
	return r.tm.GetDB(ctx).Save(entity).Error
}

func (r *gormBaseRepository[T]) UpdateFields(ctx context.Context, id uuid.UUID, input any) error {
	updates := utils.StructToMap(input)
	if len(updates) == 0 {
		return nil
	}
	var model T
	result := r.tm.GetDB(ctx).Model(&model).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormBaseRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	var model T
	result := r.tm.GetDB(ctx).Delete(&model, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormBaseRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	var entity T
	err := r.tm.GetDB(ctx).First(&entity, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &entity, nil
}

func (r *gormBaseRepository[T]) GetByColumn(ctx context.Context, column string, value any) ([]*T, error) {
	var entities []*T
	err := r.tm.GetDB(ctx).Where(column+" = ?", value).Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

func (r *gormBaseRepository[T]) GetFirstByColumn(ctx context.Context, column string, value any) (*T, error) {
	var entity T
	err := r.tm.GetDB(ctx).Where(column+" = ?", value).First(&entity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &entity, nil
}

func (r *gormBaseRepository[T]) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*T, error) {
	if len(ids) == 0 {
		return []*T{}, nil
	}
	var entities []*T
	err := r.tm.GetDB(ctx).Where("id IN ?", ids).Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// Filter возвращает FilterBuilder, привязанный к текущей транзакции и модели T.
func (r *gormBaseRepository[T]) Filter(ctx context.Context) *FilterBuilder {
	var model T
	return r.tm.Filter(ctx, &model)
}

func (r *gormBaseRepository[T]) List(ctx context.Context, applyFilters func(*FilterBuilder), pagination *PaginationInput) ([]*T, *Pagination, error) {
	var entities []*T
	fb := r.Filter(ctx)
	if applyFilters != nil {
		applyFilters(fb)
	}
	pag, err := fb.FindWithCount(&entities, pagination)
	if err != nil {
		return nil, nil, err
	}
	return entities, pag, nil
}

func (r *gormBaseRepository[T]) Count(ctx context.Context, applyFilters func(*FilterBuilder)) (int64, error) {
	fb := r.Filter(ctx)
	if applyFilters != nil {
		applyFilters(fb)
	}
	return fb.Count()
}

func (r *gormBaseRepository[T]) TM() TransactionManager {
	return r.tm
}
