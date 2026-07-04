package tmsgraphql

import (
	"context"
	"fmt"
	"slices"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/response"
)

func AuthDirective(ctx context.Context, obj interface{}, next graphql.Resolver, actorTypes []string) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil || actor.IsGuest {
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

// HasPermDirective backs `@hasPerm(perms: [...])`. Effective perms are read
// from the request context (stashed by auth.IdentifyUserPerms middleware on
// every service); the JWT no longer carries perms.
//
// Matching is hierarchical: holding "accounting" grants every key under it.
// Any one of `perms` being granted is sufficient (OR semantics, preserving
// the prior directive's contract).
//
// Guests bypass the perm check. Guest access is granted per-field by
// `@authGuest`, which verifies the share-link token's resource scope; a guest
// reaching a field without `@authGuest` still fails closed at that directive.
//
// Super-admins also bypass. A wiped or mid-migration role_permissions table
// would otherwise lock super-admins out of every gated endpoint — this gives
// ops a permanent recovery path that doesn't depend on the catalog state.
func HasPermDirective(ctx context.Context, obj interface{}, next graphql.Resolver, perms []string) (interface{}, error) {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return nil, consts.ErrUnauthorized
	}
	if actor.IsGuest {
		return next(ctx)
	}
	if actor.IsSuperAdmin() {
		return next(ctx)
	}

	userPerms := middleware.GetUserPermsFromContext(ctx)
	for _, required := range perms {
		if middleware.HasPermission(userPerms, required) {
			return next(ctx)
		}
	}

	return nil, response.NewForbidden("access denied: missing permission", "access denied: missing permission")
}
