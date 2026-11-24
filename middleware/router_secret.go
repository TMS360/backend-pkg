package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// HeaderRouterSecret is the header the Apollo router adds to every request
	// it sends to a subgraph (apollo-router router.yaml, DEV-4807). Browsers
	// and scripts calling a subgraph's public domain do not have it.
	HeaderRouterSecret = "X-Router-Secret"
	// EnvRouterSecret holds the shared secret. The router and every subgraph
	// that mounts RequireRouterSecret read the same Railway variable.
	EnvRouterSecret = "ROUTER_SUBGRAPH_SECRET"
)

// RequireRouterSecret refuses a request to the GraphQL endpoint that did not
// come through the Apollo router (DEV-4807). Some subgraphs keep a public
// domain for webhooks and uploads, and without this gate their /query answers
// anyone, federation-only fields (_entities, _service) included, with none of
// the router's limits.
//
// Mount it on the GraphQL routes only (/query, /playground), never globally:
// webhooks, uploads, health checks and gRPC must keep working without it.
//
// The secret is read once, at startup. When it is unset the gate logs one
// warning and lets every request through, so the variable can be set on the
// router first and on the subgraphs afterwards without an outage. When it is
// set, a request without the matching header gets 403.
func RequireRouterSecret() gin.HandlerFunc {
	secret := strings.TrimSpace(os.Getenv(EnvRouterSecret))
	if secret == "" {
		slog.Warn(EnvRouterSecret + " is unset: the GraphQL endpoint accepts requests that did not come through the router")
		return func(c *gin.Context) { c.Next() }
	}
	want := []byte(secret)
	return func(c *gin.Context) {
		got := []byte(c.GetHeader(HeaderRouterSecret))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"errors": []gin.H{{"message": "forbidden"}},
			})
			return
		}
		c.Next()
	}
}
