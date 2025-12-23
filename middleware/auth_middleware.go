package middleware

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// IdentifyUser извлекает и проверяет JWT из заголовка Authorization и устанавливает информацию о пользователе в контекст
func IdentifyUser(publicKey *rsa.PublicKey) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.Next()
			return
		}

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
			ctx.Set(consts.ClaimsObjectKey, claims)
			ctx.Set(consts.UserContextKey, claims.UserID)

			reqCtx := ctx.Request.Context()
			reqCtx = context.WithValue(reqCtx, consts.ClaimsObjectKey, claims)
			reqCtx = context.WithValue(reqCtx, consts.UserContextKey, claims.UserID)

			ctx.Request = ctx.Request.WithContext(reqCtx)
		}

		ctx.Next()
	}
}

// RequireAuth проверяет, был ли пользователь установлен в контекст предыдущим middleware
func RequireAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		_, exists := ctx.Get(consts.UserContextKey)
		if !exists {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		ctx.Next()
	}
}
