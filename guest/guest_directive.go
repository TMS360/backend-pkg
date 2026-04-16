package guest

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
)

// AuthGuest implements the @authGuest(resource: "shipment") directive.
//
//   - If a real (non-guest) user is on context, it passes through immediately.
//   - Otherwise it triggers the lazy Redis lookup, checks the resource type,
//     and injects a synthetic guest actor for downstream business logic.
//
// Wire this into your gqlgen DirectiveRoot:
//
//	Directives: generated.DirectiveRoot{
//	    AuthGuest: guestResolver.AuthGuest,
//	}
func (gr *GuestResolver) AuthGuest(
	ctx context.Context,
	obj interface{},
	next graphql.Resolver,
	resource string,
) (interface{}, error) {
	// Real user — let through; the directive only gates guest access.
	if actor, err := middleware.GetActor(ctx); err == nil && !actor.IsGuest {
		return next(ctx)
	}

	// Lazy resolve — hits Redis on first call, cached for the rest of this request.
	newCtx, resolved, err := gr.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	// Check resource type matches what the directive declares.
	if resolved.Resource != resource {
		return nil, fmt.Errorf("unauthorized: guest access not allowed for this resource")
	}

	// Inject guest actor so downstream resolvers and business logic can read it.
	actor := &consts.Actor{
		ID:      uuid.Nil,
		IsGuest: true,
		Claims: &consts.UserClaims{
			CompanyID:  &resolved.CompanyID,
			Resource:   resolved.Resource,
			ResourceID: resolved.ResourceID,
		},
	}
	newCtx = middleware.WithActor(newCtx, actor)

	return next(newCtx)
}
