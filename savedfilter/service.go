package savedfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/response"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"github.com/google/uuid"
)

const maxFiltersPerUser = 10

var (
	ErrMaxFiltersReached = errors.New("maximum number of saved filters reached")
	ErrAccessDenied      = errors.New("access denied")
)

// rejection is a user-facing PublicError that still matches its package
// sentinel via errors.Is, so callers that branch on ErrMaxFiltersReached or
// ErrAccessDenied keep working while the GraphQL presenter treats the error as
// a 4xx rejection (Sentry warning, real message) instead of a 500.
type rejection struct {
	response.PublicError
	sentinel error
}

func (e *rejection) Unwrap() []error { return []error{e.PublicError, e.sentinel} }

func errMaxFiltersReached(entity string) error {
	return &rejection{
		PublicError: response.NewCodedError(
			"SAVED_FILTER_LIMIT_REACHED",
			fmt.Sprintf("%s: entity=%s limit=%d", ErrMaxFiltersReached.Error(), entity, maxFiltersPerUser),
			fmt.Sprintf("You can save up to %d filters per list. Delete one to save a new filter.", maxFiltersPerUser),
			http.StatusUnprocessableEntity,
			map[string]any{"limit": maxFiltersPerUser, "entity": entity},
		),
		sentinel: ErrMaxFiltersReached,
	}
}

func errAccessDenied(reason string) error {
	return &rejection{
		PublicError: response.NewForbidden(
			fmt.Sprintf("saved filter %s: %s", ErrAccessDenied.Error(), reason),
			"You do not have access to this saved filter.",
		),
		sentinel: ErrAccessDenied,
	}
}

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

// NewCountOnlyService creates a Service that only supports counting entities.
// Used by services that extend SavedFilter via Federation without owning the data.
func NewCountOnlyService() *Service {
	return &Service{
		countFuncs: make(map[string]CountFunc),
	}
}

// RegisterCountFunc registers a callback that counts entities for a given entity type.
// Entity type matching is case-insensitive — keys are normalized to lowercase
// on both registration and lookup, so callers may pass "shipment", "Shipment",
// or "SHIPMENT" interchangeably.
func (s *Service) RegisterCountFunc(entityType string, fn CountFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countFuncs[strings.ToLower(entityType)] = fn
}

func (s *Service) getCountFunc(entityType string) CountFunc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countFuncs[strings.ToLower(entityType)]
}

// Count returns the count of entities matching the given filter for the specified entity type.
func (s *Service) Count(ctx context.Context, entityType string, filter json.RawMessage) (int64, error) {
	countFn := s.getCountFunc(entityType)
	if countFn == nil {
		return 0, nil
	}
	return countFn(ctx, filter)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*SavedFilter, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil || actor == nil || actor.Claims == nil {
		return nil, errAccessDenied("no authenticated actor")
	}

	count, err := s.repo.CountByUserAndEntity(ctx, actor.Claims.UserID, input.Entity)
	if err != nil {
		return nil, err
	}
	if count >= maxFiltersPerUser {
		return nil, errMaxFiltersReached(input.Entity)
	}

	filter := &SavedFilter{
		UserID: actor.Claims.UserID,
		Entity: input.Entity,
		Name:   input.Name,
		Filter: model.JSONRaw(input.Filter),
		View:   model.JSONRaw(input.View),
		Pinned: true,
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
		filter.Filter = model.JSONRaw(*input.Filter)
	}
	if input.View != nil {
		filter.View = model.JSONRaw(*input.View)
	}
	if input.Pinned != nil {
		filter.Pinned = *input.Pinned
	}

	filter.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, filter); err != nil {
		return nil, err
	}
	return filter, nil
}

func (s *Service) SetPinned(ctx context.Context, id uuid.UUID, pinned bool) (*SavedFilter, error) {
	filter, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.checkOwnership(ctx, filter); err != nil {
		return nil, err
	}

	if filter.Pinned == pinned {
		return filter, nil
	}

	filter.Pinned = pinned
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
	if err != nil || actor == nil || actor.Claims == nil {
		return nil, errAccessDenied("no authenticated actor")
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
			count, err = countFn(ctx, json.RawMessage(f.Filter))
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
	if countFn := s.getCountFunc(filter.Entity); countFn != nil {
		count, err = countFn(ctx, json.RawMessage(filter.Filter))
		if err != nil {
			return nil, err
		}
	}

	return &SavedFilterWithCount{
		SavedFilter: filter,
		Count:       count,
	}, nil
}

func (s *Service) checkOwnership(ctx context.Context, filter *SavedFilter) error {
	actor, err := middleware.GetActor(ctx)
	if err != nil || actor == nil || actor.Claims == nil {
		return errAccessDenied("no authenticated actor")
	}
	if filter.UserID != actor.Claims.UserID {
		return errAccessDenied("filter owned by another user")
	}
	return nil
}
