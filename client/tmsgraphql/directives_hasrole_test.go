package tmsgraphql

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1970 — "you are not allowed" used to come back as INTERNAL_SERVER_ERROR
// on every @hasRole field, in every service, and paged us on each one. The fix
// lives in this one shared directive, so these tests are the whole fleet's
// coverage: all seven services bind tmsgraphql.HasRoleDirective.

func actorWithRoles(roles ...string) context.Context {
	uid := uuid.New()
	return consts.WithActor(context.Background(), &consts.Actor{
		ID: uid,
		Claims: &consts.UserClaims{
			UserID:           uid,
			Roles:            roles,
			RegisteredClaims: jwt.RegisteredClaims{Subject: uid.String()},
		},
	})
}

// resolverThatMustNotRun fails the test if the directive lets the call through.
func resolverThatMustNotRun(t *testing.T) graphql.Resolver {
	t.Helper()
	return func(ctx context.Context) (any, error) {
		require.FailNow(t, "resolver ran despite the role check failing")
		return nil, nil
	}
}

// AC1: a user without the role gets a denial code and a readable message —
// not INTERNAL_SERVER_ERROR.
func TestHasRole_DenialIsAForbidden_NotAServerError(t *testing.T) {
	spy := withCaptureSpy(t)

	_, err := HasRoleDirective(actorWithRoles(enums.UserRoleAdmin.String()),
		nil, resolverThatMustNotRun(t), []string{"super_admin"})
	require.Error(t, err)

	gqlErr := NewErrorPresenter(false)(context.Background(), err)

	assert.Equal(t, AccessDeniedMessage, gqlErr.Message)
	assert.NotEqual(t, "Internal Server Error", gqlErr.Message)
	assert.Equal(t, "FORBIDDEN", gqlErr.Extensions["code"])
	assert.Equal(t, http.StatusForbidden, gqlErr.Extensions["status"])

	// AC4: a denial must not raise a server-error alert.
	assert.Equal(t, 0, spy.errors, "a routine denial must never page anyone")
	assert.Equal(t, 1, spy.warnings, "it is still queryable in Sentry as a warning")
}

// AC2 (the reported field): companyFeatureFlags is @hasRole(["super_admin"]).
// A company admin is refused; platform staff still gets through untouched.
func TestHasRole_SuperAdminOnlyField(t *testing.T) {
	const gate = "super_admin"

	_, err := HasRoleDirective(actorWithRoles(enums.UserRoleAdmin.String()),
		nil, resolverThatMustNotRun(t), []string{gate})
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, err.(interface{ ErrorStatus() int }).ErrorStatus())

	// AC5: a user who has the role sees no change at all.
	out, err := HasRoleDirective(actorWithRoles(gate), nil,
		func(context.Context) (any, error) { return "the list", nil }, []string{gate})
	require.NoError(t, err)
	assert.Equal(t, "the list", out)
}

// The denial must not tell the caller which role would have worked.
func TestHasRole_DenialDoesNotLeakTheGate(t *testing.T) {
	_, err := HasRoleDirective(actorWithRoles("dispatcher"), nil,
		resolverThatMustNotRun(t), []string{"super_admin"})
	require.Error(t, err)

	gqlErr := NewErrorPresenter(false)(context.Background(), err)
	assert.NotContains(t, gqlErr.Message, "super_admin")
	assert.Contains(t, err.Error(), "super_admin", "the technical text still says it, for the log")
}

// Edge: no login at all must still be "not signed in" (401), never the new 403.
func TestHasRole_NotSignedInStays401(t *testing.T) {
	_, err := HasRoleDirective(context.Background(), nil,
		resolverThatMustNotRun(t), []string{"super_admin"})
	require.ErrorIs(t, err, consts.ErrUnauthorized)

	gqlErr := NewErrorPresenter(false)(context.Background(), err)
	assert.Equal(t, http.StatusUnauthorized, gqlErr.Extensions["status"])
}

// Edge: an actor with no claims at all (system / guest — DEV-1732) is a
// "not signed in", not a crash and not a denial.
func TestHasRole_ActorWithoutClaimsStays401(t *testing.T) {
	ctx := consts.WithActor(context.Background(), &consts.Actor{ID: uuid.New()})

	_, err := HasRoleDirective(ctx, nil, resolverThatMustNotRun(t), []string{"super_admin"})
	require.ErrorIs(t, err, consts.ErrUnauthorized)
}

// Edge: a real crash inside a role-gated field must stay a crash — the
// directive only converts its own verdict.
func TestHasRole_CrashInsideTheFieldStaysAServerError(t *testing.T) {
	spy := withCaptureSpy(t)
	boom := errors.New("nil map write in the resolver")

	_, err := HasRoleDirective(actorWithRoles("super_admin"), nil,
		func(context.Context) (any, error) { return nil, boom }, []string{"super_admin"})
	require.ErrorIs(t, err, boom)

	gqlErr := NewErrorPresenter(false)(context.Background(), err)
	assert.Equal(t, "Internal Server Error", gqlErr.Message)
	assert.Equal(t, "INTERNAL_SERVER_ERROR", gqlErr.Extensions["code"])
	assert.Equal(t, 1, spy.errors, "a real crash still alerts")
	assert.Equal(t, 0, spy.warnings)
}

// The same treatment for @auth(actorTypes:) — the other directive verdict that
// was a bare error and therefore a 500.
func TestAuth_WrongActorTypeIsAForbidden(t *testing.T) {
	spy := withCaptureSpy(t)
	uid := uuid.New()
	ctx := consts.WithActor(context.Background(), &consts.Actor{
		ID: uid,
		Claims: &consts.UserClaims{
			UserID:           uid,
			ActorType:        consts.ActorBroker,
			RegisteredClaims: jwt.RegisteredClaims{Subject: uid.String()},
		},
	})

	_, err := AuthDirective(ctx, nil, resolverThatMustNotRun(t), []string{string(consts.ActorCourier)})
	require.Error(t, err)

	gqlErr := NewErrorPresenter(false)(context.Background(), err)
	assert.Equal(t, AccessDeniedMessage, gqlErr.Message)
	assert.Equal(t, http.StatusForbidden, gqlErr.Extensions["status"])
	assert.Equal(t, 0, spy.errors)
	assert.Equal(t, 1, spy.warnings)
}
