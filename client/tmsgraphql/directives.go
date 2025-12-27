package tmsgraphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/go-playground/validator/v10"
)

func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	_, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, fmt.Errorf("access denied: unauthenticated: %w", err)
	}

	return next(ctx)
}

// TODO: implement hasRole directive
func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, role string) (interface{}, error) {
	return next(ctx)
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, fmt.Errorf("access denied: unauthenticated")
	}
	if actor.Claims == nil {
		return nil, fmt.Errorf("access denied: unauthenticated")
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
		return nil, fmt.Errorf("access denied: unauthenticated")
	}
	if actor.Claims == nil {
		return nil, fmt.Errorf("access denied: unauthenticated")
	}

	for _, r := range actor.Claims.Permissions {
		if r == perm {
			return next(ctx)
		}
	}
	return next(ctx)
}

func ValidateDirective(v *validator.Validate) func(context.Context, interface{}, graphql.Resolver, string) (interface{}, error) {
	return func(ctx context.Context, obj interface{}, next graphql.Resolver, constraint string) (interface{}, error) {
		val, err := next(ctx)
		if err != nil {
			return nil, err
		}
		err = v.Var(val, constraint)
		if err != nil {
			return nil, fmt.Errorf("validation failed: %s", err.Error())
		}
		return val, nil
	}
}
