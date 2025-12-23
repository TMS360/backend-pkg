package directives

import (
	"context"
	"errors"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
	userId, ok := ctx.Value(consts.UserContextKey).(uuid.UUID)
	if !ok || userId == uuid.Nil {
		return nil, fmt.Errorf("access denied: unauthenticated")
	}

	return next(ctx)
}

// TODO: implement hasRole directive
func HasRoleDirective(ctx context.Context, obj interface{}, next graphql.Resolver, role string) (interface{}, error) {
	return next(ctx)
	claims, ok := ctx.Value(consts.ClaimsObjectKey).(*consts.UserClaims)
	if !ok {
		return nil, errors.New("access denied: unauthorized")
	}

	for _, r := range claims.Roles {
		if r == role {
			return next(ctx)
		}
	}

	return nil, fmt.Errorf("access denied: missing role '%s'", role)
}

// TODO: implement hasPerm directive
func HasPermDirective(ctx context.Context, obj interface{}, next graphql.Resolver, perm string) (interface{}, error) {
	return next(ctx)
	permissions, ok := ctx.Value("permissions").(map[string]bool)
	if !ok {
		return nil, errors.New("access denied: unauthenticated")
	}
	if !permissions[perm] {
		return nil, fmt.Errorf("access denied: missing permission '%s'", perm)
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
