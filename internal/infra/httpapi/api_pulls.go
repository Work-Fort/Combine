package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/gorilla/mux"
)

type createPullRequestRequest struct {
	Title        string  `json:"title"`
	Body         string  `json:"body,omitempty"`
	SourceBranch string  `json:"source_branch"`
	TargetBranch string  `json:"target_branch"`
	AssigneeID   *string `json:"assignee_id,omitempty"`
}

type updatePullRequestRequest struct {
	Title      *string `json:"title,omitempty"`
	Body       *string `json:"body,omitempty"`
	Status     *string `json:"status,omitempty"`
	AssigneeID *string `json:"assignee_id,omitempty"`
}

type pullRequestResponse struct {
	Number       int64        `json:"number"`
	Title        string       `json:"title"`
	Body         string       `json:"body"`
	SourceBranch string       `json:"source_branch"`
	TargetBranch string       `json:"target_branch"`
	Status       string       `json:"status"`
	MergeMethod  *string      `json:"merge_method,omitempty"`
	Author       identityRef  `json:"author"`
	MergedBy     *identityRef `json:"merged_by,omitempty"`
	Assignee     *identityRef `json:"assignee,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	MergedAt     *time.Time   `json:"merged_at,omitempty"`
	ClosedAt     *time.Time   `json:"closed_at,omitempty"`
}

// RegisterPullRequestRoutes registers the pull request REST API routes.
func RegisterPullRequestRoutes(r *mux.Router) {
	r.HandleFunc("/repos/{repo:.+}/pulls", handleListPullRequests).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/pulls", handleCreatePullRequest).Methods("POST")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}", handleGetPullRequest).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}", handleUpdatePullRequest).Methods("PATCH")
}

func prToResponse(ctx context.Context, store domain.Store, pr *domain.PullRequest) pullRequestResponse {
	resp := pullRequestResponse{
		Number:       pr.Number,
		Title:        pr.Title,
		Body:         pr.Body,
		SourceBranch: pr.SourceBranch,
		TargetBranch: pr.TargetBranch,
		Status:       string(pr.Status),
		Author:       resolveIdentity(ctx, store, pr.AuthorID),
		CreatedAt:    pr.CreatedAt,
		UpdatedAt:    pr.UpdatedAt,
		MergedAt:     pr.MergedAt,
		ClosedAt:     pr.ClosedAt,
	}
	if pr.MergeMethod != nil {
		mm := string(*pr.MergeMethod)
		resp.MergeMethod = &mm
	}
	if pr.MergedBy != nil {
		ref := resolveIdentity(ctx, store, *pr.MergedBy)
		resp.MergedBy = &ref
	}
	if pr.AssigneeID != nil {
		ref := resolveIdentity(ctx, store, *pr.AssigneeID)
		resp.Assignee = &ref
	}
	return resp
}

func handleCreatePullRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]

	var req createPullRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if req.SourceBranch == "" || req.TargetBranch == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_branch and target_branch are required"})
		return
	}
	if req.SourceBranch == req.TargetBranch {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source and target branches must differ"})
		return
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	// TODO (plan 7b): Validate that source and target branches exist in the git repo.

	pr := domain.PullRequest{
		RepoID:       repo.ID,
		AuthorID:     identity.ID,
		Title:        req.Title,
		Body:         req.Body,
		SourceBranch: req.SourceBranch,
		TargetBranch: req.TargetBranch,
		Status:       domain.PullRequestStatusOpen,
		AssigneeID:   req.AssigneeID,
	}

	if err := store.CreatePullRequest(ctx, &pr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create pull request"})
		return
	}

	writeJSON(w, http.StatusCreated, prToResponse(ctx, store, &pr))

	// TODO (plan 7b): Fire pull_request_opened webhook event.
}

func handleGetPullRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	pr, err := store.GetPullRequestByNumber(ctx, repo.ID, number)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request not found"})
		return
	}

	// TODO (plan 7b): Include mergeable status in response.
	writeJSON(w, http.StatusOK, prToResponse(ctx, store, pr))
}

func handleListPullRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	var opts domain.PullRequestListOptions
	if s := r.URL.Query().Get("status"); s != "" {
		status := domain.PullRequestStatus(s)
		opts.Status = &status
	}
	if a := r.URL.Query().Get("author"); a != "" {
		opts.AuthorID = &a
	}

	prs, err := store.ListPullRequests(ctx, repo.ID, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list pull requests"})
		return
	}

	resp := make([]pullRequestResponse, 0, len(prs))
	for _, pr := range prs {
		resp = append(resp, prToResponse(ctx, store, pr))
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleUpdatePullRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	var req updatePullRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	repo, err := store.GetRepoByName(ctx, repoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	pr, err := store.GetPullRequestByNumber(ctx, repo.ID, number)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request not found"})
		return
	}

	if req.Title != nil {
		pr.Title = *req.Title
	}
	if req.Body != nil {
		pr.Body = *req.Body
	}
	if req.AssigneeID != nil {
		pr.AssigneeID = req.AssigneeID
	}
	if req.Status != nil {
		newStatus := domain.PullRequestStatus(*req.Status)
		// Only allow closing via PATCH — merging is via POST /merge (plan 7b).
		if newStatus == domain.PullRequestStatusClosed && pr.Status == domain.PullRequestStatusOpen {
			now := time.Now()
			pr.ClosedAt = &now
			pr.Status = newStatus
		} else if newStatus == domain.PullRequestStatusOpen && pr.Status == domain.PullRequestStatusClosed {
			pr.ClosedAt = nil
			pr.Status = newStatus
		}
		// Ignore attempts to set status to "merged" via PATCH.
	}

	if err := store.UpdatePullRequest(ctx, pr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update pull request"})
		return
	}

	writeJSON(w, http.StatusOK, prToResponse(ctx, store, pr))

	// TODO (plan 7b): Fire webhook events on status changes.
}
