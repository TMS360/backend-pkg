package middleware

import (
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Cors echoes an Origin back (with credentials) only when it exactly matches an
// entry of allowedOrigins. Entries are trimmed of spaces, surrounding double
// quotes and a trailing "/", and lowercased (browsers send lowercase origins).
// An empty list, or a "*" entry, allows nothing: a wildcard is never combined
// with credentials.
func Cors(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(strings.Trim(strings.TrimSpace(o), `"`))
		if o = strings.ToLower(strings.TrimRight(o, "/")); o != "" && o != "*" {
			allowed[o] = struct{}{}
		}
	}
	origins := make([]string, 0, len(allowed))
	for o := range allowed {
		origins = append(origins, o)
	}
	sort.Strings(origins)
	if len(origins) == 0 {
		slog.Warn("CORS allow-list is empty: no cross-origin request will be allowed")
	} else {
		slog.Info("CORS allow-list", "origins", origins)
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
