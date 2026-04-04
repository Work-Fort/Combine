# Passport Auth + Repo REST API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate Passport for REST API authentication, replace the User model with Identity, and add repo management and SSH key REST APIs.

**Architecture:** Passport's `service-auth` middleware validates JWT and API key tokens on REST endpoints. A local `identities` table stores Passport-sourced identity records (auto-provisioned on first request). The existing SSH key auth stays in Combine. New REST API at `/api/v1/` provides repo CRUD and SSH key management. Git smart HTTP and SSH transport keep their existing auth paths.

**Tech Stack:** github.com/Work-Fort/Passport/go/service-auth, gorilla/mux, standard encoding/json

---

## Phase A: Domain Model — Identity Replaces User

### Task 1: Add Identity domain type and update ports

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/ports.go`
- Modify: `internal/domain/context.go`
- Modify: `internal/domain/errors.go`

**Step 1: Add Identity type to types.go**

Add after the existing `User` type (don't remove User yet — we'll do that after migration):

```go
// Identity represents a Passport-authenticated identity stored locally.
// Auto-provisioned on first authenticated request.
type Identity struct {
    ID          string    // Passport UUID, primary key
    Username    string    // From Passport claims
    DisplayName string    // From Passport claims
    Type        string    // "user", "agent", "service"
    IsAdmin     bool      // Local admin flag
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

const (
    IdentityTypeUser    = "user"
    IdentityTypeAgent   = "agent"
    IdentityTypeService = "service"
)
```

**Step 2: Add IdentityStore port to ports.go**

```go
// IdentityStore persists Passport identity records.
type IdentityStore interface {
    UpsertIdentity(ctx context.Context, id, username, displayName, identityType string) (*Identity, error)
    GetIdentityByID(ctx context.Context, id string) (*Identity, error)
    GetIdentityByUsername(ctx context.Context, username string) (*Identity, error)
    GetIdentityByPublicKey(ctx context.Context, pk ssh.PublicKey) (*Identity, error)
    ListIdentities(ctx context.Context) ([]*Identity, error)
    SetIdentityAdmin(ctx context.Context, id string, isAdmin bool) error
    AddPublicKey(ctx context.Context, identityID string, pk ssh.PublicKey) error
    RemovePublicKey(ctx context.Context, identityID string, keyID int64) error
    ListPublicKeys(ctx context.Context, identityID string) ([]*PublicKey, error)
}
```

Update `PublicKey` type to use string identity ID:
```go
type PublicKey struct {
    ID         int64
    IdentityID string   // was UserID int64
    Key        string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

Add `IdentityStore` to the composite `Store` interface.

**Step 3: Add Identity context helpers to context.go**

```go
func IdentityFromContext(ctx context.Context) *Identity { ... }
func WithIdentityContext(ctx context.Context, id *Identity) context.Context { ... }
```

**Step 4: Add errors**

```go
var ErrIdentityNotFound = errors.New("identity not found")
```

**Step 5: Verify and commit**

```bash
go build ./internal/domain/
git commit -m "feat: add Identity domain type and IdentityStore port"
```

---

### Task 2: Implement IdentityStore in SQLite adapter

**Files:**
- Create: `internal/infra/sqlite/migrations/002_identities.sql`
- Create: `internal/infra/sqlite/identity.go`

**Step 1: Write the migration**

```sql
-- +goose Up

CREATE TABLE identities (
    id           TEXT PRIMARY KEY,
    username     TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL DEFAULT 'user',
    is_admin     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Migrate existing public_keys to reference identities
-- (old public_keys.user_id becomes text identity_id in a new table)
CREATE TABLE identity_public_keys (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id TEXT NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    public_key  TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(identity_id, public_key)
);

-- +goose Down
DROP TABLE IF EXISTS identity_public_keys;
DROP TABLE IF EXISTS identities;
```

**Step 2: Implement identity.go**

Implement all `IdentityStore` methods. Key method:

`UpsertIdentity` — INSERT on first visit, UPDATE username/displayName/type on
subsequent visits (Passport claims may change). Use `INSERT ... ON CONFLICT(id) DO UPDATE`.

`GetIdentityByPublicKey` — JOIN identities with identity_public_keys.

**Step 3: Write tests**

Add to `internal/infra/sqlite/store_test.go`:
- TestIdentityStore — upsert, get by ID, get by username, list, admin flag, public keys

**Step 4: Verify and commit**

```bash
go test ./internal/infra/sqlite/ -v
git commit -m "feat: implement IdentityStore in SQLite adapter"
```

---

### Task 3: Implement IdentityStore in PostgreSQL adapter

**Files:**
- Create: `internal/infra/postgres/migrations/002_identities.sql`
- Create: `internal/infra/postgres/identity.go`

Same as Task 2 but with PostgreSQL syntax (`$1` placeholders, `TIMESTAMPTZ`,
`INSERT ... ON CONFLICT ... DO UPDATE`).

**Commit:**
```bash
git commit -m "feat: implement IdentityStore in PostgreSQL adapter"
```

---

## Phase B: Passport Auth Middleware

### Task 4: Add Passport service-auth dependency and middleware

**Files:**
- Modify: `go.mod` (add service-auth dependency)
- Create: `internal/infra/httpapi/passport.go`
- Modify: `internal/infra/httpapi/server.go`

**Step 1: Add dependency**

```bash
go get github.com/Work-Fort/Passport/go/service-auth@latest
```

**Step 2: Create passport.go**

```go
package web

import (
    "context"
    "net/http"
    "strings"

    auth "github.com/Work-Fort/Passport/go/service-auth"
    "github.com/Work-Fort/Passport/go/service-auth/apikey"
    "github.com/Work-Fort/Passport/go/service-auth/jwt"
    "github.com/Work-Fort/Combine/internal/domain"
)

// PassportMiddleware wraps handlers with Passport JWT + API key auth.
// It also auto-provisions identities on first request.
type PassportMiddleware struct {
    mw    auth.Middleware
    store domain.Store
    jwtV  *jwt.Validator
}

// NewPassportMiddleware creates the Passport auth middleware.
// Returns nil if passportURL is empty (standalone mode).
func NewPassportMiddleware(ctx context.Context, passportURL string, store domain.Store) (*PassportMiddleware, error) {
    if passportURL == "" {
        return nil, nil
    }

    opts := auth.DefaultOptions(passportURL)
    jwtV, err := jwt.New(ctx, opts.JWKSURL, opts.JWKSRefreshInterval)
    if err != nil {
        return nil, fmt.Errorf("init JWT validator: %w", err)
    }
    akV := apikey.New(opts.VerifyAPIKeyURL, opts.APIKeyCacheTTL)
    mw := auth.NewFromValidators(jwtV, akV)

    return &PassportMiddleware{mw: mw, store: store, jwtV: jwtV}, nil
}

// Close stops the JWKS refresh goroutine.
func (p *PassportMiddleware) Close() {
    if p != nil && p.jwtV != nil {
        p.jwtV.Close()
    }
}

// Wrap returns an http.Handler that requires Passport auth and
// auto-provisions the identity.
func (p *PassportMiddleware) Wrap(next http.Handler) http.Handler {
    return p.mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Identity is in context from service-auth middleware
        id, ok := auth.IdentityFromContext(r.Context())
        if !ok {
            http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
            return
        }

        // Auto-provision identity
        identity, err := p.store.UpsertIdentity(r.Context(),
            id.ID, id.Username, id.DisplayName, id.Type)
        if err != nil {
            http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
            return
        }

        // Put domain.Identity in context for downstream handlers
        ctx := domain.WithIdentityContext(r.Context(), identity)
        next.ServeHTTP(w, r.WithContext(ctx))
    }))
}

// isPublicPath returns true for paths that skip auth.
func isPublicPath(path string) bool {
    switch {
    case path == "/v1/health",
         path == "/ui/health":
        return true
    default:
        return false
    }
}
```

**Step 3: Update server.go to wire Passport middleware**

The router needs to apply Passport middleware to `/api/v1/` routes but NOT to
Git transport routes, LFS routes, or health routes.

Update `NewRouter` to accept a `*PassportMiddleware` parameter. Apply it to
an API subrouter:

```go
func NewRouter(ctx context.Context, passport *PassportMiddleware) http.Handler {
    router := mux.NewRouter()

    // Health routes (no auth)
    router.HandleFunc("/v1/health", handleHealth).Methods("GET")
    router.HandleFunc("/ui/health", handleUIHealth).Methods("GET")

    // API routes (Passport auth)
    if passport != nil {
        api := router.PathPrefix("/api/v1").Subrouter()
        api.Use(passport.Wrap)
        RegisterRepoRoutes(api)
        RegisterKeyRoutes(api)
    }

    // Git transport routes (existing auth)
    GitController(ctx, router)

    // ...
}
```

**Step 4: Wire in daemon.go**

In `runDaemon`, create the passport middleware and pass to the router:

```go
passportURL := cfg.PassportURL
passport, err := web.NewPassportMiddleware(ctx, passportURL, store)
if err != nil {
    return fmt.Errorf("init passport: %w", err)
}
if passport != nil {
    defer passport.Close()
}
```

Add `passport-url` to viper defaults in `internal/config/config.go` and the
`Config` struct.

**Step 5: Verify and commit**

```bash
go build ./...
git commit -m "feat: add Passport auth middleware with auto-provisioning"
```

---

## Phase C: Health Endpoints

### Task 5: Replace health endpoints

**Files:**
- Modify: `internal/infra/httpapi/health.go`

**Step 1: Replace /livez and /readyz with /v1/health and /ui/health**

```go
func handleHealth(w http.ResponseWriter, r *http.Request) {
    store := domain.StoreFromContext(r.Context())
    if err := store.Ping(r.Context()); err != nil {
        writeJSON(w, http.StatusServiceUnavailable, map[string]string{
            "status": "unhealthy",
            "error":  err.Error(),
        })
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func handleUIHealth(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]any{
        "service": "combine",
        "version": version.Version,
        "routes": []map[string]string{
            {"route": "/api/v1", "label": "API"},
        },
    })
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}
```

**Step 2: Update E2E test**

The E2E test checks `/readyz` and `/livez`. Update to check `/v1/health`:

```go
// In tests/e2e/combine_test.go TestHealth:
resp, err := http.Get("http://" + d.HTTPAddr + "/v1/health")
```

Also update the harness readiness polling to use `/v1/health`.

**Step 3: Verify and commit**

```bash
go build ./...
cd tests/e2e && go test -v -run TestHealth -timeout 60s
git commit -m "feat: replace health endpoints with /v1/health and /ui/health"
```

---

## Phase D: Repo REST API

### Task 6: Add repo REST API handlers

**Files:**
- Create: `internal/infra/httpapi/api_repos.go`

**Step 1: Write the repo handlers**

```go
package web

// RegisterRepoRoutes registers repo CRUD routes on the API subrouter.
func RegisterRepoRoutes(r *mux.Router) {
    r.HandleFunc("/repos", handleListRepos).Methods("GET")
    r.HandleFunc("/repos", handleCreateRepo).Methods("POST")
    r.HandleFunc("/repos/{repo:.+}", handleGetRepo).Methods("GET")
    r.HandleFunc("/repos/{repo:.+}", handleUpdateRepo).Methods("PATCH")
    r.HandleFunc("/repos/{repo:.+}", handleDeleteRepo).Methods("DELETE")
}
```

Each handler:
1. Extracts `*domain.Identity` from context
2. Gets Backend from context
3. Calls the appropriate Backend method
4. Returns JSON response

**Request/response types:**

```go
type createRepoRequest struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Private     bool   `json:"private"`
}

type repoResponse struct {
    Name        string    `json:"name"`
    Description string    `json:"description"`
    ProjectName string    `json:"project_name"`
    Private     bool      `json:"private"`
    Mirror      bool      `json:"mirror"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

**Handlers:**
- `handleListRepos` — `be.Repositories(ctx)`, filter by access level, return JSON array
- `handleCreateRepo` — decode body, `be.CreateRepository(ctx, req.Name, identity, opts)`, return 201
- `handleGetRepo` — `be.Repository(ctx, name)`, return JSON
- `handleUpdateRepo` — decode body, update fields, return JSON
- `handleDeleteRepo` — `be.DeleteRepository(ctx, name)`, return 204

Note: Backend methods currently take `*domain.User`. These need to be updated
to accept `*domain.Identity` (or the handler does the mapping). The
implementing agent should update Backend's CreateRepository signature to work
with Identity.

**Step 2: Verify and commit**

```bash
go build ./...
git commit -m "feat: add repo management REST API"
```

---

### Task 7: Add SSH key REST API handlers

**Files:**
- Create: `internal/infra/httpapi/api_keys.go`

**Step 1: Write the key handlers**

```go
func RegisterKeyRoutes(r *mux.Router) {
    r.HandleFunc("/user/keys", handleListKeys).Methods("GET")
    r.HandleFunc("/user/keys", handleAddKey).Methods("POST")
    r.HandleFunc("/user/keys/{id}", handleDeleteKey).Methods("DELETE")
}
```

- `handleListKeys` — get identity from context, `store.ListPublicKeys(ctx, identity.ID)`, return JSON
- `handleAddKey` — decode body (`{"key": "ssh-ed25519 AAAA..."}`)`, parse the SSH key, `store.AddPublicKey(ctx, identity.ID, pk)`, return 201
- `handleDeleteKey` — parse key ID from URL, `store.RemovePublicKey(ctx, identity.ID, keyID)`, return 204

**Step 2: Verify and commit**

```bash
go build ./...
git commit -m "feat: add SSH key management REST API"
```

---

## Phase E: Update Backend for Identity

### Task 8: Update Backend to work with Identity

**Files:**
- Modify: `internal/app/backend/backend.go`
- Modify: `internal/app/backend/user.go`
- Modify: `internal/app/backend/repo.go`
- Modify: `internal/app/backend/collab.go`

**Step 1: Update Backend methods**

The Backend currently uses `*domain.User` for auth context. Methods like
`CreateRepository`, `AccessLevelForUser`, `AccessLevelByPublicKey` reference
User. These need to work with Identity.

Key changes:
- `CreateRepository(ctx, name, identity *domain.Identity, opts)` — use identity.ID as owner
- `AccessLevelForIdentity(ctx, repoName, identity *domain.Identity)` — check admin flag, collabs
- `AccessLevelByPublicKey` — looks up identity by public key instead of user
- Repo owner changes from `*int64` (user_id) to `*string` (owner_id, identity UUID)

This requires updating `domain.Repo.UserID *int64` to `domain.Repo.OwnerID *string`.

**Step 2: Update collabs to use identity ID**

Collab store methods change from username-based to identity-ID-based where
applicable. The REST API will use identity IDs for collab management.

**Step 3: Update SSH adapter**

The SSH middleware currently looks up users by public key. Update to look up
identities by public key via `IdentityStore.GetIdentityByPublicKey`.

**Step 4: Verify and commit**

This is a large change touching many files. Build and test:
```bash
go build ./...
go test ./...
```

```bash
git commit -m "refactor: update Backend and adapters to use Identity model"
```

---

## Phase F: Update E2E Tests and Verification

### Task 9: Update E2E tests for new health endpoint

**Files:**
- Modify: `tests/e2e/harness/harness.go`
- Modify: `tests/e2e/combine_test.go`

**Step 1: Update readiness polling**

Change the harness to poll `/v1/health` instead of `/readyz`.

**Step 2: Update TestHealth**

Check `/v1/health` and `/ui/health` instead of `/readyz` and `/livez`.

**Step 3: Add REST API E2E tests**

Add new tests (these only run when `COMBINE_PASSPORT_URL` is set, skip otherwise):

- `TestRESTCreateRepo` — create repo via POST, verify via GET
- `TestRESTListRepos` — create multiple repos, list, verify count
- `TestRESTDeleteRepo` — create repo, delete, verify 404
- `TestRESTSSHKeys` — add key via REST, verify SSH push works with that key

Note: these tests need a Passport instance. For now, skip them in local dev
and run them in CI where Passport is available. Add a `skipWithoutPassport(t)`
helper.

**Step 4: Run full E2E suite**

```bash
cd tests/e2e && go test -v -race -timeout 180s
```

All existing SSH/HTTP tests must still pass (they don't use Passport).

**Step 5: Commit**

```bash
git commit -m "test: update E2E tests for new health and REST API endpoints"
```

---

### Task 10: Config and cleanup

**Files:**
- Modify: `internal/config/config.go`
- Modify: `docs/remaining-features.md`

**Step 1: Add passport-url to config**

In `InitViper()`:
```go
viper.SetDefault("passport-url", "")
```

In `Config` struct:
```go
PassportURL string
```

In `FromViper()`:
```go
cfg.PassportURL = viper.GetString("passport-url")
```

**Step 2: Update remaining features doc**

Mark Feature 5 as complete.

**Step 3: Final verification**

```bash
go build ./...
go test ./...
cd tests/e2e && go test -v -race -timeout 180s
```

**Step 4: Commit**

```bash
git commit -m "feat: add passport-url config, mark feature complete"
```

---

## Notes for Implementer

### Migration strategy

The `users` table is NOT dropped immediately. Migration 002 adds the
`identities` table alongside. The old `users` table becomes unused after
Backend is updated to use Identity. A future migration (003) can drop it
and migrate `collabs.user_id` → `collabs.identity_id`.

### Standalone mode

When `passport-url` is empty:
- No REST API endpoints are registered
- SSH auth works as before (admin keys via `COMBINE_INITIAL_ADMIN_KEYS`)
- Git HTTP transport works as before (anonymous read for public repos)
- The `/v1/health` and `/ui/health` endpoints are always available

### Identity auto-provisioning

On every authenticated REST request, `UpsertIdentity` is called. This:
- Creates the identity if it doesn't exist
- Updates username/displayName/type if Passport claims changed
- Returns the local `*domain.Identity` for downstream handlers

### Repo ownership

Repos currently have `UserID *int64`. After migration, new repos created via
the REST API will have `OwnerID` set to the identity UUID (string). Existing
repos created via SSH push will have `OwnerID` null (no identity association
until that's backfilled).
