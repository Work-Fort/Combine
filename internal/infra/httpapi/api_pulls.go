package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
	gitpkg "github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
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

type mergePullRequestRequest struct {
	MergeMethod string `json:"merge_method"` // "merge", "squash", "rebase"
	Message     string `json:"message,omitempty"`
}

type pullRequestResponse struct {
	Number       int64        `json:"number"`
	Title        string       `json:"title"`
	Body         string       `json:"body"`
	SourceBranch string       `json:"source_branch"`
	TargetBranch string       `json:"target_branch"`
	Status       string       `json:"status"`
	MergeMethod  *string      `json:"merge_method,omitempty"`
	Mergeable    *bool        `json:"mergeable,omitempty"`
	Author       identityRef  `json:"author"`
	MergedBy     *identityRef `json:"merged_by,omitempty"`
	Assignee     *identityRef `json:"assignee,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	MergedAt     *time.Time   `json:"merged_at,omitempty"`
	ClosedAt     *time.Time   `json:"closed_at,omitempty"`
}

type commitResponse struct {
	SHA     string    `json:"sha"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// RegisterPullRequestRoutes registers the pull request REST API routes.
func RegisterPullRequestRoutes(r *mux.Router) {
	r.HandleFunc("/repos/{repo:.+}/pulls", handleListPullRequests).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/pulls", handleCreatePullRequest).Methods("POST")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}", handleGetPullRequest).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}", handleUpdatePullRequest).Methods("PATCH")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/merge", handleMergePullRequest).Methods("POST")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/diff", handlePullRequestDiff).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/commits", handlePullRequestCommits).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/files", handlePullRequestFiles).Methods("GET")
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

	if wh, err := webhook.NewPullRequestOpenedEvent(ctx, identity, repo, &pr); err == nil {
		webhook.SendEvent(ctx, wh) //nolint:errcheck
	}
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

	resp := prToResponse(ctx, store, pr)
	if pr.Status == domain.PullRequestStatusOpen {
		be := backend.FromContext(ctx)
		if be != nil {
			mergeable, err := be.IsPullRequestMergeable(ctx, repoName, pr.SourceBranch, pr.TargetBranch)
			if err == nil {
				resp.Mergeable = &mergeable
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
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

	var statusChanged bool
	if req.Status != nil {
		newStatus := domain.PullRequestStatus(*req.Status)
		// Only allow closing via PATCH — merging is via POST /merge.
		if newStatus == domain.PullRequestStatusClosed && pr.Status == domain.PullRequestStatusOpen {
			now := time.Now()
			pr.ClosedAt = &now
			pr.Status = newStatus
			statusChanged = true
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

	if statusChanged && pr.Status == domain.PullRequestStatusClosed {
		if wh, err := webhook.NewPullRequestClosedEvent(ctx, identity, repo, pr); err == nil {
			webhook.SendEvent(ctx, wh) //nolint:errcheck
		}
	}
}

func handleMergePullRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	be := backend.FromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	var req mergePullRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	method := domain.MergeMethod(req.MergeMethod)
	if method != domain.MergeMethodMerge && method != domain.MergeMethodSquash && method != domain.MergeMethodRebase {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "merge_method must be merge, squash, or rebase"})
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

	if pr.Status != domain.PullRequestStatusOpen {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pull request is not open"})
		return
	}

	mergeable, err := be.IsPullRequestMergeable(ctx, repoName, pr.SourceBranch, pr.TargetBranch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check mergeability"})
		return
	}
	if !mergeable {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pull request has conflicts"})
		return
	}

	message := req.Message
	if message == "" {
		message = fmt.Sprintf("Merge pull request #%d from %s\n\n%s", pr.Number, pr.SourceBranch, pr.Title)
	}

	if _, err := be.MergePullRequest(ctx, repoName, pr.SourceBranch, pr.TargetBranch, method, message); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "merge failed: " + err.Error()})
		return
	}

	now := time.Now()
	pr.Status = domain.PullRequestStatusMerged
	pr.MergeMethod = &method
	pr.MergedBy = &identity.ID
	pr.MergedAt = &now
	if err := store.UpdatePullRequest(ctx, pr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update pull request"})
		return
	}

	closeReferencedIssues(ctx, store, repo, pr.Body, identity)

	writeJSON(w, http.StatusOK, prToResponse(ctx, store, pr))

	if wh, err := webhook.NewPullRequestMergedEvent(ctx, identity, repo, pr); err == nil {
		webhook.SendEvent(ctx, wh) //nolint:errcheck
	}
}

func handlePullRequestDiff(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	be := backend.FromContext(ctx)
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

	diff, err := be.DiffPullRequest(ctx, repoName, pr.SourceBranch, pr.TargetBranch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute diff"})
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(diff.Patch())) //nolint:errcheck
}

func handlePullRequestCommits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	be := backend.FromContext(ctx)
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

	commits, err := be.PullRequestCommits(ctx, repoName, pr.SourceBranch, pr.TargetBranch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list commits"})
		return
	}

	resp := make([]commitResponse, 0, len(commits))
	for _, c := range commits {
		resp = append(resp, commitResponse{
			SHA:     c.ID.String(),
			Message: c.Message,
			Author:  c.Author.Name,
			Date:    c.Author.When,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func handlePullRequestFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := domain.StoreFromContext(ctx)
	be := backend.FromContext(ctx)
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

	files, err := be.PullRequestFiles(ctx, repoName, pr.SourceBranch, pr.TargetBranch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list changed files"})
		return
	}

	writeJSON(w, http.StatusOK, files)
}

func closeReferencedIssues(ctx context.Context, store domain.Store, repo *domain.Repo, text string, identity *domain.Identity) {
	nums := gitpkg.ParseClosingKeywords(text)
	for _, num := range nums {
		issue, err := store.GetIssueByNumber(ctx, repo.ID, num)
		if err != nil || issue.Status == domain.IssueStatusClosed {
			continue
		}
		now := time.Now()
		issue.Status = domain.IssueStatusClosed
		issue.Resolution = domain.IssueResolutionFixed
		issue.ClosedAt = &now
		if err := store.UpdateIssue(ctx, issue); err != nil {
			continue
		}
		if wh, err := webhook.NewIssueClosedEvent(ctx, identity, repo, issue, "fixed"); err == nil {
			webhook.SendEvent(ctx, wh) //nolint:errcheck
		}
	}
}
