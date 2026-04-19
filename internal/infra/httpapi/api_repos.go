package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
)

type createRepoRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Private     bool   `json:"private,omitempty"`
}

type updateRepoRequest struct {
	Description *string `json:"description,omitempty"`
	Private     *bool   `json:"private,omitempty"`
	Hidden      *bool   `json:"hidden,omitempty"`
	ProjectName *string `json:"project_name,omitempty"`
}

type repoResponse struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ProjectName string    `json:"project_name"`
	Private     bool      `json:"private"`
	Mirror      bool      `json:"mirror"`
	Hidden      bool      `json:"hidden"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func repoToResponse(r *domain.Repo) repoResponse {
	return repoResponse{
		Name:        r.Name,
		Description: r.Description,
		ProjectName: r.ProjectName,
		Private:     r.Private,
		Mirror:      r.Mirror,
		Hidden:      r.Hidden,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// RegisterRepoRoutes registers the repo CRUD REST API routes on the given router.
func RegisterRepoRoutes(r *mux.Router) {
	r.HandleFunc("/repos", handleListRepos).Methods("GET")
	r.HandleFunc("/repos", handleCreateRepo).Methods("POST")
	r.HandleFunc("/repos/{repo:.+}", handleGetRepo).Methods("GET")
	r.HandleFunc("/repos/{repo:.+}", handleUpdateRepo).Methods("PATCH")
	r.HandleFunc("/repos/{repo:.+}", handleDeleteRepo).Methods("DELETE")
}

func handleListRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	be := backend.FromContext(ctx)

	repos, err := be.Repositories(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list repositories"})
		return
	}

	resp := make([]repoResponse, 0, len(repos))
	for _, repo := range repos {
		resp = append(resp, repoToResponse(repo))
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	be := backend.FromContext(ctx)

	var req createRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	opts := domain.RepoOptions{
		Description: req.Description,
		Private:     req.Private,
	}

	repo, err := be.CreateRepository(ctx, req.Name, nil, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create repository"})
		return
	}

	writeJSON(w, http.StatusCreated, repoToResponse(repo))
}

func handleGetRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	be := backend.FromContext(ctx)
	name := mux.Vars(r)["repo"]

	repo, err := be.Repository(ctx, name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	writeJSON(w, http.StatusOK, repoToResponse(repo))
}

func handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	be := backend.FromContext(ctx)
	name := mux.Vars(r)["repo"]

	var req updateRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Description != nil {
		if err := be.SetDescription(ctx, name, *req.Description); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update description"})
			return
		}
	}
	if req.Private != nil {
		if err := be.SetPrivate(ctx, name, *req.Private); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update private flag"})
			return
		}
	}
	if req.Hidden != nil {
		if err := be.SetHidden(ctx, name, *req.Hidden); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update hidden flag"})
			return
		}
	}
	if req.ProjectName != nil {
		if err := be.SetProjectName(ctx, name, *req.ProjectName); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update project name"})
			return
		}
	}

	// Fetch the updated repo to return
	repo, err := be.Repository(ctx, name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	writeJSON(w, http.StatusOK, repoToResponse(repo))
}

func handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	be := backend.FromContext(ctx)
	name := mux.Vars(r)["repo"]

	if err := be.DeleteRepository(ctx, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete repository"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
