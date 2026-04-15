package guest

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ShareLinkClaims is the JWT payload for guest tokens.
type ShareLinkClaims struct {
	ShareLinkID uuid.UUID `json:"slid"`
	jwt.RegisteredClaims
}

// ParseGuestToken verifies the HS256 signature and expiry. Pure CPU — no network.
func ParseGuestToken(tokenString string, secret []byte) (*ShareLinkClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &ShareLinkClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid guest token: %w", err)
	}

	claims, ok := token.Claims.(*ShareLinkClaims)
	if !ok {
		return nil, fmt.Errorf("failed to cast share link claims")
	}
	return claims, nil
}
