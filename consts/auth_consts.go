package consts

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

// TODO: encapsulate context keys with methods to avoid collisions
const UserContextKey contextKey = "userUuid"
const ClaimsObjectKey contextKey = "claimsObject"

// UserClaims represents the JWT claims for a user
type UserClaims struct {
	UserID      uuid.UUID `json:"sub"`
	Roles       []string  `json:"roles"`
	Permissions []string  `json:"perms"`

	// Embed Standard/Registered claims for standard fields like exp, iat, iss
	jwt.RegisteredClaims
}
