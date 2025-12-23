package middleware

import (
	"slices"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Cors(allowedOrigins []string) gin.HandlerFunc {
	cfg := cors.Config{
		AllowOriginFunc: func(origin string) bool {
			if slices.Contains(allowedOrigins, origin) {
				return true
			}
			return false
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	return cors.New(cfg)
}
