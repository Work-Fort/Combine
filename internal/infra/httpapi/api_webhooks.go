package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
	"github.com/gorilla/mux"
)

type createWebhookRequest struct {
	URL         string   `json:"url"`
	Events      []string `json:"events"`
	ContentType string   `json:"content_type,omitempty"`
	Active      *bool    `json:"active,omitempty"`
}

type updateWebhookRequest struct {
	URL         *string  `json:"url,omitempty"`
	Events      []string `json:"events,omitempty"`
	ContentType *string  `json:"content_type,omitempty"`
	Active      *bool    `json:"active,omitempty"`
}

type webhookResponse struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Events      []string  `json:"events"`
	ContentType string    `json:"content_type"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RegisterWebhookRoutes registers the webhook REST API routes on the given router.
func RegisterWebhookRoutes(r *mux.Router) {
	r.HandleFunc("/repos/{repo:.+}/webhooks", handleListWebhooks).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/webhooks", handleCreateWebhook).Methods("POST")
	r.HandleFunc("/repos/{repo:.+}/webhooks/{id:[0-9]+}", handleGetWebhook).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/webhooks/{id:[0-9]+}", handleUpdateWebhook).Methods("PATCH")
	r.HandleFunc("/repos/{repo:.+}/webhooks/{id:[0-9]+}", handleDeleteWebhook).Methods("DELETE")
}

func contentTypeFromString(s string) int {
	switch s {
	case "form":
		return int(webhook.ContentTypeForm)
	default:
		return int(webhook.ContentTypeJSON)
	}
}

func contentTypeToString(ct int) string {
	switch webhook.ContentType(ct) {
	case webhook.ContentTypeForm:
		return "form"
	default:
		return "json"
	}
}

func webhookToResponse(w *domain.Webhook, events []*domain.WebhookEvent) webhookResponse {
	evStrs := make([]string, 0, len(events))
	for _, e := range events {
		evStrs = append(evStrs, webhook.Event(e.Event).String())
	}
	return webhookResponse{
		ID:          w.ID,
		URL:         w.URL,
		Events:      evStrs,
		ContentType: contentTypeToString(w.ContentType),
		Active:      w.Active,
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	}
}

func handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]

	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}
	if len(req.Events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "events is required"})
		return
	}

	eventInts := make([]int, 0, len(req.Events))
	for _, name := range req.Events {
		ev, err := webhook.ParseEvent(name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event: " + name})
			return
		}
		eventInts = append(eventInts, int(ev))
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}

	ct := contentTypeFromString(req.ContentType)
	whID, err := store.CreateWebhook(ctx, repo.ID, req.URL, ct, active)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create webhook"})
		return
	}

	if err := store.CreateWebhookEvents(ctx, whID, eventInts); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create webhook events"})
		return
	}

	wh, err := store.GetWebhookByID(ctx, repo.ID, whID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to retrieve webhook"})
		return
	}

	events, err := store.ListWebhookEventsByWebhookID(ctx, whID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to retrieve webhook events"})
		return
	}

	writeJSON(w, http.StatusCreated, webhookToResponse(wh, events))
}

func handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	webhooks, err := store.ListWebhooksByRepoID(ctx, repo.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list webhooks"})
		return
	}

	resp := make([]webhookResponse, 0, len(webhooks))
	for _, wh := range webhooks {
		events, err := store.ListWebhookEventsByWebhookID(ctx, wh.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list webhook events"})
			return
		}
		resp = append(resp, webhookToResponse(wh, events))
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	whID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	wh, err := store.GetWebhookByID(ctx, repo.ID, whID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook not found"})
		return
	}

	events, err := store.ListWebhookEventsByWebhookID(ctx, wh.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list webhook events"})
		return
	}

	writeJSON(w, http.StatusOK, webhookToResponse(wh, events))
}

func handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	whID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)

	var req updateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	existing, err := store.GetWebhookByID(ctx, repo.ID, whID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook not found"})
		return
	}

	url := existing.URL
	if req.URL != nil {
		url = *req.URL
	}
	ct := existing.ContentType
	if req.ContentType != nil {
		ct = contentTypeFromString(*req.ContentType)
	}
	active := existing.Active
	if req.Active != nil {
		active = *req.Active
	}

	if err := store.UpdateWebhookByID(ctx, repo.ID, whID, url, ct, active); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update webhook"})
		return
	}

	if req.Events != nil {
		eventInts := make([]int, 0, len(req.Events))
		for _, name := range req.Events {
			ev, err := webhook.ParseEvent(name)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event: " + name})
				return
			}
			eventInts = append(eventInts, int(ev))
		}

		existingEvents, err := store.ListWebhookEventsByWebhookID(ctx, whID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list webhook events"})
			return
		}
		if len(existingEvents) > 0 {
			ids := make([]int64, len(existingEvents))
			for i, e := range existingEvents {
				ids[i] = e.ID
			}
			if err := store.DeleteWebhookEventsByID(ctx, ids); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete webhook events"})
				return
			}
		}
		if err := store.CreateWebhookEvents(ctx, whID, eventInts); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create webhook events"})
			return
		}
	}

	wh, err := store.GetWebhookByID(ctx, repo.ID, whID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to retrieve webhook"})
		return
	}
	events, err := store.ListWebhookEventsByWebhookID(ctx, whID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list webhook events"})
		return
	}

	writeJSON(w, http.StatusOK, webhookToResponse(wh, events))
}

func handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	whID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	if err := store.DeleteWebhookForRepoByID(ctx, repo.ID, whID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete webhook"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
