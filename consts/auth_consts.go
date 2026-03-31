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

type Actor struct {
	ID       uuid.UUID
	Claims   *UserClaims
	Token    *string
	IsSystem bool

	// For guests
	IsGuest          bool
	AccessResource   *string
	AccessResourceID *uuid.UUID
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
	UserID      uuid.UUID  `json:"sub"`
	CompanyID   *uuid.UUID `json:"company_id"`
	ActorType   ActorType  `json:"actor_type"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"perms"`

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

	c.PermissionsMap = make(map[string]struct{}, len(c.Permissions))
	for _, p := range c.Permissions {
		c.PermissionsMap[p] = struct{}{}
	}
}
