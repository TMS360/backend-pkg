package consts

import (
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


// CodeRateLimited / MsgRateLimited are the signal that an authenticated caller
// exceeded the per-(user, IP) request ceiling (DEV-1485). It is deliberately
// distinct from the guest sign-in throttle (which returns its own GraphQL error
// string) so the two 429s stay tellable apart in logs, Sentry and the FE. The
// message is intentionally generic — it must not leak the limit or the window.
const (
	CodeRateLimited = "rate_limited"
	MsgRateLimited  = "Too many requests. Please slow down and try again shortly."
)


type Actor struct {
	ID       uuid.UUID
	Claims   *UserClaims
	Token    *string
	IsSystem bool
	IsGuest  bool
}

// IsSuperAdmin reports whether the actor carries the super-admin role.
//
// A system actor (cron, scheduler, seeder) has no JWT, so Claims is nil and the
// answer is simply false. The nil check is load-bearing, not defensive: callers
// evaluate this BEFORE the IsSystem branch — e.g. model/tenant_scoped.go does
// `if actor.IsSuperAdmin() || actor.IsSystem` — so reaching through Claims here
// panics on the exact path that is supposed to produce a readable error
// (DEV-1732, same family as the audit-writer crash).
func (actor *Actor) IsSuperAdmin() bool {
	if actor.Claims == nil {
		return false
	}
	for _, role := range actor.Claims.Roles {
		if role == enums.UserRoleSuperAdmin.String() {
			return true
		}
	}
	return false
}

// IsAdmin reports whether the actor carries the admin role. Nil-safe for the
// same reason as IsSuperAdmin: a system actor has no claims.
func (actor *Actor) IsAdmin() bool {
	if actor.Claims == nil {
		return false
	}
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
