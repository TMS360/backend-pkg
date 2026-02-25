package tmsgraphql

import (
	"context"
	"fmt"
	"slices"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
)

func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	_, err := middleware.GetActor(ctx)
	if err != nil {
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

	for _, r := range actor.Claims.Permissions {
		if r == perm {
			return next(ctx)
		}
	}
	return next(ctx)
}
