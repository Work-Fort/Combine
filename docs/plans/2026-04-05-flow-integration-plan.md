# Flow Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add webhook registration REST API so Flow can programmatically register webhook callbacks. Verify push payload includes commit details (already confirmed -- no code change needed).

**Architecture:** New handler file `internal/infra/httpapi/api_webhooks.go` following the same pattern as `api_issues.go`. Direct store access, Passport auth via existing middleware. Event names mapped via `webhook.ParseEvent()` / `Event.String()`.

**Tech Stack:** gorilla/mux, encoding/json, existing webhook and domain packages

---

## Task 0: Remove webhook secret from codebase

**Why:** The `secret` column and HMAC signing logic were inherited from Soft Serve. Within WorkFort's internal network, services authenticate via Passport tokens, so webhook payload signing is unnecessary. Remove it entirely before building the REST API so the new handlers never need to deal with it.

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/ports.go`
- Modify: `internal/app/backend/webhooks.go`
- Modify: `internal/infra/sqlite/webhook.go`
- Modify: `internal/infra/postgres/webhook.go`
- Modify: `internal/infra/webhook/webhook.go`
- Modify: `internal/infra/webhook/ssrf_test.go`
- Modify: `internal/infra/sqlite/migrations/001_init.sql`
- Modify: `internal/infra/postgres/migrations/001_init.sql`

**Step 1: Drop `secret` column from database schema**

Remove `secret TEXT NOT NULL` from the `webhooks` CREATE TABLE in both:
- `internal/infra/sqlite/migrations/001_init.sql`
- `internal/infra/postgres/migrations/001_init.sql`

**Step 2: Remove `Secret` from domain types and ports**

In `internal/domain/types.go`, remove the `Secret string` field from the `Webhook` struct.

In `internal/domain/ports.go`, remove the `secret string` parameter from:
- `CreateWebhook(ctx context.Context, repoID int64, url string, contentType int, active bool) (int64, error)`
- `UpdateWebhookByID(ctx context.Context, repoID int64, id int64, url string, contentType int, active bool) error`

**Step 3: Update SQLite and Postgres adapters**

In both `internal/infra/sqlite/webhook.go` and `internal/infra/postgres/webhook.go`:
- Remove `secret` from `webhookColumns` constant
- Remove `&w.Secret` from `scanWebhook`
- Remove `secret` parameter from `createWebhook` and `updateWebhookByID` internal functions
- Remove `secret` from SQL INSERT and UPDATE statements
- Remove `secret` from the JOIN query in `listWebhooksByRepoIDWhereEvent`
- Update all Store and txStore method signatures to match

**Step 4: Update Backend methods**

In `internal/app/backend/webhooks.go`:
- Remove `secret string` parameter from `Backend.CreateWebhook` and `Backend.UpdateWebhook`
- Remove `secret` from the store calls within those methods

**Step 5: Remove HMAC signing from webhook delivery**

In `internal/infra/webhook/webhook.go`:
- Remove `crypto/hmac`, `crypto/sha256`, `encoding/hex` imports
- Remove the HMAC signing block that adds the `X-SoftServe-Signature` header

In `internal/infra/webhook/ssrf_test.go`:
- Remove `Secret: ""` from the test webhook struct literal

**Verification:**
```bash
cd /home/kazw/Work/WorkFort/combine/lead && go build ./...
```

**Commit:** `refactor: remove webhook secret from domain, stores, and delivery`

---

## Task 1: Webhook REST API handlers

**Files:**
- Create: `internal/infra/httpapi/api_webhooks.go`
- Modify: `internal/infra/httpapi/server.go`

**Step 1: Create api_webhooks.go with request/response types**

In `internal/infra/httpapi/api_webhooks.go` (package `web`):

```go
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
```

**Step 2: Add route registration and helper functions**

```go
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
```

Check that `webhook.ContentTypeJSON` and `webhook.ContentTypeForm` are exported. If they are not, look at `internal/infra/webhook/` for the actual constant names and use those.

Build the `webhookToResponse` helper:

```go
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
```

**Step 3: Implement handleCreateWebhook**

```go
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

	// Parse event strings to ints
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
```

**Step 4: Implement handleListWebhooks**

```go
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
```

**Step 5: Implement handleGetWebhook**

```go
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
```

**Step 6: Implement handleUpdateWebhook**

```go
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

	// Update events if provided
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

		// Delete existing events and recreate
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
```

**Step 7: Implement handleDeleteWebhook**

```go
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
```

**Step 8: Register routes in server.go**

In `internal/infra/httpapi/server.go`, add `RegisterWebhookRoutes(api)` inside the `if passport != nil` block, before `RegisterRepoRoutes(api)` (because the `{repo:.+}` pattern is greedy):

```go
RegisterIssueRoutes(api)
RegisterPullRequestRoutes(api)
RegisterWebhookRoutes(api)  // <-- add this line
RegisterRepoRoutes(api)
RegisterKeyRoutes(api)
```

**Step 9: Verify ContentType constants are accessible**

Check that `webhook.ContentTypeJSON` and `webhook.ContentTypeForm` exist. If they are unexported (e.g., `contentTypeJSON`), either export them or use the integer values directly (0 for JSON, 1 for form). Adapt `contentTypeFromString`/`contentTypeToString` accordingly.

**Verification:**
```bash
cd /home/kazw/Work/WorkFort/combine/lead && go build ./...
```

**Commit:** `feat: add webhook registration REST API for Flow integration`

---

## Task 2: E2E tests for webhook registration

**Files:**
- Modify: `tests/e2e/combine_test.go`
- Possibly modify: `tests/e2e/harness/api_client.go`

**Step 1: Add webhook helper methods to API client**

Add these methods to the harness `APIClient` (in `tests/e2e/harness/api_client.go`):

```go
func (c *APIClient) CreateWebhook(t *testing.T, repo string, url string, events []string) map[string]any {
	t.Helper()
	body := map[string]any{
		"url":    url,
		"events": events,
		"active": true,
	}
	resp := c.DoRequest(t, "POST", "/api/v1/repos/"+repo+"/webhooks", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST webhooks: status %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (c *APIClient) ListWebhooks(t *testing.T, repo string) []map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", "/api/v1/repos/"+repo+"/webhooks", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET webhooks: status %d", resp.StatusCode)
	}
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (c *APIClient) GetWebhook(t *testing.T, repo string, id int64) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET webhook: status %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (c *APIClient) UpdateWebhook(t *testing.T, repo string, id int64, body map[string]any) map[string]any {
	t.Helper()
	resp := c.DoRequest(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH webhook: status %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func (c *APIClient) DeleteWebhook(t *testing.T, repo string, id int64) {
	t.Helper()
	resp := c.DoRequest(t, "DELETE", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE webhook: status %d", resp.StatusCode)
	}
}
```

**Step 2: Add E2E test for webhook CRUD**

Add to `tests/e2e/combine_test.go`:

```go
func TestWebhookCRUD(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)
	client := d.APIClient(t, "testuser")
	client.CreateRepo(t, "webhook-test", false)

	// Create webhook
	wh := client.CreateWebhook(t, "webhook-test", "http://example.com/hook", []string{"push", "issue_opened"})
	if wh["url"] != "http://example.com/hook" {
		t.Errorf("url = %v", wh["url"])
	}
	events, _ := wh["events"].([]any)
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if wh["active"] != true {
		t.Errorf("active = %v", wh["active"])
	}

	// Get webhook ID
	whID := int64(wh["id"].(float64))

	// Get webhook
	got := client.GetWebhook(t, "webhook-test", whID)
	if got["url"] != "http://example.com/hook" {
		t.Errorf("get url = %v", got["url"])
	}

	// List webhooks
	list := client.ListWebhooks(t, "webhook-test")
	if len(list) != 1 {
		t.Errorf("expected 1 webhook, got %d", len(list))
	}

	// Update webhook
	updated := client.UpdateWebhook(t, "webhook-test", whID, map[string]any{
		"url":    "http://example.com/hook2",
		"events": []string{"push"},
		"active": false,
	})
	if updated["url"] != "http://example.com/hook2" {
		t.Errorf("updated url = %v", updated["url"])
	}
	if updated["active"] != false {
		t.Errorf("updated active = %v", updated["active"])
	}
	updatedEvents, _ := updated["events"].([]any)
	if len(updatedEvents) != 1 {
		t.Errorf("expected 1 event after update, got %d", len(updatedEvents))
	}

	// Delete webhook
	client.DeleteWebhook(t, "webhook-test", whID)

	list = client.ListWebhooks(t, "webhook-test")
	if len(list) != 0 {
		t.Errorf("expected 0 webhooks after delete, got %d", len(list))
	}
}

func TestWebhookInvalidEvent(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)
	client := d.APIClient(t, "testuser")
	client.CreateRepo(t, "webhook-invalid", false)

	resp := client.DoRequest(t, "POST", "/api/v1/repos/webhook-invalid/webhooks", map[string]any{
		"url":    "http://example.com/hook",
		"events": []string{"nonexistent_event"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid event, got %d", resp.StatusCode)
	}
}
```

**Verification:**
```bash
cd /home/kazw/Work/WorkFort/combine/lead && go test ./tests/e2e/ -run TestWebhook -count=1 -v
```

**Commit:** `test: add E2E tests for webhook registration API`

---

## Task 3: Update remaining-features.md

**Files:**
- Modify: `docs/remaining-features.md`

**Step 1: Update feature 8 (Flow Integration) to show progress**

Change the `## 8. Flow Integration` section to:

```markdown
## 8. Flow Integration ✅

[Design](2026-04-05-flow-integration-design.md) · [Plan](plans/2026-04-05-flow-integration-plan.md)

Webhook registration REST API (`/api/v1/repos/{repo}/webhooks`) for
programmatic webhook management. Five endpoints: create, list, get, update,
delete. Events specified as strings, stored as integers. Push webhook payload
already includes commit details (SHA, message, author, timestamp).
Commit-issue linking handled by Flow's Combine adapter using push webhook
data.
```

**Verification:**
- Check the markdown renders correctly
- Confirm all prior sections remain unchanged

**Commit:** `docs: mark Flow integration as complete in remaining-features`
