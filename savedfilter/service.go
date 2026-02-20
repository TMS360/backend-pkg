package savedfilter

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
)

const maxFiltersPerUser = 10

var (
	ErrMaxFiltersReached = errors.New("maximum number of saved filters reached")
	ErrAccessDenied      = errors.New("access denied")
)

type Service struct {
	repo       Repository
	countFuncs map[string]CountFunc
	mu         sync.RWMutex
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:       repo,
		countFuncs: make(map[string]CountFunc),
	}
}

// RegisterCountFunc registers a callback that counts entities for a given entity type.
func (s *Service) RegisterCountFunc(entityType string, fn CountFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countFuncs[entityType] = fn
}

func (s *Service) getCountFunc(entityType string) CountFunc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countFuncs[entityType]
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*SavedFilter, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, ErrAccessDenied
	}

	count, err := s.repo.CountByUserAndEntity(ctx, actor.Claims.UserID, input.EntityType)
	if err != nil {
		return nil, err
	}
	if count >= maxFiltersPerUser {
		return nil, ErrMaxFiltersReached
	}

	filter := &SavedFilter{
		UserID:     actor.Claims.UserID,
		EntityType: input.EntityType,
		Name:       input.Name,
		Filter:     input.Filter,
	}

	if input.IsDefault != nil && *input.IsDefault {
		if err := s.repo.ClearDefault(ctx, actor.Claims.UserID, input.EntityType); err != nil {
			return nil, err
		}
		filter.IsDefault = true
	}

	if err := s.repo.Create(ctx, filter); err != nil {
		return nil, err
	}
	return filter, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (*SavedFilter, error) {
	filter, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.checkOwnership(ctx, filter); err != nil {
		return nil, err
	}

	if input.Name != nil {
		filter.Name = *input.Name
	}
	if input.Filter != nil {
		filter.Filter = *input.Filter
	}
	if input.IsDefault != nil {
		if *input.IsDefault && !filter.IsDefault {
			actor, _ := middleware.GetActor(ctx)
			if err := s.repo.ClearDefault(ctx, actor.Claims.UserID, filter.EntityType); err != nil {
				return nil, err
			}
		}
		filter.IsDefault = *input.IsDefault
	}

	filter.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, filter); err != nil {
		return nil, err
	}
	return filter, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	filter, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.checkOwnership(ctx, filter); err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context, entityType string) ([]*SavedFilterWithCount, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, ErrAccessDenied
	}

	filters, err := s.repo.ListByUserAndEntity(ctx, actor.Claims.UserID, entityType)
	if err != nil {
		return nil, err
	}

	result := make([]*SavedFilterWithCount, len(filters))
	countFn := s.getCountFunc(entityType)

	for i, f := range filters {
		var count int64
		if countFn != nil {
			count, err = countFn(ctx, f.Filter)
			if err != nil {
				return nil, err
			}
		}
		result[i] = &SavedFilterWithCount{
			SavedFilter: f,
			Count:       count,
		}
	}

	return result, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*SavedFilterWithCount, error) {
	filter, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.checkOwnership(ctx, filter); err != nil {
		return nil, err
	}

	var count int64
	if countFn := s.getCountFunc(filter.EntityType); countFn != nil {
		count, err = countFn(ctx, filter.Filter)
		if err != nil {
			return nil, err
		}
	}

	return &SavedFilterWithCount{
		SavedFilter: filter,
		Count:       count,
	}, nil
}

func (s *Service) SetDefault(ctx context.Context, id uuid.UUID) (*SavedFilter, error) {
	filter, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.checkOwnership(ctx, filter); err != nil {
		return nil, err
	}

	actor, _ := middleware.GetActor(ctx)
	if err := s.repo.ClearDefault(ctx, actor.Claims.UserID, filter.EntityType); err != nil {
		return nil, err
	}

	filter.IsDefault = true
	filter.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, filter); err != nil {
		return nil, err
	}

	return filter, nil
}

func (s *Service) GetDefault(ctx context.Context, entityType string) (*SavedFilter, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, ErrAccessDenied
	}

	return s.repo.GetDefault(ctx, actor.Claims.UserID, entityType)
}

func (s *Service) checkOwnership(ctx context.Context, filter *SavedFilter) error {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return ErrAccessDenied
	}
	if filter.UserID != actor.Claims.UserID {
		return ErrAccessDenied
	}
	return nil
}
