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
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// IdentifyUser извлекает и проверяет JWT из заголовка Authorization и устанавливает информацию о пользователе в контекст
func IdentifyUser(publicKey *rsa.PublicKey) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.Next()
			return
		}

		fmt.Println("authHeader", authHeader)

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			slog.Error("Invalid Authorization header format")
			ctx.Next()
			return
		}
		tokenString := parts[1]

		token, err := jwt.ParseWithClaims(tokenString, &consts.UserClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return publicKey, nil
		})

		if err != nil || !token.Valid {
			slog.Error("Token invalid or expired:", err)
			ctx.Next()
			return
		}

		claims, ok := token.Claims.(*consts.UserClaims)
		if ok {
			ctxWithActor := WithActor(ctx.Request.Context(), claims.UserID, claims)
			ctx.Request = ctx.Request.WithContext(ctxWithActor)
		}

		ctx.Next()
	}
}

// RequireAuth проверяет, был ли пользователь установлен в контекст предыдущим middleware
func RequireAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		_, err := GetActor(ctx.Request.Context())
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		ctx.Next()
	}
}

// WithActor adds user info to the context (Used by your Middleware)
func WithActor(ctx context.Context, userID uuid.UUID, userClaims *consts.UserClaims) context.Context {
	return context.WithValue(ctx, consts.ActorCtx, consts.Actor{
		ID:     userID,
		Claims: userClaims,
	})
}

// GetActor safely extracts the actor.
func GetActor(ctx context.Context) (consts.Actor, error) {
	actor, ok := ctx.Value(consts.ActorCtx).(consts.Actor)
	if !ok {
		return consts.Actor{}, errors.New("actor not found in context")
	}
	if actor.ID == uuid.Nil {
		return consts.Actor{}, errors.New("invalid actor ID")
	}

	return actor, nil
}

// MustGetActor for when you are sure (or want to panic/default)
func MustGetActor(ctx context.Context) consts.Actor {
	actor, ok := ctx.Value(consts.ActorCtx).(consts.Actor)
	if !ok {
		return consts.Actor{ID: uuid.Nil, Claims: nil, IsSystem: true}
	}
	return actor
}
