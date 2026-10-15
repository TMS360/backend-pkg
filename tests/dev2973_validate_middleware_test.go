package tests

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

// DEV-2973 — a mutation whose input failed @validate must not run its resolver.
// gqlgen parses arguments (running the directive) before field middleware, so
// the refusal is already in the context when Middleware is reached.

func validationOperationCtx(t *testing.T) context.Context {
	t.Helper()
	var ctx context.Context
	validate.OperationMiddleware()(context.Background(), func(c context.Context) graphql.ResponseHandler {
		ctx = c
		return nil
	})
	require.NotNil(t, ctx)
	return ctx
}

func resolverFieldCtx(ctx context.Context, isResolver bool) context.Context {
	return graphql.WithFieldContext(ctx, &graphql.FieldContext{
		Field:      graphql.CollectedField{Field: &ast.Field{Name: "createDriver"}},
		IsResolver: isResolver,
	})
}

func TestValidateMiddleware_RefusedInputNeverRunsTheResolver(t *testing.T) {
	ctx := validationOperationCtx(t)
	validate.WithValidationError(ctx, &validate.FieldValidationError{
		Field: "email", Rules: []string{"email"}, Messages: []string{"email must be a valid email address"},
	})

	ran := false
	result, err := validate.Middleware()(resolverFieldCtx(ctx, true), func(context.Context) (any, error) {
		ran = true
		return "created", nil
	})

	assert.False(t, ran, "the resolver must not run (it would save the row)")
	assert.Nil(t, result)
	var ve *validate.ValidationErrors
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "validation failed for field: email", ve.Error())
}

func TestValidateMiddleware_ValidInputRunsTheResolver(t *testing.T) {
	ctx := validationOperationCtx(t)

	result, err := validate.Middleware()(resolverFieldCtx(ctx, true), func(context.Context) (any, error) {
		return "created", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "created", result)
}

func TestValidateMiddleware_ErrorRecordedDuringResolverStillRefuses(t *testing.T) {
	ctx := validationOperationCtx(t)

	_, err := validate.Middleware()(resolverFieldCtx(ctx, true), func(c context.Context) (any, error) {
		validate.WithValidationError(c, &validate.FieldValidationError{Field: "name", Rules: []string{"minLen"}})
		return "created", nil
	})

	var ve *validate.ValidationErrors
	require.ErrorAs(t, err, &ve)
}

func TestValidateMiddleware_PlainFieldAccessorsPassThrough(t *testing.T) {
	ctx := validationOperationCtx(t)
	validate.WithValidationError(ctx, &validate.FieldValidationError{Field: "email"})

	result, err := validate.Middleware()(resolverFieldCtx(ctx, false), func(context.Context) (any, error) {
		return "id-1", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "id-1", result)
}
