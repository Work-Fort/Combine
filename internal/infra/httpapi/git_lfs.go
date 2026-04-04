package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/legacy/config"
	"github.com/Work-Fort/Combine/internal/infra/lfs"
	"github.com/Work-Fort/Combine/internal/infra/storage"
	"github.com/gorilla/mux"
)

// serviceLfsBatch handles a Git LFS batch requests.
func serviceLfsBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx).WithPrefix("http.lfs")

	if !isLfs(r) {
		logger.Errorf("invalid content type: %s", r.Header.Get("Content-Type"))
		renderNotAcceptable(w)
		return
	}

	var batchRequest lfs.BatchRequest
	defer r.Body.Close() //nolint: errcheck
	if err := json.NewDecoder(r.Body).Decode(&batchRequest); err != nil {
		logger.Errorf("error decoding json: %s", err)
		renderJSON(w, http.StatusUnprocessableEntity, lfs.ErrorResponse{
			Message: "validation error in request: " + err.Error(),
		})
		return
	}

	if len(batchRequest.Transfers) > 0 {
		var isBasic bool
		for _, t := range batchRequest.Transfers {
			if t == lfs.TransferBasic {
				isBasic = true
				break
			}
		}

		if !isBasic {
			renderJSON(w, http.StatusUnprocessableEntity, lfs.ErrorResponse{
				Message: "unsupported transfer",
			})
			return
		}
	}

	if len(batchRequest.Objects) == 0 {
		renderJSON(w, http.StatusUnprocessableEntity, lfs.ErrorResponse{
			Message: "no objects found",
		})
		return
	}

	name := mux.Vars(r)["repo"]
	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	cfg := config.FromContext(ctx)
	datastore := domain.StoreFromContext(ctx)
	repoID := strconv.FormatInt(repo.ID, 10)
	strg := storage.NewLocalStorage(filepath.Join(cfg.DataPath, "lfs", repoID))

	baseHref := fmt.Sprintf("%s/%s/info/lfs/objects/basic", cfg.HTTP.PublicURL, name+".git")

	var batchResponse lfs.BatchResponse
	batchResponse.Transfer = lfs.TransferBasic
	batchResponse.HashAlgo = lfs.HashAlgorithmSHA256

	objects := make([]*lfs.ObjectResponse, 0, len(batchRequest.Objects))
	switch batchRequest.Operation {
	case lfs.OperationDownload:
		for _, o := range batchRequest.Objects {
			exist, err := strg.Exists(path.Join("objects", o.RelativePath()))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				logger.Error("error getting object stat", "oid", o.Oid, "repo", name, "err", err)
				renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
					Message: "internal server error",
				})
				return
			}

			obj, err := datastore.GetLFSObjectByOid(ctx, repo.ID, o.Oid)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				logger.Error("error getting object from database", "oid", o.Oid, "repo", name, "err", err)
				renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
					Message: "internal server error",
				})
				return
			}

			if !exist {
				objects = append(objects, &lfs.ObjectResponse{
					Pointer: o,
					Error: &lfs.ObjectError{
						Code:    http.StatusNotFound,
						Message: "object not found",
					},
				})
			} else if obj != nil && obj.Size != o.Size {
				objects = append(objects, &lfs.ObjectResponse{
					Pointer: o,
					Error: &lfs.ObjectError{
						Code:    http.StatusUnprocessableEntity,
						Message: "size mismatch",
					},
				})
			} else if o.IsValid() {
				download := &lfs.Link{
					Href: fmt.Sprintf("%s/%s", baseHref, o.Oid),
				}
				if auth := r.Header.Get("Authorization"); auth != "" {
					download.Header = map[string]string{
						"Authorization": auth,
					}
				}

				objects = append(objects, &lfs.ObjectResponse{
					Pointer: o,
					Actions: map[string]*lfs.Link{
						lfs.ActionDownload: download,
					},
				})

				// If the object doesn't exist in the database, create it
				if exist && (obj == nil || obj.ID == 0) {
					if err := datastore.CreateLFSObject(ctx, repo.ID, o.Oid, o.Size); err != nil {
						logger.Error("error creating object in datastore", "oid", o.Oid, "repo", name, "err", err)
						renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
							Message: "internal server error",
						})
						return
					}
				}
			} else {
				logger.Error("invalid object", "oid", o.Oid, "repo", name)
				objects = append(objects, &lfs.ObjectResponse{
					Pointer: o,
					Error: &lfs.ObjectError{
						Code:    http.StatusUnprocessableEntity,
						Message: "invalid object",
					},
				})
			}
		}
	case lfs.OperationUpload:
		accessLevel := domain.AccessLevelFromContext(ctx)
		if accessLevel < domain.ReadWriteAccess {
			askCredentials(w, r)
			renderJSON(w, http.StatusForbidden, lfs.ErrorResponse{
				Message: "write access required",
			})
			return
		}

		for _, o := range batchRequest.Objects {
			if !o.IsValid() {
				objects = append(objects, &lfs.ObjectResponse{
					Pointer: o,
					Error: &lfs.ObjectError{
						Code:    http.StatusUnprocessableEntity,
						Message: "invalid object",
					},
				})
			} else {
				upload := &lfs.Link{
					Href: fmt.Sprintf("%s/%s", baseHref, o.Oid),
					Header: map[string]string{
						"Content-Type": "application/octet-stream",
					},
				}
				verify := &lfs.Link{
					Href: fmt.Sprintf("%s/verify", baseHref),
				}
				if auth := r.Header.Get("Authorization"); auth != "" {
					upload.Header["Authorization"] = auth
					verify.Header = map[string]string{
						"Authorization": auth,
					}
				}

				objects = append(objects, &lfs.ObjectResponse{
					Pointer: o,
					Actions: map[string]*lfs.Link{
						lfs.ActionUpload: upload,
						lfs.ActionVerify: verify,
					},
				})
			}
		}
	default:
		renderJSON(w, http.StatusUnprocessableEntity, lfs.ErrorResponse{
			Message: "unsupported operation",
		})
		return
	}

	batchResponse.Objects = objects
	renderJSON(w, http.StatusOK, batchResponse)
}

func serviceLfsBasic(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serviceLfsBasicDownload(w, r)
	case http.MethodPut:
		serviceLfsBasicUpload(w, r)
	}
}

func serviceLfsBasicDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	oid := mux.Vars(r)["oid"]
	repo := domain.RepoFromContext(ctx)
	cfg := config.FromContext(ctx)
	logger := log.FromContext(ctx).WithPrefix("http.lfs-basic")
	datastore := domain.StoreFromContext(ctx)
	repoID := strconv.FormatInt(repo.ID, 10)
	strg := storage.NewLocalStorage(filepath.Join(cfg.DataPath, "lfs", repoID))

	obj, err := datastore.GetLFSObjectByOid(ctx, repo.ID, oid)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		logger.Error("error getting object from database", "oid", oid, "repo", repo.Name, "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	pointer := lfs.Pointer{Oid: oid}
	f, err := strg.Open(path.Join("objects", pointer.RelativePath()))
	if err != nil {
		logger.Error("error opening object", "oid", oid, "err", err)
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "object not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if obj != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	}
	defer f.Close() //nolint: errcheck
	if _, err := io.Copy(w, f); err != nil {
		logger.Error("error copying object to response", "oid", oid, "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}
}

func serviceLfsBasicUpload(w http.ResponseWriter, r *http.Request) {
	if !isBinary(r) {
		renderJSON(w, http.StatusUnsupportedMediaType, lfs.ErrorResponse{
			Message: "invalid content type",
		})
		return
	}

	ctx := r.Context()
	oid := mux.Vars(r)["oid"]
	cfg := config.FromContext(ctx)
	be := backend.FromContext(ctx)
	datastore := domain.StoreFromContext(ctx)
	logger := log.FromContext(ctx).WithPrefix("http.lfs-basic")
	repo := domain.RepoFromContext(ctx)
	repoID := strconv.FormatInt(repo.ID, 10)
	strg := storage.NewLocalStorage(filepath.Join(cfg.DataPath, "lfs", repoID))
	name := mux.Vars(r)["repo"]

	defer r.Body.Close() //nolint: errcheck
	repo, err := be.Repository(ctx, name)
	if err != nil {
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	// NOTE: Git LFS client will retry uploading the same object if there was a
	// partial error, so we need to skip existing objects.
	if _, err := datastore.GetLFSObjectByOid(ctx, repo.ID, oid); err == nil {
		// Object exists, skip request
		io.Copy(io.Discard, r.Body) //nolint: errcheck
		renderStatus(http.StatusOK)(w, nil)
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		logger.Error("error getting object", "oid", oid, "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	pointer := lfs.Pointer{Oid: oid}
	if _, err := strg.Put(path.Join("objects", pointer.RelativePath()), r.Body); err != nil {
		logger.Error("error writing object", "oid", oid, "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	size, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		logger.Error("error parsing content length", "err", err)
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid content length",
		})
		return
	}

	if err := datastore.CreateLFSObject(ctx, repo.ID, oid, size); err != nil {
		logger.Error("error creating object", "oid", oid, "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	renderStatus(http.StatusOK)(w, nil)
}

func serviceLfsBasicVerify(w http.ResponseWriter, r *http.Request) {
	if !isLfs(r) {
		renderNotAcceptable(w)
		return
	}

	var pointer lfs.Pointer
	ctx := r.Context()
	logger := log.FromContext(ctx).WithPrefix("http.lfs-basic")
	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		logger.Error("error getting repository from context")
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	defer r.Body.Close() //nolint: errcheck
	if err := json.NewDecoder(r.Body).Decode(&pointer); err != nil {
		logger.Error("error decoding json", "err", err)
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid request: " + err.Error(),
		})
		return
	}

	cfg := config.FromContext(ctx)
	datastore := domain.StoreFromContext(ctx)
	repoID := strconv.FormatInt(repo.ID, 10)
	strg := storage.NewLocalStorage(filepath.Join(cfg.DataPath, "lfs", repoID))
	if stat, err := strg.Stat(path.Join("objects", pointer.RelativePath())); err == nil {
		obj, err := datastore.GetLFSObjectByOid(ctx, repo.ID, pointer.Oid)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				logger.Error("object not found", "oid", pointer.Oid)
				renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
					Message: "object not found",
				})
				return
			}
			logger.Error("error getting object", "oid", pointer.Oid, "err", err)
			renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
				Message: "internal server error",
			})
			return
		}

		if obj.Size != pointer.Size {
			renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
				Message: "object size mismatch",
			})
			return
		}

		if pointer.IsValid() && stat.Size() == pointer.Size {
			renderStatus(http.StatusOK)(w, nil)
			return
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		logger.Error("file not found", "oid", pointer.Oid)
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "object not found",
		})
		return
	} else {
		logger.Error("error getting object", "oid", pointer.Oid, "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}
}

func serviceLfsLocks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serviceLfsLocksGet(w, r)
	case http.MethodPost:
		serviceLfsLocksCreate(w, r)
	default:
		renderMethodNotAllowed(w, r)
	}
}

func serviceLfsLocksCreate(w http.ResponseWriter, r *http.Request) {
	if !isLfs(r) {
		renderNotAcceptable(w)
		return
	}

	ctx := r.Context()
	logger := log.FromContext(ctx).WithPrefix("http.lfs-locks")

	var req lfs.LockCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("error decoding json", "err", err)
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid request: " + err.Error(),
		})
		return
	}

	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		logger.Error("error getting repository from context")
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	user := domain.UserFromContext(ctx)
	if user == nil {
		logger.Error("error getting user from context")
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "user not found",
		})
		return
	}

	datastore := domain.StoreFromContext(ctx)
	if err := datastore.CreateLFSLockForUser(ctx, repo.ID, user.ID, req.Path, req.Ref.Name); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			errResp := lfs.LockResponse{
				ErrorResponse: lfs.ErrorResponse{
					Message: "lock already exists",
				},
			}
			lock, err := datastore.GetLFSLockForUserPath(ctx, repo.ID, user.ID, req.Path)
			if err == nil {
				errResp.Lock = lfs.Lock{
					ID:       strconv.FormatInt(lock.ID, 10),
					Path:     lock.Path,
					LockedAt: lock.CreatedAt,
				}
				lockOwner := lfs.Owner{
					Name: user.Username,
				}
				if lock.UserID != user.ID {
					owner, err := datastore.GetUserByID(ctx, lock.UserID)
					if err != nil {
						logger.Error("error getting lock owner", "err", err)
						renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
							Message: "internal server error",
						})
						return
					}
					lockOwner.Name = owner.Username
				}
				errResp.Lock.Owner = lockOwner
			}
			renderJSON(w, http.StatusConflict, errResp)
			return
		}
		logger.Error("error creating lock", "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	lock, err := datastore.GetLFSLockForUserPath(ctx, repo.ID, user.ID, req.Path)
	if err != nil {
		logger.Error("error getting lock", "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	renderJSON(w, http.StatusCreated, lfs.LockResponse{
		Lock: lfs.Lock{
			ID:       strconv.FormatInt(lock.ID, 10),
			Path:     lock.Path,
			LockedAt: lock.CreatedAt,
			Owner: lfs.Owner{
				Name: user.Username,
			},
		},
	})
}

func serviceLfsLocksGet(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if !strings.HasPrefix(accept, lfs.MediaType) {
		renderNotAcceptable(w)
		return
	}

	parseLocksQuery := func(values url.Values) (path string, id int64, cursor int, limit int, refspec string) {
		path = values.Get("path")
		idStr := values.Get("id")
		if idStr != "" {
			id, _ = strconv.ParseInt(idStr, 10, 64)
		}
		cursorStr := values.Get("cursor")
		if cursorStr != "" {
			cursor, _ = strconv.Atoi(cursorStr)
		}
		limitStr := values.Get("limit")
		if limitStr != "" {
			limit, _ = strconv.Atoi(limitStr)
		}
		refspec = values.Get("refspec")
		return
	}

	ctx := r.Context()
	path, id, cursor, limit, _ := parseLocksQuery(r.URL.Query())
	if limit > 100 {
		limit = 100
	} else if limit <= 0 {
		limit = lfs.DefaultLocksLimit
	}

	if cursor <= 0 {
		cursor = 1
	}

	logger := log.FromContext(ctx).WithPrefix("http.lfs-locks")
	datastore := domain.StoreFromContext(ctx)
	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		logger.Error("error getting repository from context")
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	if id > 0 {
		lock, err := datastore.GetLFSLockByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
					Message: "lock not found",
				})
				return
			}
			logger.Error("error getting lock", "err", err)
			renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
				Message: "internal server error",
			})
			return
		}

		owner, err := datastore.GetUserByID(ctx, lock.UserID)
		if err != nil {
			logger.Error("error getting lock owner", "err", err)
			renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
				Message: "internal server error",
			})
			return
		}

		renderJSON(w, http.StatusOK, lfs.LockListResponse{
			Locks: []lfs.Lock{
				{
					ID:       strconv.FormatInt(lock.ID, 10),
					Path:     lock.Path,
					LockedAt: lock.CreatedAt,
					Owner: lfs.Owner{
						Name: owner.Username,
					},
				},
			},
		})
		return
	} else if path != "" {
		lock, err := datastore.GetLFSLockForPath(ctx, repo.ID, path)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
					Message: "lock not found",
				})
				return
			}
			logger.Error("error getting lock", "err", err)
			renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
				Message: "internal server error",
			})
			return
		}

		owner, err := datastore.GetUserByID(ctx, lock.UserID)
		if err != nil {
			logger.Error("error getting lock owner", "err", err)
			renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
				Message: "internal server error",
			})
			return
		}

		renderJSON(w, http.StatusOK, lfs.LockListResponse{
			Locks: []lfs.Lock{
				{
					ID:       strconv.FormatInt(lock.ID, 10),
					Path:     lock.Path,
					LockedAt: lock.CreatedAt,
					Owner: lfs.Owner{
						Name: owner.Username,
					},
				},
			},
		})
		return
	}

	locks, err := datastore.ListLFSLocks(ctx, repo.ID, cursor, limit)
	if err != nil {
		logger.Error("error getting locks", "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	lockList := make([]lfs.Lock, len(locks))
	users := map[int64]*domain.User{}
	for i, lock := range locks {
		owner, ok := users[lock.UserID]
		if !ok {
			owner, err = datastore.GetUserByID(ctx, lock.UserID)
			if err != nil {
				logger.Error("error getting lock owner", "err", err)
				renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
					Message: "internal server error",
				})
				return
			}
			users[lock.UserID] = owner
		}

		lockList[i] = lfs.Lock{
			ID:       strconv.FormatInt(lock.ID, 10),
			Path:     lock.Path,
			LockedAt: lock.CreatedAt,
			Owner: lfs.Owner{
				Name: owner.Username,
			},
		}
	}

	resp := lfs.LockListResponse{
		Locks: lockList,
	}
	if len(locks) == limit {
		resp.NextCursor = strconv.Itoa(cursor + 1)
	}

	renderJSON(w, http.StatusOK, resp)
}

func serviceLfsLocksVerify(w http.ResponseWriter, r *http.Request) {
	if !isLfs(r) {
		renderNotAcceptable(w)
		return
	}

	ctx := r.Context()
	logger := log.FromContext(ctx).WithPrefix("http.lfs-locks")
	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		logger.Error("error getting repository from context")
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	var req lfs.LockVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("error decoding request", "err", err)
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid request: " + err.Error(),
		})
		return
	}

	cursor, _ := strconv.Atoi(req.Cursor)
	if cursor <= 0 {
		cursor = 1
	}

	limit := req.Limit
	if limit > 100 {
		limit = 100
	} else if limit <= 0 {
		limit = lfs.DefaultLocksLimit
	}

	datastore := domain.StoreFromContext(ctx)
	user := domain.UserFromContext(ctx)
	ours := make([]lfs.Lock, 0)
	theirs := make([]lfs.Lock, 0)

	var resp lfs.LockVerifyResponse
	locks, err := datastore.ListLFSLocks(ctx, repo.ID, cursor, limit)
	if err != nil {
		logger.Error("error getting locks", "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	users := map[int64]*domain.User{}
	for _, lock := range locks {
		owner, ok := users[lock.UserID]
		if !ok {
			owner, err = datastore.GetUserByID(ctx, lock.UserID)
			if err != nil {
				logger.Error("error getting lock owner", "err", err)
				renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
					Message: "internal server error",
				})
				return
			}
			users[lock.UserID] = owner
		}

		l := lfs.Lock{
			ID:       strconv.FormatInt(lock.ID, 10),
			Path:     lock.Path,
			LockedAt: lock.CreatedAt,
			Owner: lfs.Owner{
				Name: owner.Username,
			},
		}

		if user != nil && user.ID == lock.UserID {
			ours = append(ours, l)
		} else {
			theirs = append(theirs, l)
		}
	}

	resp.Ours = ours
	resp.Theirs = theirs

	if len(locks) == limit {
		resp.NextCursor = strconv.Itoa(cursor + 1)
	}

	renderJSON(w, http.StatusOK, resp)
}

func serviceLfsLocksDelete(w http.ResponseWriter, r *http.Request) {
	if !isLfs(r) {
		renderNotAcceptable(w)
		return
	}

	ctx := r.Context()
	logger := log.FromContext(ctx).WithPrefix("http.lfs-locks")
	lockIDStr := mux.Vars(r)["lock_id"]
	if lockIDStr == "" {
		logger.Error("error getting lock id")
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid request",
		})
		return
	}

	lockID, err := strconv.ParseInt(lockIDStr, 10, 64)
	if err != nil {
		logger.Error("error parsing lock id", "err", err)
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid request",
		})
		return
	}

	var req lfs.LockDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("error decoding request", "err", err)
		renderJSON(w, http.StatusBadRequest, lfs.ErrorResponse{
			Message: "invalid request: " + err.Error(),
		})
		return
	}

	datastore := domain.StoreFromContext(ctx)
	repo := domain.RepoFromContext(ctx)
	if repo == nil {
		logger.Error("error getting repository from context")
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "repository not found",
		})
		return
	}

	lock, err := datastore.GetLFSLockByID(ctx, lockID)
	if err != nil {
		logger.Error("error getting lock", "err", err)
		renderJSON(w, http.StatusNotFound, lfs.ErrorResponse{
			Message: "lock not found",
		})
		return
	}

	owner, err := datastore.GetUserByID(ctx, lock.UserID)
	if err != nil {
		logger.Error("error getting lock owner", "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	l := lfs.Lock{
		ID:       strconv.FormatInt(lock.ID, 10),
		Path:     lock.Path,
		LockedAt: lock.CreatedAt,
		Owner: lfs.Owner{
			Name: owner.Username,
		},
	}

	user := domain.UserFromContext(ctx)
	if user == nil {
		logger.Error("error getting user from context")
		renderJSON(w, http.StatusUnauthorized, lfs.ErrorResponse{
			Message: "unauthorized",
		})
		return
	}

	if req.Force {
		if !user.Admin {
			logger.Error("non-admin user attempted force delete", "user", user.Username)
			renderJSON(w, http.StatusForbidden, lfs.ErrorResponse{
				Message: "admin access required for force delete",
			})
			return
		}

		if err := datastore.DeleteLFSLock(ctx, repo.ID, lockID); err != nil {
			logger.Error("error deleting lock", "err", err)
			renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
				Message: "internal server error",
			})
			return
		}

		renderJSON(w, http.StatusOK, l)
		return
	}

	if owner.ID != user.ID {
		logger.Error("error deleting another user's lock")
		renderJSON(w, http.StatusForbidden, lfs.ErrorResponse{
			Message: "lock belongs to another user",
		})
		return
	}

	if err := datastore.DeleteLFSLock(ctx, repo.ID, lockID); err != nil {
		logger.Error("error deleting lock", "err", err)
		renderJSON(w, http.StatusInternalServerError, lfs.ErrorResponse{
			Message: "internal server error",
		})
		return
	}

	renderJSON(w, http.StatusOK, lfs.LockResponse{Lock: l})
}

func renderJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	hdrLfs(w)
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error("error encoding json", "err", err)
	}
}

func renderNotAcceptable(w http.ResponseWriter) {
	renderStatus(http.StatusNotAcceptable)(w, nil)
}

func isLfs(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	accept := r.Header.Get("Accept")
	return strings.HasPrefix(contentType, lfs.MediaType) && strings.HasPrefix(accept, lfs.MediaType)
}

func isBinary(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "application/octet-stream")
}

func hdrLfs(w http.ResponseWriter) {
	w.Header().Set("Content-Type", lfs.MediaType)
	w.Header().Set("Accept", lfs.MediaType)
}
