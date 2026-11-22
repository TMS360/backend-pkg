package savedfilter

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/response"
	"github.com/google/uuid"
)

// fakeRepo embeds the Repository interface so only the methods exercised by
// these tests need implementing; anything else panics on a nil receiver.
type fakeRepo struct {
	Repository
	count   int64
	created bool
}

func (f *fakeRepo) CountByUserAndEntity(context.Context, uuid.UUID, string) (int64, error) {
	return f.count, nil
}

func (f *fakeRepo) Create(_ context.Context, _ *SavedFilter) error {
	f.created = true
	return nil
}

func userCtx() context.Context {
	return middleware.WithActor(context.Background(), &consts.Actor{
		Claims: &consts.UserClaims{UserID: uuid.New()},
	})
}

func TestCreate_LimitReachedIsPublic422(t *testing.T) {
	repo := &fakeRepo{count: maxFiltersPerUser}
	_, err := NewService(repo).Create(userCtx(), CreateInput{Entity: "shipment", Name: "x"})
	if err == nil {
		t.Fatal("expected error at limit")
	}
	if repo.created {
		t.Fatal("filter must not be created at limit")
	}
	if !errors.Is(err, ErrMaxFiltersReached) {
		t.Fatalf("errors.Is(err, ErrMaxFiltersReached) = false; err=%v", err)
	}
	var pub response.PublicError
	if !errors.As(err, &pub) {
		t.Fatalf("expected response.PublicError, got %T", err)
	}
	if pub.ErrorStatus() != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", pub.ErrorStatus())
	}
	var coded response.CodedError
	if !errors.As(err, &coded) || coded.ErrorCodeString() != "SAVED_FILTER_LIMIT_REACHED" {
		t.Fatalf("expected code SAVED_FILTER_LIMIT_REACHED, got %v", err)
	}
	if response.IsUnreported(err) {
		t.Fatal("limit rejection must still be reported (as a warning)")
	}
}

func TestCreate_BelowLimitSucceeds(t *testing.T) {
	repo := &fakeRepo{count: maxFiltersPerUser - 1}
	if _, err := NewService(repo).Create(userCtx(), CreateInput{Entity: "shipment", Name: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.created {
		t.Fatal("expected filter to be created")
	}
}

func TestCreate_NoActorIsPublic403(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"missing": context.Background(),
		"nil":     middleware.WithActor(context.Background(), nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewService(&fakeRepo{}).Create(ctx, CreateInput{Entity: "shipment"})
			if !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("errors.Is(err, ErrAccessDenied) = false; err=%v", err)
			}
			var pub response.PublicError
			if !errors.As(err, &pub) || pub.ErrorStatus() != http.StatusForbidden {
				t.Fatalf("expected 403 PublicError, got %v", err)
			}
		})
	}
}

func TestCheckOwnership_OtherUserIsPublic403(t *testing.T) {
	err := NewService(&fakeRepo{}).checkOwnership(userCtx(), &SavedFilter{UserID: uuid.New()})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("errors.Is(err, ErrAccessDenied) = false; err=%v", err)
	}
	var pub response.PublicError
	if !errors.As(err, &pub) || pub.ErrorStatus() != http.StatusForbidden {
		t.Fatalf("expected 403 PublicError, got %v", err)
	}
}
