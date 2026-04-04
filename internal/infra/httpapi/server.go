package web

import (
	"context"
	"net/http"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/config"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// NewRouter returns a new HTTP router.
func NewRouter(ctx context.Context, passport *PassportAuth) http.Handler {
	logger := log.FromContext(ctx).WithPrefix("http")
	router := mux.NewRouter()

	// Health routes (no auth required)
	router.HandleFunc("/v1/health", handleHealth).Methods("GET")
	router.HandleFunc("/ui/health", handleUIHealth).Methods("GET")

	// REST API routes (Passport auth required)
	if passport != nil {
		api := router.PathPrefix("/api/v1").Subrouter()
		api.Use(passport.Middleware)
		// Issue routes MUST be registered before repo routes — the {repo:.+} pattern
		// is greedy and will swallow /repos/{repo}/issues paths otherwise.
		RegisterIssueRoutes(api)
		RegisterRepoRoutes(api)
		RegisterKeyRoutes(api)
	}

	// Git routes
	GitController(ctx, router)

	router.PathPrefix("/").HandlerFunc(renderNotFound)

	// Context handler
	// Adds context to the request
	h := NewLoggingMiddleware(router, logger)
	h = NewContextHandler(ctx)(h)
	h = handlers.CompressHandler(h)
	h = handlers.RecoveryHandler()(h)

	cfg := config.FromContext(ctx)

	h = handlers.CORS(handlers.AllowedHeaders(cfg.HTTP.CORS.AllowedHeaders),
		handlers.AllowedOrigins(cfg.HTTP.CORS.AllowedOrigins),
		handlers.AllowedMethods(cfg.HTTP.CORS.AllowedMethods),
	)(h)

	return h
}
