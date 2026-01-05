package consts

import (
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
	IsSystem bool
}

type UserClaims struct {
	UserID      uuid.UUID `json:"sub"`
	ActorType   ActorType `json:"actor_type"`
	Roles       []string  `json:"roles"`
	Permissions []string  `json:"perms"`

	// Embed Standard/Registered claims for standard fields like exp, iat, iss
	jwt.RegisteredClaims
}
