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

type reviewCommentRequest struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side,omitempty"` // defaults to "right"
	Body string `json:"body"`
}

type submitReviewRequest struct {
	State    string                 `json:"state"` // "approved", "changes_requested", "commented"
	Body     string                 `json:"body,omitempty"`
	Comments []reviewCommentRequest `json:"comments,omitempty"`
}

type reviewCommentResponse struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	Line      int       `json:"line"`
	Side      string    `json:"side"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type reviewResponse struct {
	ID        int64                   `json:"id"`
	Author    identityRef             `json:"author"`
	State     string                  `json:"state"`
	Body      string                  `json:"body"`
	Comments  []reviewCommentResponse `json:"comments"`
	CreatedAt time.Time               `json:"created_at"`
}

func handleListReviews(w http.ResponseWriter, r *http.Request) {
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

	reviews, err := store.ListReviewsByPRID(ctx, pr.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list reviews"})
		return
	}

	resp := make([]reviewResponse, 0, len(reviews))
	for _, rev := range reviews {
		rr := reviewResponse{
			ID:        rev.ID,
			Author:    resolveIdentity(ctx, store, rev.AuthorID),
			State:     string(rev.State),
			Body:      rev.Body,
			CreatedAt: rev.CreatedAt,
		}
		rr.Comments = make([]reviewCommentResponse, 0, len(rev.Comments))
		for _, c := range rev.Comments {
			rr.Comments = append(rr.Comments, reviewCommentResponse{
				ID:        c.ID,
				Path:      c.Path,
				Line:      c.Line,
				Side:      c.Side,
				Body:      c.Body,
				CreatedAt: c.CreatedAt,
			})
		}
		resp = append(resp, rr)
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := domain.StoreFromContext(ctx)
	repoName := mux.Vars(r)["repo"]
	number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

	var req submitReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	state := domain.ReviewState(req.State)
	if state != domain.ReviewStateApproved &&
		state != domain.ReviewStateChangesRequested &&
		state != domain.ReviewStateCommented {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state must be approved, changes_requested, or commented"})
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
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot review a closed or merged pull request"})
		return
	}

	review := domain.PullRequestReview{
		PRID:     pr.ID,
		AuthorID: identity.ID,
		State:    state,
		Body:     req.Body,
	}

	for _, c := range req.Comments {
		side := c.Side
		if side == "" {
			side = "right"
		}
		review.Comments = append(review.Comments, &domain.ReviewComment{
			Path: c.Path,
			Line: c.Line,
			Side: side,
			Body: c.Body,
		})
	}

	if err := store.CreateReview(ctx, &review); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to submit review"})
		return
	}

	// Build response.
	rr := reviewResponse{
		ID:        review.ID,
		Author:    resolveIdentity(ctx, store, review.AuthorID),
		State:     string(review.State),
		Body:      review.Body,
		CreatedAt: review.CreatedAt,
	}
	rr.Comments = make([]reviewCommentResponse, 0, len(review.Comments))
	for _, c := range review.Comments {
		rr.Comments = append(rr.Comments, reviewCommentResponse{
			ID:        c.ID,
			Path:      c.Path,
			Line:      c.Line,
			Side:      c.Side,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		})
	}
	writeJSON(w, http.StatusCreated, rr)

	// Fire webhook.
	if wh, err := webhook.NewPullRequestReviewEvent(ctx, identity, repo, pr, &review); err == nil {
		webhook.SendEvent(ctx, wh) //nolint:errcheck
	}
}
