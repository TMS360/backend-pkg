package tmsgraphql

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/TMS360/backend-pkg/validate"
	"github.com/vektah/gqlparser/v2/ast"
)

// NewHandler DefaultServer creates a handler with your organization's standard configuration
func NewHandler(es graphql.ExecutableSchema, isDebug bool) *handler.Server {
	srv := handler.New(es)

	// Apply the shared Error Presenter
	srv.SetErrorPresenter(NewErrorPresenter(isDebug))
	srv.AroundOperations(validate.OperationMiddleware())
	srv.AroundFields(validate.Middleware())

	// Standard Transports
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{
		MaxUploadSize: 32 * 1024 * 1024,
		MaxMemory:     32 * 1024 * 1024,
	})

	// Standard Caching & Extensions
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	// If you have APM (DataDog/Sentry/Prometheus), add it here too!
	// srv.Use(extension.FixedComplexityLimit(1000))

	return srv
}
