package middleware

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// IdentifyUser извлекает и проверяет JWT из заголовка Authorization и устанавливает информацию о пользователе в контекст
func IdentifyUser(rsaPubKey *rsa.PublicKey, args ...ed25519.PublicKey) gin.HandlerFunc {
	// 1. Safely extract the optional Ed25519 public key
	var edPubKey ed25519.PublicKey
	if len(args) > 0 {
		edPubKey = args[0]
	}

	return func(ctx *gin.Context) {
		// 1. Attempt System User Authentication
		if authHeader := ctx.GetHeader("Authorization"); authHeader != "" {
			actor, err := parseAuthToken(authHeader, rsaPubKey)
			if err == nil {
				ctx.Request = ctx.Request.WithContext(WithActor(ctx.Request.Context(), actor))
				ctx.Next()
				return
			}
			slog.Debug("System auth attempt failed", "error", err)
		}

		// 2. Attempt Guest Authentication (Fallback)
		if guestToken := ctx.GetHeader("X-Guest-Token"); guestToken != "" && edPubKey != nil {
			actor, err := parseGuestToken(guestToken, edPubKey)
			if err == nil {
				ctx.Request = ctx.Request.WithContext(WithActor(ctx.Request.Context(), actor))
				ctx.Next()
				return
			}
			slog.Debug("Guest auth attempt failed", "error", err)
		}

		// 3. Unauthenticated (Anonymous)
		ctx.Next()
	}
}

func parseAuthToken(authHeader string, publicKey *rsa.PublicKey) (*consts.Actor, error) {
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, errors.New("invalid authorization header format")
	}

	tokenString := parts[1]
	token, err := jwt.ParseWithClaims(tokenString, &consts.UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*consts.UserClaims)
	if !ok {
		return nil, errors.New("failed to cast user claims")
	}

	claims.RolesMap = make(map[string]struct{}, len(claims.Roles))
	for _, r := range claims.Roles {
		claims.RolesMap[r] = struct{}{}
	}

	claims.PermissionsMap = make(map[string]struct{}, len(claims.Permissions))
	for _, p := range claims.Permissions {
		claims.PermissionsMap[p] = struct{}{}
	}

	return &consts.Actor{
		ID:      claims.UserID,
		Claims:  claims,
		Token:   utils.Pointer(tokenString),
		IsGuest: false,
	}, nil
}

func parseGuestToken(tokenString string, pubKey ed25519.PublicKey) (*consts.Actor, error) {
	token, err := jwt.ParseWithClaims(tokenString, &consts.GuestClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid guest token: %w", err)
	}

	claims, ok := token.Claims.(*consts.GuestClaims)
	if !ok {
		return nil, errors.New("failed to cast guest claims")
	}

	return &consts.Actor{
		ID:               uuid.Nil,
		Token:            utils.Pointer(tokenString),
		IsGuest:          true,
		AccessResource:   &claims.Resource,
		AccessResourceID: &claims.ResourceID,
	}, nil
}

// RequireAuth проверяет, был ли пользователь установлен в контекст предыдущим middleware
func RequireAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		actor, err := GetActor(ctx.Request.Context())
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// Prevent guests from accessing standard REST routes
		if actor.IsGuest {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		ctx.Next()
	}
}

func WithActor(ctx context.Context, actor *consts.Actor) context.Context {
	return context.WithValue(ctx, consts.ActorCtx, *actor)
}

func WithSystemActor(ctx context.Context) context.Context {
	return context.WithValue(ctx, consts.ActorCtx, consts.Actor{ID: uuid.Nil, IsSystem: true})
}

// GetActor safely extracts the actor.
func GetActor(ctx context.Context) (*consts.Actor, error) {
	actor, ok := ctx.Value(consts.ActorCtx).(consts.Actor)
	if !ok {
		return nil, errors.New("actor not found in context")
	}
	return &actor, nil
}

// MustGetActor for when you are sure (or want to panic/default)
func MustGetActor(ctx context.Context) *consts.Actor {
	actor, ok := ctx.Value(consts.ActorCtx).(consts.Actor)
	if !ok {
		return &consts.Actor{ID: uuid.Nil, Claims: nil, IsSystem: true}
	}
	return &actor
}
