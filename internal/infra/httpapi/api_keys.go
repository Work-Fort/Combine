package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
	"github.com/gorilla/mux"
)

type addKeyRequest struct {
	Key string `json:"key"`
}

type keyResponse struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterKeyRoutes registers the SSH key management REST API routes.
func RegisterKeyRoutes(r *mux.Router) {
	r.HandleFunc("/user/keys", handleListKeys).Methods("GET")
	r.HandleFunc("/user/keys", handleAddKey).Methods("POST")
	r.HandleFunc("/user/keys/{id}", handleDeleteKey).Methods("DELETE")
}

func handleListKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	store := domain.StoreFromContext(ctx)
	keys, err := store.ListIdentityPublicKeys(ctx, identity.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list keys"})
		return
	}

	resp := make([]keyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, keyResponse{
			ID:        k.ID,
			Key:       k.PublicKey,
			CreatedAt: k.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleAddKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req addKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}

	pk, _, err := sshutils.ParseAuthorizedKey(req.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid SSH public key"})
		return
	}

	store := domain.StoreFromContext(ctx)
	if err := store.AddIdentityPublicKey(ctx, identity.ID, pk); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add key"})
		return
	}

	// Return the newly added key. List and find it by matching the key string.
	keys, err := store.ListIdentityPublicKeys(ctx, identity.ID)
	if err == nil {
		ak := sshutils.MarshalAuthorizedKey(pk)
		for _, k := range keys {
			if k.PublicKey == ak {
				writeJSON(w, http.StatusCreated, keyResponse{
					ID:        k.ID,
					Key:       k.PublicKey,
					CreatedAt: k.CreatedAt,
				})
				return
			}
		}
	}

	// Fallback if we can't find it
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity := domain.IdentityFromContext(ctx)
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	keyIDStr := mux.Vars(r)["id"]
	keyID, err := strconv.ParseInt(keyIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid key ID"})
		return
	}

	store := domain.StoreFromContext(ctx)
	if err := store.RemoveIdentityPublicKey(ctx, identity.ID, keyID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete key"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
