package tmsgraphql

import (
	"context"
	"fmt"

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

// TODO: implement hasRole directive
func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, role string) (interface{}, error) {
	return next(ctx)
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if actor.Claims == nil {
		return nil, consts.ErrUnauthorized
	}

	for _, r := range actor.Claims.Roles {
		if r == role {
			return next(ctx)
		}
	}

	return nil, fmt.Errorf("access denied: missing role '%s'", role)
}

// TODO: implement hasPerm directive
func HasPermDirective(ctx context.Context, obj interface{}, next graphql.Resolver, perm string) (interface{}, error) {
	return next(ctx)
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
