package tmsgraphql

import (
	"context"
	"fmt"
	"slices"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
)

func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver, actorTypes []string) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}

	if actor.IsGuest {
		return nil, consts.ErrUnauthorized
	}

	if len(actorTypes) > 0 {
		isAllowed := false
		currentType := string(actor.Claims.ActorType)

		for _, allowedType := range actorTypes {
			// e.g., allowedType == "courier", currentType == "courier"
			if currentType == allowedType {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			// Optional: Create a consts.ErrForbidden for cleaner error handling
			return nil, fmt.Errorf("forbidden: actor type '%s' does not have access", currentType)
		}
	}

	return next(ctx)
}

// AuthGuestDirective allows both standard users and guests. Used for @authGuest.
func AuthGuestDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil || actor == nil {
		return nil, consts.ErrUnauthorized
	}

	return next(ctx)
}

func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, roles []string) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if actor.Claims == nil {
		return nil, consts.ErrUnauthorized
	}

	for _, role := range roles {
		if slices.Contains(actor.Claims.Roles, role) {
			return next(ctx)
		}
	}

	return nil, fmt.Errorf("access denied: missing role")
}

func HasPermDirective(ctx context.Context, obj interface{}, next graphql.Resolver, perms []string) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if actor.Claims == nil {
		return nil, consts.ErrUnauthorized
	}

	for _, perm := range perms {
		if slices.Contains(actor.Claims.Permissions, perm) {
			return next(ctx)
		}
	}

	return nil, fmt.Errorf("access denied: missing permission")
}
