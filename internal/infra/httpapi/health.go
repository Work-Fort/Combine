package web

import (
	"net/http"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/version"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	store := domain.StoreFromContext(r.Context())
	status := "healthy"
	code := http.StatusOK

	if err := store.Ping(r.Context()); err != nil {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]string{"status": status})
}

func handleUIHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "combine",
		"version": version.Version,
		"routes": []map[string]string{
			{"route": "/api/v1", "label": "API"},
		},
	})
}
