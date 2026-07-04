package consts

import (
	"context"
	"errors"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ActorType string

const (
	ActorCourier ActorType = "courier"
	ActorBroker  ActorType = "broker"
)

type contextKey string

// TODO: encapsulate context keys with methods to avoid collisions
const ActorCtx contextKey = "actor"

// PermsCtx holds the caller's resolved permission codes (stashed by
// IdentifyUserPerms). It lives here — not in middleware — so low-level packages
// (auth, cache) can read/write request-scoped auth data without importing
// middleware, which would form an import cycle
// (middleware -> auth -> cache -> middleware).
const PermsCtx contextKey = "user_perms"

// WithActor / GetActor / MustGetActor / WithSystemActor are the canonical actor
// context accessors. They live in consts (alongside the ActorCtx key) so that
// cache and auth can reach the actor without importing middleware. middleware
// keeps same-named wrappers that delegate here, so existing call sites are
// unaffected.
func WithActor(ctx context.Context, actor *Actor) context.Context {
	return context.WithValue(ctx, ActorCtx, actor)
}

func WithSystemActor(ctx context.Context) context.Context {
	return context.WithValue(ctx, ActorCtx, &Actor{ID: uuid.Nil, IsSystem: true})
}

func GetActor(ctx context.Context) (*Actor, error) {
	actor, ok := ctx.Value(ActorCtx).(*Actor)
	if !ok {
		return nil, errors.New("actor not found in context")
	}
	return actor, nil
}

// MustGetActor returns a default system actor when none is present, for call
// sites that would otherwise have to panic or nil-check.
func MustGetActor(ctx context.Context) *Actor {
	actor, ok := ctx.Value(ActorCtx).(*Actor)
	if !ok {
		return &Actor{ID: uuid.Nil, Claims: nil, IsSystem: true}
	}
	return actor
}

// WithUserPerms stashes resolved permission codes; GetUserPermsFromContext reads
// them back (empty slice when absent, which denies all under HasPermission).
func WithUserPerms(ctx context.Context, perms []string) context.Context {
	return context.WithValue(ctx, PermsCtx, perms)
}

type Actor struct {
	ID       uuid.UUID
	Claims   *UserClaims
	Token    *string
	IsSystem bool
	IsGuest  bool
}

func (actor *Actor) IsSuperAdmin() bool {
	for _, role := range actor.Claims.Roles {
		if role == enums.UserRoleSuperAdmin.String() {
			return true
		}
	}
	return false
}

func (actor *Actor) IsAdmin() bool {
	for _, role := range actor.Claims.Roles {
		if role == enums.UserRoleAdmin.String() {
			return true
		}
	}
	return false
}

func (actor *Actor) GetCompanyID() *uuid.UUID {
	if actor.Claims == nil {
		return nil
	}
	return actor.Claims.CompanyID
}

type UserClaims struct {
	UserID    uuid.UUID  `json:"sub"`
	CompanyID *uuid.UUID `json:"company_id"`
	ActorType ActorType  `json:"actor_type"`
	Roles     []string   `json:"roles"`

	// --- Guest/Share Fields ---
	Resource   string    `json:"res,omitempty"`
	ResourceID uuid.UUID `json:"res_id,omitempty"`

	// Internal Maps (Use JSON:"-" so they don't interfere with JWT parsing)
	RolesMap       map[string]struct{} `json:"-"`
	PermissionsMap map[string]struct{} `json:"-"`

	// Embed Standard/Registered claims for standard fields like exp, iat, iss
	jwt.RegisteredClaims
}

// PopulateMaps hydrates the fast-lookup maps from the string slices.
// Call this AFTER parsing a JWT or BEFORE injecting into an internal Go context.
func (c *UserClaims) PopulateMaps() {
	c.RolesMap = make(map[string]struct{}, len(c.Roles))
	for _, r := range c.Roles {
		c.RolesMap[r] = struct{}{}
	}
}
