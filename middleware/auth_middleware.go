package middleware

import (
	"context"
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
func IdentifyUser(rsaPubKey *rsa.PublicKey) gin.HandlerFunc {
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

		// 3. Unauthenticated (Anonymous)
		ctx.Next()
	}
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

func ClearAuthContext() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(WithActor(ctx.Request.Context(), nil))
		ctx.Next()
	}
}

func WithActor(ctx context.Context, actor *consts.Actor) context.Context {
	return context.WithValue(ctx, consts.ActorCtx, actor)
}

func WithSystemActor(ctx context.Context) context.Context {
	return context.WithValue(ctx, consts.ActorCtx, &consts.Actor{ID: uuid.Nil, IsSystem: true})
}

// GetActor safely extracts the actor.
func GetActor(ctx context.Context) (*consts.Actor, error) {
	actor, ok := ctx.Value(consts.ActorCtx).(*consts.Actor)
	if !ok {
		return nil, errors.New("actor not found in context")
	}
	return actor, nil
}

// MustGetActor for when you are sure (or want to panic/default)
func MustGetActor(ctx context.Context) *consts.Actor {
	actor, ok := ctx.Value(consts.ActorCtx).(*consts.Actor)
	if !ok {
		return &consts.Actor{ID: uuid.Nil, Claims: nil, IsSystem: true}
	}
	return actor
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

	claims.PopulateMaps()

	return &consts.Actor{
		ID:      claims.UserID,
		Claims:  claims,
		Token:   utils.Pointer(tokenString),
		IsGuest: false,
	}, nil
}
