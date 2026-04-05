# Pull Requests Plan 7a: Shared Number Sequence + PR Domain + Store + CRUD API

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Introduce a shared per-repo number counter for issues and PRs, add the PullRequest domain model and PullRequestStore port, implement SQLite and Postgres store adapters, and expose PR CRUD endpoints via the REST API.

**Architecture:** Domain types and `PullRequestStore` port in `internal/domain/`. `repo_counters` table provides atomic shared numbering. SQLite and Postgres adapters implement the port. REST handlers at `/api/v1/repos/{repo}/pulls`. Issue creation refactored to use the shared counter.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), PostgreSQL (pgx/v5), gorilla/mux, encoding/json, goose migrations

---

## Phase A: Shared Number Sequence

### Task 1: Add repo_counters migration and refactor issue numbering

**Files:**
- Create: `internal/infra/sqlite/migrations/004_pull_requests.sql`
- Modify: `internal/infra/sqlite/issue.go`
- Modify: `internal/infra/sqlite/store.go` (if needed for helper)

**Step 1: Create migration 004_pull_requests.sql**

```sql
-- +goose Up

-- Shared per-repo number counter for issues and PRs.
CREATE TABLE repo_counters (
    repo_id     INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    next_number INTEGER NOT NULL DEFAULT 1
);

-- Initialize counters from existing issue numbers.
INSERT INTO repo_counters (repo_id, next_number)
SELECT repo_id, MAX(number) + 1
FROM issues
GROUP BY repo_id;

-- Ensure repos without issues also get a counter row.
INSERT OR IGNORE INTO repo_counters (repo_id, next_number)
SELECT id, 1 FROM repos;

-- Pull requests table.
CREATE TABLE pull_requests (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    number         INTEGER NOT NULL,
    repo_id        INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    author_id      TEXT NOT NULL REFERENCES identities(id),
    title          TEXT NOT NULL,
    body           TEXT NOT NULL DEFAULT '',
    source_branch  TEXT NOT NULL,
    target_branch  TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'open',
    merge_method   TEXT,
    merged_by      TEXT REFERENCES identities(id),
    assignee_id    TEXT REFERENCES identities(id),
    created_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    merged_at      DATETIME,
    closed_at      DATETIME,
    UNIQUE(repo_id, number)
);

CREATE INDEX idx_pull_requests_repo_status ON pull_requests(repo_id, status);

-- Review tables (schema only — populated in plan 7c).
CREATE TABLE pull_request_reviews (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    pr_id      INTEGER NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES identities(id),
    state      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE review_comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id  INTEGER NOT NULL REFERENCES pull_request_reviews(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    line       INTEGER NOT NULL,
    side       TEXT NOT NULL DEFAULT 'right',
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS review_comments;
DROP TABLE IF EXISTS pull_request_reviews;
DROP TABLE IF EXISTS pull_requests;
DROP TABLE IF EXISTS repo_counters;
```

**Step 2: Add `nextNumber` helper to `internal/infra/sqlite/issue.go` (or a new shared file)**

Add a new file `internal/infra/sqlite/counter.go`:

```go
package sqlite

import (
    "context"
    "database/sql"
)

// nextNumber atomically allocates the next per-repo number (shared by issues and PRs).
// Must be called within a transaction for safety.
func nextNumber(ctx context.Context, q querier, repoID int64) (int64, error) {
    // Ensure a counter row exists for this repo.
    _, err := q.ExecContext(ctx,
        `INSERT OR IGNORE INTO repo_counters (repo_id, next_number) VALUES (?, 1)`, repoID)
    if err != nil {
        return 0, err
    }

    // Atomically increment and return.
    var num int64
    err = q.QueryRowContext(ctx,
        `UPDATE repo_counters SET next_number = next_number + 1 WHERE repo_id = ? RETURNING next_number - 1`,
        repoID).Scan(&num)
    if err != nil {
        return 0, err
    }
    return num, nil
}
```

**Note:** modernc.org/sqlite supports `RETURNING` (SQLite 3.35+). No fallback needed.

**Step 3: Refactor `createIssue` in `internal/infra/sqlite/issue.go`**

Replace the `MAX(number) + 1` subquery with `nextNumber`:

Old:
```go
res, err := q.ExecContext(ctx,
    `INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
     VALUES ((SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE repo_id = ?),
             ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
    issue.RepoID, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
    issue.Status, issue.Resolution, issue.AssigneeID,
)
```

New:
```go
num, err := nextNumber(ctx, q, issue.RepoID)
if err != nil {
    return err
}

res, err := q.ExecContext(ctx,
    `INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
    num, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
    issue.Status, issue.Resolution, issue.AssigneeID,
)
```

Also update the backfill read to just set `issue.Number = num` directly instead of reading it back (the number is known):

```go
issue.Number = num
```

Keep the `SELECT created_at, updated_at` read-back for timestamps.

**Verification:** Run existing issue E2E tests — `TestIssueCreate`, `TestIssuePerRepoNumbering` must still pass. The numbering behavior should be identical.

---

## Phase B: PR Domain Types and Store Port

### Task 2: Add PR domain types

**Files:**
- Modify: `internal/domain/types.go`

**Step 1: Add after the `IssueComment` type:**

```go
// PullRequestStatus represents the status of a pull request.
type PullRequestStatus string

const (
    PullRequestStatusOpen   PullRequestStatus = "open"
    PullRequestStatusMerged PullRequestStatus = "merged"
    PullRequestStatusClosed PullRequestStatus = "closed"
)

// MergeMethod represents a pull request merge strategy.
type MergeMethod string

const (
    MergeMethodMerge  MergeMethod = "merge"
    MergeMethodSquash MergeMethod = "squash"
    MergeMethodRebase MergeMethod = "rebase"
)

// PullRequest is a repository pull request.
type PullRequest struct {
    ID           int64             // Global autoincrement PK (internal)
    Number       int64             // Per-repo number (shared with issues)
    RepoID       int64             // FK to repos.id
    AuthorID     string            // FK to identities.id
    Title        string
    Body         string
    SourceBranch string
    TargetBranch string
    Status       PullRequestStatus
    MergeMethod  *MergeMethod      // Set when merged
    MergedBy     *string           // FK to identities.id, nullable
    AssigneeID   *string           // FK to identities.id, nullable
    CreatedAt    time.Time
    UpdatedAt    time.Time
    MergedAt     *time.Time
    ClosedAt     *time.Time
}

// PullRequestListOptions controls filtering for ListPullRequests.
type PullRequestListOptions struct {
    Status   *PullRequestStatus
    AuthorID *string
}
```

### Task 3: Add PullRequestStore port and error

**Files:**
- Modify: `internal/domain/ports.go`
- Modify: `internal/domain/errors.go`

**Step 1: Add to errors.go:**

```go
var ErrPullRequestNotFound = errors.New("pull request not found")
```

**Step 2: Add PullRequestStore port to ports.go (before the Store composite interface):**

```go
// PullRequestStore is the port for pull request persistence.
type PullRequestStore interface {
    CreatePullRequest(ctx context.Context, pr *PullRequest) error
    GetPullRequestByNumber(ctx context.Context, repoID int64, number int64) (*PullRequest, error)
    ListPullRequests(ctx context.Context, repoID int64, opts PullRequestListOptions) ([]*PullRequest, error)
    UpdatePullRequest(ctx context.Context, pr *PullRequest) error
}
```

**Step 3: Add `PullRequestStore` to the `Store` composite interface:**

```go
type Store interface {
    RepoStore
    UserStore
    CollabStore
    SettingStore
    AccessTokenStore
    LFSStore
    WebhookStore
    IdentityStore
    IssueStore
    PullRequestStore  // <-- add this line

    Transaction(ctx context.Context, fn func(tx Store) error) error
    Ping(ctx context.Context) error
    io.Closer
}
```

---

## Phase C: SQLite PR Store Adapter

### Task 4: Implement SQLite pull request store

**Files:**
- Create: `internal/infra/sqlite/pull_request.go`

**Step 1: Create the file following the same pattern as `issue.go`:**

```go
package sqlite

import (
    "context"
    "database/sql"
    "fmt"
    "strings"

    "github.com/Work-Fort/Combine/internal/domain"
)

const prColumns = `id, number, repo_id, author_id, title, body, source_branch, target_branch, status, merge_method, merged_by, assignee_id, created_at, updated_at, merged_at, closed_at`

func scanPullRequest(row interface{ Scan(dest ...any) error }) (*domain.PullRequest, error) {
    var pr domain.PullRequest
    var mergeMethod sql.NullString
    var mergedBy sql.NullString
    var assigneeID sql.NullString
    var mergedAt sql.NullTime
    var closedAt sql.NullTime
    if err := row.Scan(
        &pr.ID, &pr.Number, &pr.RepoID, &pr.AuthorID,
        &pr.Title, &pr.Body, &pr.SourceBranch, &pr.TargetBranch,
        &pr.Status, &mergeMethod, &mergedBy, &assigneeID,
        &pr.CreatedAt, &pr.UpdatedAt, &mergedAt, &closedAt,
    ); err != nil {
        return nil, err
    }
    if mergeMethod.Valid {
        mm := domain.MergeMethod(mergeMethod.String)
        pr.MergeMethod = &mm
    }
    if mergedBy.Valid {
        pr.MergedBy = &mergedBy.String
    }
    if assigneeID.Valid {
        pr.AssigneeID = &assigneeID.String
    }
    if mergedAt.Valid {
        pr.MergedAt = &mergedAt.Time
    }
    if closedAt.Valid {
        pr.ClosedAt = &closedAt.Time
    }
    return &pr, nil
}

func scanPullRequests(rows *sql.Rows) ([]*domain.PullRequest, error) {
    var prs []*domain.PullRequest
    for rows.Next() {
        pr, err := scanPullRequest(rows)
        if err != nil {
            return nil, err
        }
        prs = append(prs, pr)
    }
    return prs, rows.Err()
}

func createPullRequest(ctx context.Context, q querier, pr *domain.PullRequest) error {
    num, err := nextNumber(ctx, q, pr.RepoID)
    if err != nil {
        return err
    }

    res, err := q.ExecContext(ctx,
        `INSERT INTO pull_requests (number, repo_id, author_id, title, body, source_branch, target_branch, status, assignee_id, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
        num, pr.RepoID, pr.AuthorID, pr.Title, pr.Body,
        pr.SourceBranch, pr.TargetBranch, pr.Status, pr.AssigneeID,
    )
    if err != nil {
        return err
    }
    id, _ := res.LastInsertId()
    pr.ID = id
    pr.Number = num
    row := q.QueryRowContext(ctx, `SELECT created_at, updated_at FROM pull_requests WHERE id = ?`, id)
    return row.Scan(&pr.CreatedAt, &pr.UpdatedAt)
}

func getPullRequestByNumber(ctx context.Context, q querier, repoID, number int64) (*domain.PullRequest, error) {
    row := q.QueryRowContext(ctx,
        `SELECT `+prColumns+` FROM pull_requests WHERE repo_id = ? AND number = ?`,
        repoID, number)
    pr, err := scanPullRequest(row)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("%w: #%d", domain.ErrPullRequestNotFound, number)
    }
    return pr, err
}

func listPullRequests(ctx context.Context, q querier, repoID int64, opts domain.PullRequestListOptions) ([]*domain.PullRequest, error) {
    where := []string{"repo_id = ?"}
    args := []any{repoID}

    if opts.Status != nil {
        where = append(where, "status = ?")
        args = append(args, string(*opts.Status))
    }
    if opts.AuthorID != nil {
        where = append(where, "author_id = ?")
        args = append(args, *opts.AuthorID)
    }

    query := `SELECT ` + prColumns + ` FROM pull_requests WHERE ` + strings.Join(where, " AND ") + ` ORDER BY number DESC`
    rows, err := q.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    return scanPullRequests(rows)
}

func updatePullRequest(ctx context.Context, q querier, pr *domain.PullRequest) error {
    _, err := q.ExecContext(ctx,
        `UPDATE pull_requests SET title = ?, body = ?, status = ?, merge_method = ?, merged_by = ?,
         assignee_id = ?, merged_at = ?, closed_at = ?, updated_at = CURRENT_TIMESTAMP
         WHERE id = ?`,
        pr.Title, pr.Body, pr.Status, pr.MergeMethod, pr.MergedBy,
        pr.AssigneeID, pr.MergedAt, pr.ClosedAt, pr.ID,
    )
    if err != nil {
        return err
    }
    row := q.QueryRowContext(ctx, `SELECT updated_at FROM pull_requests WHERE id = ?`, pr.ID)
    return row.Scan(&pr.UpdatedAt)
}

// Store methods.

func (s *Store) CreatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
    return createPullRequest(ctx, s.q(), pr)
}

func (s *Store) GetPullRequestByNumber(ctx context.Context, repoID int64, number int64) (*domain.PullRequest, error) {
    return getPullRequestByNumber(ctx, s.q(), repoID, number)
}

func (s *Store) ListPullRequests(ctx context.Context, repoID int64, opts domain.PullRequestListOptions) ([]*domain.PullRequest, error) {
    return listPullRequests(ctx, s.q(), repoID, opts)
}

func (s *Store) UpdatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
    return updatePullRequest(ctx, s.q(), pr)
}

// txStore methods.

func (ts *txStore) CreatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
    return createPullRequest(ctx, ts.q(), pr)
}

func (ts *txStore) GetPullRequestByNumber(ctx context.Context, repoID int64, number int64) (*domain.PullRequest, error) {
    return getPullRequestByNumber(ctx, ts.q(), repoID, number)
}

func (ts *txStore) ListPullRequests(ctx context.Context, repoID int64, opts domain.PullRequestListOptions) ([]*domain.PullRequest, error) {
    return listPullRequests(ctx, ts.q(), repoID, opts)
}

func (ts *txStore) UpdatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
    return updatePullRequest(ctx, ts.q(), pr)
}
```

**Verification:** `go build ./...` must compile. Run existing issue tests to confirm the migration doesn't break anything.

---

## Phase D: Postgres PR Store Adapter

### Task 5: Add Postgres migration and adapter

**Files:**
- Create: `internal/infra/postgres/migrations/004_pull_requests.sql`
- Create: `internal/infra/postgres/pull_request.go`

**Step 1: Postgres migration**

Same schema as SQLite but with Postgres syntax:
- Replace `INTEGER PRIMARY KEY AUTOINCREMENT` with `SERIAL PRIMARY KEY` (or `BIGSERIAL`)
- Replace `datetime('now')` with `NOW()`
- Replace `INSERT OR IGNORE` with `INSERT ... ON CONFLICT DO NOTHING`
- Replace `RETURNING next_number - 1` pattern if needed

**Step 2: Postgres PR store adapter**

Follow the same structure as the SQLite adapter. The Postgres store likely uses `pgx` directly with `$1, $2` parameter placeholders instead of `?`. Mirror the existing pattern from any Postgres issue store file.

**Note:** If no Postgres adapter exists yet for issues, create a minimal one following the SQLite pattern but with `$N` placeholders. Check `internal/infra/postgres/` for the existing adapter patterns.

**Verification:** `go build ./...` compiles.

---

## Phase E: PR CRUD REST API

### Task 6: Create PR REST API handlers

**Files:**
- Create: `internal/infra/httpapi/api_pulls.go`
- Modify: `internal/infra/httpapi/server.go`

**Step 1: Create `api_pulls.go`**

Follow the exact same pattern as `api_issues.go`:

```go
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
```

**Step 2: Register routes in `server.go`**

In `NewRouter`, add after `RegisterIssueRoutes(api)`:

```go
RegisterPullRequestRoutes(api)
```

This must also be registered before `RegisterRepoRoutes(api)` due to the greedy `{repo:.+}` pattern.

**Verification:** `go build ./...` compiles. Manual curl test: create a PR, get it, list PRs.

---

## Phase F: Postgres Counter Helper

### Task 7: Add Postgres nextNumber helper

**Files:**
- Create: `internal/infra/postgres/counter.go`

Follow the same pattern as SQLite but with Postgres syntax:
- `INSERT ... ON CONFLICT DO NOTHING` instead of `INSERT OR IGNORE`
- `$1, $2` placeholders instead of `?`
- Postgres supports `RETURNING` natively

```go
func nextNumber(ctx context.Context, q querier, repoID int64) (int64, error) {
    _, err := q.ExecContext(ctx,
        `INSERT INTO repo_counters (repo_id, next_number) VALUES ($1, 1) ON CONFLICT DO NOTHING`, repoID)
    if err != nil {
        return 0, err
    }

    var num int64
    err = q.QueryRowContext(ctx,
        `UPDATE repo_counters SET next_number = next_number + 1 WHERE repo_id = $1 RETURNING next_number - 1`,
        repoID).Scan(&num)
    return num, err
}
```

**Step 2: Refactor Postgres issue creation to use `nextNumber`**

In `internal/infra/postgres/issue.go`, replace the `MAX(number) + 1` subquery with `nextNumber`, mirroring the SQLite change:

Old:
```go
res, err := q.ExecContext(ctx,
    `INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
     VALUES ((SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE repo_id = $1),
             $2, $3, $4, $5, $6, $7, $8, NOW())`,
    issue.RepoID, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
    issue.Status, issue.Resolution, issue.AssigneeID,
)
```

New:
```go
num, err := nextNumber(ctx, q, issue.RepoID)
if err != nil {
    return err
}

res, err := q.ExecContext(ctx,
    `INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
     VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
    num, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
    issue.Status, issue.Resolution, issue.AssigneeID,
)
```

Set `issue.Number = num` directly instead of reading it back.

**Verification:** `go build ./...` compiles.

---

## Summary

After completing Plan 7a:
- Issues and PRs share a per-repo number counter via `repo_counters`
- `PullRequest` domain type and `PullRequestStore` port exist
- SQLite and Postgres adapters implement CRUD for PRs
- REST API supports create/get/list/update PRs at `/api/v1/repos/{repo}/pulls`
- No git operations yet (diff, merge, mergeability) — those come in Plan 7b
- No webhooks yet — those come in Plans 7b and 7c
