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
	return actor.Claims.CompanyID
}

type UserClaims struct {
	UserID      uuid.UUID  `json:"sub"`
	CompanyID   *uuid.UUID `json:"company_id"`
	ActorType   ActorType  `json:"actor_type"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"perms"`

	// Embed Standard/Registered claims for standard fields like exp, iat, iss
	jwt.RegisteredClaims
}
