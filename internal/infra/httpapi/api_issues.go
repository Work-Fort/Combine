package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
	"github.com/gorilla/mux"
)

type createIssueRequest struct {
	Title      string   `json:"title"`
	Body       string   `json:"body,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	AssigneeID *string  `json:"assignee_id,omitempty"`
}

type updateIssueRequest struct {
	Title      *string  `json:"title,omitempty"`
	Body       *string  `json:"body,omitempty"`
	Status     *string  `json:"status,omitempty"`
	Resolution *string  `json:"resolution,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	AssigneeID *string  `json:"assignee_id,omitempty"`
}

type createCommentRequest struct {
	Body string `json:"body"`
}

type identityRef struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type issueResponse struct {
	Number     int64        `json:"number"`
	Title      string       `json:"title"`
	Body       string       `json:"body"`
	Status     string       `json:"status"`
	Resolution string       `json:"resolution"`
	Author     identityRef  `json:"author"`
	Assignee   *identityRef `json:"assignee,omitempty"`
	Labels     []string     `json:"labels"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	ClosedAt   *time.Time   `json:"closed_at,omitempty"`
}

type commentResponse struct {
	ID        int64       `json:"id"`
	Author    identityRef `json:"author"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// RegisterIssueRoutes registers the issue REST API routes on the given router.
func RegisterIssueRoutes(r *mux.Router) {
	r.HandleFunc("/repos/{repo:.+}/issues", handleListIssues).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/issues", handleCreateIssue).Methods("POST")
	r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}", handleGetIssue).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}", handleUpdateIssue).Methods("PATCH")
	r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}/comments", handleListComments).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}/comments", handleCreateComment).Methods("POST")
}

func resolveIdentity(ctx context.Context, store domain.Store, id string) identityRef {
	ident, err := store.GetIdentityByID(ctx, id)
	if err != nil {
		return identityRef{ID: id}
	}
	return identityRef{ID: ident.ID, Username: ident.Username}
}

func issueToResponse(ctx context.Context, store domain.Store, issue *domain.Issue) issueResponse {
	resp := issueResponse{
		Number:     issue.Number,
		Title:      issue.Title,
		Body:       issue.Body,
		Status:     string(issue.Status),
		Resolution: string(issue.Resolution),
		Author:     resolveIdentity(ctx, store, issue.AuthorID),
		Labels:     issue.Labels,
		CreatedAt:  issue.CreatedAt,
		UpdatedAt:  issue.UpdatedAt,
		ClosedAt:   issue.ClosedAt,
	}
	if resp.Labels == nil {
		resp.Labels = []string{}
	}
	if issue.AssigneeID != nil {
		ref := resolveIdentity(ctx, store, *issue.AssigneeID)
		resp.Assignee = &ref
	}
	return resp
}

func handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]

	var req createIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	issue := domain.Issue{
		RepoID:     repo.ID,
		AuthorID:   identity.ID,
		Title:      req.Title,
		Body:       req.Body,
		Status:     domain.IssueStatusOpen,
		AssigneeID: req.AssigneeID,
	}

	if err := store.CreateIssue(ctx, &issue); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create issue"})
		return
	}

	if len(req.Labels) > 0 {
		if err := store.SetIssueLabels(ctx, issue.ID, req.Labels); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set labels"})
			return
		}
		issue.Labels = req.Labels
	}

	writeJSON(w, http.StatusCreated, issueToResponse(ctx, store, &issue))

	if wh, err := webhook.NewIssueOpenedEvent(ctx, identity, repo, &issue); err == nil {
		webhook.SendEvent(ctx, wh) //nolint:errcheck
	}
}

func handleGetIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	issue, err := store.GetIssueByNumber(ctx, repo.ID, number)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "issue not found"})
		return
	}

	writeJSON(w, http.StatusOK, issueToResponse(ctx, store, issue))
}

func handleListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	var opts domain.IssueListOptions
	if s := r.URL.Query().Get("status"); s != "" {
		status := domain.IssueStatus(s)
		opts.Status = &status
	}
	if l := r.URL.Query().Get("label"); l != "" {
		opts.Label = &l
	}
	if a := r.URL.Query().Get("assignee"); a != "" {
		opts.AssigneeID = &a
	}

	issues, err := store.ListIssues(ctx, repo.ID, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list issues"})
		return
	}

	resp := make([]issueResponse, 0, len(issues))
	for _, issue := range issues {
		resp = append(resp, issueToResponse(ctx, store, issue))
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	var req updateIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	issue, err := store.GetIssueByNumber(ctx, repo.ID, number)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "issue not found"})
		return
	}

	oldStatus := issue.Status

	if req.Title != nil {
		issue.Title = *req.Title
	}
	if req.Body != nil {
		issue.Body = *req.Body
	}
	if req.AssigneeID != nil {
		issue.AssigneeID = req.AssigneeID
	}
	if req.Status != nil {
		newStatus := domain.IssueStatus(*req.Status)
		if newStatus == domain.IssueStatusClosed && issue.Status != domain.IssueStatusClosed {
			now := time.Now()
			issue.ClosedAt = &now
		} else if newStatus != domain.IssueStatusClosed && issue.Status == domain.IssueStatusClosed {
			issue.ClosedAt = nil
		}
		issue.Status = newStatus
	}
	if req.Resolution != nil {
		issue.Resolution = domain.IssueResolution(*req.Resolution)
	}

	if err := store.UpdateIssue(ctx, issue); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update issue"})
		return
	}

	if req.Labels != nil {
		if err := store.SetIssueLabels(ctx, issue.ID, req.Labels); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set labels"})
			return
		}
		issue.Labels = req.Labels
	}

	writeJSON(w, http.StatusOK, issueToResponse(ctx, store, issue))

	if req.Status != nil && domain.IssueStatus(*req.Status) != oldStatus {
		if wh, err := webhook.NewIssueStatusChangedEvent(ctx, identity, repo, issue, string(oldStatus), *req.Status); err == nil {
			webhook.SendEvent(ctx, wh) //nolint:errcheck
		}
		if issue.Status == domain.IssueStatusClosed {
			if wh, err := webhook.NewIssueClosedEvent(ctx, identity, repo, issue, string(issue.Resolution)); err == nil {
				webhook.SendEvent(ctx, wh) //nolint:errcheck
			}
		}
	}
}

func handleListComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	issue, err := store.GetIssueByNumber(ctx, repo.ID, number)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "issue not found"})
		return
	}

	comments, err := store.ListIssueComments(ctx, issue.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list comments"})
		return
	}

	resp := make([]commentResponse, 0, len(comments))
	for _, c := range comments {
		resp = append(resp, commentResponse{
			ID:        c.ID,
			Author:    resolveIdentity(ctx, store, c.AuthorID),
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleCreateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	var req createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body is required"})
		return
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	issue, err := store.GetIssueByNumber(ctx, repo.ID, number)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "issue not found"})
		return
	}

	comment := domain.IssueComment{
		IssueID:  issue.ID,
		AuthorID: identity.ID,
		Body:     req.Body,
	}

	if err := store.CreateIssueComment(ctx, &comment); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create comment"})
		return
	}

	writeJSON(w, http.StatusCreated, commentResponse{
		ID:        comment.ID,
		Author:    resolveIdentity(ctx, store, comment.AuthorID),
		Body:      comment.Body,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	})

	if wh, err := webhook.NewIssueCommentEvent(ctx, identity, repo, issue, &comment); err == nil {
		webhook.SendEvent(ctx, wh) //nolint:errcheck
	}
}
