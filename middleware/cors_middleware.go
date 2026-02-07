package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

//func Cors() gin.HandlerFunc {
//	cfg := cors.Config{
//		AllowOriginFunc: func(origin string) bool {
//			return true
//		},
//		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
//		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
//		ExposeHeaders:    []string{"Content-Length", "Set-Cookie"},
//		AllowCredentials: true,
//		MaxAge:           12 * time.Hour,
//	}
//	return cors.New(cfg)
//}

func Cors(allowedOrigins []string) gin.HandlerFunc {
	cfg := cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length", "Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	return cors.New(cfg)
}
