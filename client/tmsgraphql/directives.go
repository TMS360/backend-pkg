package tmsgraphql

import (
	"context"
	"fmt"
	"net/http"
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
			// A denial, not a crash — same reasoning as HasRoleDirective below
			// (DEV-1970): a bare error would be presented as
			// INTERNAL_SERVER_ERROR and page us on every wrong actor type.
			return nil, deniedError(
				fmt.Sprintf("forbidden: actor type %q does not have access (allowed: %v)", currentType, actorTypes),
			)
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

	// DEV-1970: a role denial is a normal answer, not a server fault. Returned
	// as a PublicError so the shared presenter emits code "FORBIDDEN" / status
	// 403 with a sentence a person can read, and reports it to Sentry as a
	// warning instead of paging on every routine "you are not allowed".
	// Only the directive's own verdict is converted — an error coming back out
	// of next(ctx) is still whatever the resolver made it, so a real crash
	// inside a role-gated field stays a crash.
	return nil, deniedError(
		fmt.Sprintf("access denied: missing role (requires one of %v)", roles),
	)
}

// AccessDeniedMessage is the single user-facing sentence for every directive
// denial. Plain words on purpose — the person reading it is not a developer.
const AccessDeniedMessage = "You are not allowed to do this. Ask an administrator if you need access."

// AccessDeniedCode is the stable machine-readable code clients branch on. It
// matches the code the shared gRPC mapping already uses for PermissionDenied,
// so a client has one code for "not allowed" no matter where it came from.
const AccessDeniedCode = "FORBIDDEN"

// deniedError builds the 403 every directive denial returns. tech is for the
// log and for Sentry; the caller only ever sees AccessDeniedMessage, so a
// denial never leaks which roles or actor types would have worked.
func deniedError(tech string) error {
	return response.NewCodedError(
		AccessDeniedCode,
		tech,
		AccessDeniedMessage,
		http.StatusForbidden,
		nil,
	)
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

	// DEV-1555: upstream perm lookup failed — not a real RBAC denial.
	if middleware.PermsUnresolved(ctx) {
		return nil, response.NewCodedError(
			"PERMS_UNRESOLVED",
			"perms unresolved: upstream auth lookup failed",
			"permission service unavailable, please retry",
			http.StatusServiceUnavailable,
			nil,
		)
	}

	userPerms := middleware.GetUserPermsFromContext(ctx)
	for _, required := range perms {
		if middleware.HasPermission(userPerms, required) {
			return next(ctx)
		}
	}

	return nil, response.NewForbidden("access denied: missing permission", "access denied: missing permission")
}
