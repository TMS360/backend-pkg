package middleware

import (
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Cors echoes an Origin back (with credentials) only when it exactly matches an
// entry of allowedOrigins. An empty list, or a "*" entry, allows nothing: a
// wildcard is never combined with credentials.
func Cors(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" && o != "*" {
			allowed[o] = struct{}{}
		}
	}

	cfg := cors.Config{
		AllowOriginFunc: func(origin string) bool {
			_, ok := allowed[origin]
			return ok
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Guest-Token"},
		ExposeHeaders:    []string{"Content-Length", "Set-Cookie"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	return cors.New(cfg)
}
