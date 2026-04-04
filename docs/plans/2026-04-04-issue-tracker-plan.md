# Issue Tracker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a lightweight issue tracker with per-repo issue numbering, comments, labels, REST API, and webhook events.

**Architecture:** Domain types (`Issue`, `IssueComment`, `IssueStatus`, `IssueResolution`) and `IssueStore` port in `internal/domain/`. SQLite and Postgres adapters implement the port. REST handlers at `/api/v1/repos/{repo}/issues`. Webhook events use the existing delivery infrastructure.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), PostgreSQL (pgx/v5), gorilla/mux, encoding/json

---

## Phase A: Domain Types and Store Port

### Task 1: Add issue domain types and IssueStore port

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/ports.go`
- Modify: `internal/domain/errors.go`

**Step 1: Add types to types.go**

Add after the `Identity` type:

```go
// IssueStatus represents the status of an issue.
type IssueStatus string

const (
	IssueStatusOpen       IssueStatus = "open"
	IssueStatusInProgress IssueStatus = "in_progress"
	IssueStatusClosed     IssueStatus = "closed"
)

// IssueResolution represents the resolution of a closed issue.
type IssueResolution string

const (
	IssueResolutionNone      IssueResolution = ""
	IssueResolutionFixed     IssueResolution = "fixed"
	IssueResolutionWontfix   IssueResolution = "wontfix"
	IssueResolutionDuplicate IssueResolution = "duplicate"
)

// Issue is a repository issue.
type Issue struct {
	ID         int64           // Global autoincrement PK (internal)
	Number     int64           // Per-repo issue number (user-facing)
	RepoID     int64           // FK to repos.id
	AuthorID   string          // FK to identities.id
	Title      string
	Body       string
	Status     IssueStatus
	Resolution IssueResolution
	AssigneeID *string         // FK to identities.id, nullable
	Labels     []string        // Denormalized from issue_labels table
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ClosedAt   *time.Time
}

// IssueComment is a comment on an issue.
type IssueComment struct {
	ID        int64
	IssueID   int64  // FK to issues.id (global PK)
	AuthorID  string // FK to identities.id
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IssueListOptions controls filtering for ListIssues.
type IssueListOptions struct {
	Status     *IssueStatus
	Label      *string
	AssigneeID *string
}
```

**Step 2: Add IssueStore port to ports.go**

```go
// IssueStore is the port for issue persistence.
type IssueStore interface {
	CreateIssue(ctx context.Context, issue *Issue) error
	GetIssueByNumber(ctx context.Context, repoID int64, number int64) (*Issue, error)
	ListIssues(ctx context.Context, repoID int64, opts IssueListOptions) ([]*Issue, error)
	UpdateIssue(ctx context.Context, issue *Issue) error

	SetIssueLabels(ctx context.Context, issueID int64, labels []string) error

	CreateIssueComment(ctx context.Context, comment *IssueComment) error
	ListIssueComments(ctx context.Context, issueID int64) ([]*IssueComment, error)
}
```

Add `IssueStore` to the composite `Store` interface.

**Step 3: Add errors to errors.go**

```go
// ErrIssueNotFound is returned when an issue is not found.
var ErrIssueNotFound = errors.New("issue not found")
```

**Step 4: Verify and commit**

```bash
go build ./internal/domain/
git commit -m "feat: add Issue domain types and IssueStore port"
```

---

## Phase B: SQLite Adapter

### Task 2: Add SQLite migration for issues

**Files:**
- Create: `internal/infra/sqlite/migrations/003_issues.sql`

```sql
-- +goose Up

CREATE TABLE issues (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    number      INTEGER NOT NULL,
    repo_id     INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    author_id   TEXT NOT NULL REFERENCES identities(id),
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open',
    resolution  TEXT NOT NULL DEFAULT '',
    assignee_id TEXT REFERENCES identities(id),
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    closed_at   DATETIME,
    UNIQUE(repo_id, number)
);

CREATE INDEX idx_issues_repo_status ON issues(repo_id, status);

CREATE TABLE issue_labels (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    UNIQUE(issue_id, label)
);

CREATE TABLE issue_comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES identities(id),
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS issue_comments;
DROP TABLE IF EXISTS issue_labels;
DROP TABLE IF EXISTS issues;
```

**Verify and commit:**

```bash
go build ./...
git commit -m "feat: add SQLite migration for issues tables"
```

---

### Task 3: Implement IssueStore in SQLite adapter

**Files:**
- Create: `internal/infra/sqlite/issue.go`

Follow the pattern in `sqlite/repo.go`:
- Package-level functions taking `querier` for testability and transaction support
- `scanIssue` / `scanIssues` helpers
- Store methods delegating to package-level functions
- txStore methods delegating to same package-level functions

**Key implementation details:**

`createIssue` — Assign per-repo number atomically:
```go
func createIssue(ctx context.Context, q querier, issue *domain.Issue) error {
    res, err := q.ExecContext(ctx,
        `INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
         VALUES ((SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE repo_id = ?),
                 ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
        issue.RepoID, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
        issue.Status, issue.Resolution, issue.AssigneeID,
    )
    if err != nil {
        return err
    }
    id, _ := res.LastInsertId()
    issue.ID = id
    // Read back the assigned number
    row := q.QueryRowContext(ctx, `SELECT number FROM issues WHERE id = ?`, id)
    return row.Scan(&issue.Number)
}
```

`getIssueByNumber` — Join with `issue_labels` to populate `Labels`:
```go
func getIssueByNumber(ctx context.Context, q querier, repoID, number int64) (*domain.Issue, error) {
    row := q.QueryRowContext(ctx,
        `SELECT `+issueColumns+` FROM issues WHERE repo_id = ? AND number = ?`,
        repoID, number)
    issue, err := scanIssue(row)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("%w: #%d", domain.ErrIssueNotFound, number)
    }
    if err != nil {
        return nil, err
    }
    issue.Labels, err = getIssueLabels(ctx, q, issue.ID)
    return issue, err
}
```

`listIssues` — Build WHERE clause dynamically from `IssueListOptions`. For label filtering, use `EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = issues.id AND label = ?)`.

`updateIssue` — Update all mutable fields. The caller is responsible for setting `ClosedAt` based on status transitions.

`setIssueLabels` — Delete existing labels, insert new ones in a loop.

`createIssueComment` / `listIssueComments` — Standard CRUD.

Also add Store and txStore method wrappers (same pattern as repo.go).

**Verify and commit:**

```bash
go build ./internal/infra/sqlite/
git commit -m "feat: implement IssueStore in SQLite adapter"
```

---

## Phase C: PostgreSQL Adapter

### Task 4: Add PostgreSQL migration and IssueStore

**Files:**
- Create: `internal/infra/postgres/migrations/003_issues.sql`
- Create: `internal/infra/postgres/issue.go`

Same schema as SQLite but with PostgreSQL syntax:
- `SERIAL` or `BIGSERIAL` for autoincrement
- `TIMESTAMPTZ` instead of `DATETIME`
- `$1` placeholders instead of `?`
- `RETURNING id, number` on INSERT instead of `LastInsertId`

**Verify and commit:**

```bash
go build ./internal/infra/postgres/
git commit -m "feat: implement IssueStore in PostgreSQL adapter"
```

---

## Phase D: REST API Handlers

### Task 5: Add issue REST API handlers

**Files:**
- Create: `internal/infra/httpapi/api_issues.go`

Follow the pattern in `httpapi/api_repos.go`.

**Step 1: Define request/response types**

```go
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
    Number     int64            `json:"number"`
    Title      string           `json:"title"`
    Body       string           `json:"body"`
    Status     string           `json:"status"`
    Resolution string           `json:"resolution"`
    Author     identityRef      `json:"author"`
    Assignee   *identityRef     `json:"assignee,omitempty"`
    Labels     []string         `json:"labels"`
    CreatedAt  time.Time        `json:"created_at"`
    UpdatedAt  time.Time        `json:"updated_at"`
    ClosedAt   *time.Time       `json:"closed_at,omitempty"`
}

type commentResponse struct {
    ID        int64       `json:"id"`
    Author    identityRef `json:"author"`
    Body      string      `json:"body"`
    CreatedAt time.Time   `json:"created_at"`
    UpdatedAt time.Time   `json:"updated_at"`
}
```

**Step 2: Register routes**

```go
func RegisterIssueRoutes(r *mux.Router) {
    r.HandleFunc("/repos/{repo:.+}/issues", handleListIssues).Methods("GET")
    r.HandleFunc("/repos/{repo:.+}/issues", handleCreateIssue).Methods("POST")
    r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}", handleGetIssue).Methods("GET")
    r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}", handleUpdateIssue).Methods("PATCH")
    r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}/comments", handleListComments).Methods("GET")
    r.HandleFunc("/repos/{repo:.+}/issues/{number:[0-9]+}/comments", handleCreateComment).Methods("POST")
}
```

**Step 3: Implement handlers**

Each handler:
1. Extracts identity from context via `domain.IdentityFromContext(ctx)`
2. Gets backend from context via `backend.FromContext(ctx)`
3. Resolves repo by name from URL path
4. Calls store methods
5. Returns JSON response

`handleCreateIssue`:
- Decode request body, validate title is non-empty
- Look up repo by name
- Create `domain.Issue` with `Status: IssueStatusOpen`, `AuthorID` from identity
- Call `store.CreateIssue(ctx, &issue)`
- If labels provided, call `store.SetIssueLabels(ctx, issue.ID, labels)`
- Fire `issue_opened` webhook
- Return 201

`handleUpdateIssue`:
- Decode request body
- Get existing issue by repo + number
- Apply non-nil fields
- Handle status transitions: if status changed to `closed`, set `ClosedAt`; if changed away from `closed`, clear `ClosedAt`
- Call `store.UpdateIssue(ctx, &issue)`
- If labels provided, call `store.SetIssueLabels(ctx, issue.ID, labels)`
- Fire appropriate webhook (`issue_status_changed`, `issue_closed`)
- Return 200

`handleListIssues`:
- Parse query params: `status`, `label`, `assignee`
- Call `store.ListIssues(ctx, repoID, opts)`
- Return 200

**Step 4: Wire routes in server.go**

Add `RegisterIssueRoutes(api)` alongside `RegisterRepoRoutes(api)` in the API subrouter setup.

**Step 5: Resolve identity usernames for responses**

The `issueResponse` includes `author` and `assignee` as `identityRef` objects.
The handler needs to look up identity usernames. Options:
- Batch lookup: collect unique identity IDs from the issue list, fetch all at once
- For single issue endpoints: two lookups (author + assignee)

Use `store.GetIdentityByID(ctx, id)` for lookups. Cache within request scope if needed.

**Verify and commit:**

```bash
go build ./...
git commit -m "feat: add issue tracker REST API handlers"
```

---

## Phase E: Webhook Events

### Task 6: Add issue webhook events

**Files:**
- Modify: `internal/infra/webhook/event.go`
- Create: `internal/infra/webhook/issue.go`

**Step 1: Add event constants to event.go**

```go
EventIssueOpened        Event = 7
EventIssueStatusChanged Event = 8
EventIssueClosed        Event = 9
EventIssueComment       Event = 10
```

Add to `Events()`, `eventStrings`, and `stringEvent` maps.

**Step 2: Create issue.go with event payload types**

Follow the pattern in `webhook/repository.go`:

```go
package webhook

import (
    "context"
    "time"

    "github.com/Work-Fort/Combine/internal/config"
    "github.com/Work-Fort/Combine/internal/domain"
)

// IssuePayload is the issue representation in webhook payloads.
type IssuePayload struct {
    Number     int64       `json:"number"`
    Title      string      `json:"title"`
    Body       string      `json:"body"`
    Status     string      `json:"status"`
    Resolution string      `json:"resolution"`
    Author     User        `json:"author"`
    Assignee   *User       `json:"assignee,omitempty"`
    Labels     []string    `json:"labels"`
    CreatedAt  time.Time   `json:"created_at"`
    UpdatedAt  time.Time   `json:"updated_at"`
    ClosedAt   *time.Time  `json:"closed_at,omitempty"`
}

// CommentPayload is the comment representation in webhook payloads.
type CommentPayload struct {
    ID        int64     `json:"id"`
    Body      string    `json:"body"`
    Author    User      `json:"author"`
    CreatedAt time.Time `json:"created_at"`
}

// IssueOpenedEvent is fired when a new issue is created.
type IssueOpenedEvent struct {
    Common
    Issue IssuePayload `json:"issue"`
}

// IssueStatusChangedEvent is fired when an issue's status changes.
type IssueStatusChangedEvent struct {
    Common
    Issue     IssuePayload `json:"issue"`
    OldStatus string       `json:"old_status"`
    NewStatus string       `json:"new_status"`
}

// IssueClosedEvent is fired when an issue is closed.
type IssueClosedEvent struct {
    Common
    Issue      IssuePayload `json:"issue"`
    Resolution string       `json:"resolution"`
}

// IssueCommentEvent is fired when a comment is added to an issue.
type IssueCommentEvent struct {
    Common
    Issue   IssuePayload   `json:"issue"`
    Comment CommentPayload `json:"comment"`
}
```

Add constructor functions (`NewIssueOpenedEvent`, etc.) following the
`NewRepositoryEvent` pattern: build Common with repo info, set sender from
identity context, populate issue/comment payloads.

Note: The `Common.Sender` field currently uses `webhook.User` which has
`ID int64`. For issue events, the sender is an Identity (string UUID). Either:
- Add a new `IdentitySender` field alongside `Sender` for issue events, or
- Update `webhook.User` to use `string` ID (breaking change for existing events)

Recommended: Add an `IdentitySender` struct with `ID string` and `Username string`
for issue event payloads. Existing push/repo events keep using the legacy
`User` sender. This avoids breaking existing webhook consumers.

```go
// IdentitySender represents a Passport identity in webhook payloads.
type IdentitySender struct {
    ID       string `json:"id"`
    Username string `json:"username"`
}
```

Issue event constructors use `IdentitySender` instead of `Sender` in `Common`.
Override the `Sender` field or embed differently — the implementing agent should
choose the cleanest approach.

**Verify and commit:**

```bash
go build ./internal/infra/webhook/
git commit -m "feat: add issue webhook event types and constructors"
```

---

## Phase F: E2E Tests

### Task 7: Add issue tracker E2E tests

**Files:**
- Modify: `tests/e2e/harness/api_client.go`
- Modify: `tests/e2e/combine_test.go`

**Step 1: Add issue helper methods to APIClient**

```go
func (c *APIClient) CreateIssue(t *testing.T, repo, title, body string) map[string]any { ... }
func (c *APIClient) GetIssue(t *testing.T, repo string, number int) map[string]any { ... }
func (c *APIClient) ListIssues(t *testing.T, repo string) []map[string]any { ... }
func (c *APIClient) UpdateIssue(t *testing.T, repo string, number int, updates map[string]any) map[string]any { ... }
func (c *APIClient) CreateComment(t *testing.T, repo string, number int, body string) map[string]any { ... }
func (c *APIClient) ListComments(t *testing.T, repo string, number int) []map[string]any { ... }
```

**Step 2: Add E2E test cases**

```go
func TestIssueCreate(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "issue-test", false)

    issue := client.CreateIssue(t, "issue-test", "Bug report", "Something is broken")
    if issue["number"] != float64(1) {
        t.Errorf("number = %v, want 1", issue["number"])
    }
    if issue["status"] != "open" {
        t.Errorf("status = %v, want open", issue["status"])
    }
}

func TestIssuePerRepoNumbering(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "repo-a", false)
    client.CreateRepo(t, "repo-b", false)

    a1 := client.CreateIssue(t, "repo-a", "Issue A1", "")
    b1 := client.CreateIssue(t, "repo-b", "Issue B1", "")
    a2 := client.CreateIssue(t, "repo-a", "Issue A2", "")

    // Both repos should start numbering at 1
    if a1["number"] != float64(1) { t.Errorf("a1 number = %v", a1["number"]) }
    if b1["number"] != float64(1) { t.Errorf("b1 number = %v", b1["number"]) }
    if a2["number"] != float64(2) { t.Errorf("a2 number = %v", a2["number"]) }
}

func TestIssueListFilter(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "filter-test", false)

    client.CreateIssue(t, "filter-test", "Open issue", "")
    client.UpdateIssue(t, "filter-test", 1, map[string]any{"status": "closed", "resolution": "fixed"})
    client.CreateIssue(t, "filter-test", "Another open", "")

    // List all
    all := client.ListIssues(t, "filter-test")
    if len(all) != 2 { t.Errorf("expected 2 issues, got %d", len(all)) }

    // List open only (via query param — need to add query param support to ListIssues)
}

func TestIssueComments(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "comment-test", false)
    client.CreateIssue(t, "comment-test", "Test issue", "")

    comment := client.CreateComment(t, "comment-test", 1, "First comment")
    if comment["body"] != "First comment" {
        t.Errorf("body = %v", comment["body"])
    }

    comments := client.ListComments(t, "comment-test", 1)
    if len(comments) != 1 {
        t.Errorf("expected 1 comment, got %d", len(comments))
    }
}

func TestIssueStatusTransitions(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "status-test", false)
    client.CreateIssue(t, "status-test", "Test issue", "")

    // Open -> in_progress
    updated := client.UpdateIssue(t, "status-test", 1, map[string]any{"status": "in_progress"})
    if updated["status"] != "in_progress" { t.Errorf("status = %v", updated["status"]) }

    // in_progress -> closed
    updated = client.UpdateIssue(t, "status-test", 1, map[string]any{
        "status": "closed", "resolution": "fixed",
    })
    if updated["status"] != "closed" { t.Errorf("status = %v", updated["status"]) }
    if updated["closed_at"] == nil { t.Error("closed_at should be set") }

    // closed -> open (reopen)
    updated = client.UpdateIssue(t, "status-test", 1, map[string]any{"status": "open"})
    if updated["status"] != "open" { t.Errorf("status = %v", updated["status"]) }
    if updated["closed_at"] != nil { t.Error("closed_at should be cleared on reopen") }
}
```

**Step 3: Run tests**

```bash
cd tests/e2e && go test -v -race -timeout 180s -run TestIssue
```

**Verify and commit:**

```bash
git commit -m "test: add issue tracker E2E tests"
```

---

## Notes for Implementer

### Per-repo numbering

The `number` column uses a subquery on INSERT:
```sql
(SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE repo_id = ?)
```

This is safe under SQLite's implicit transaction per statement. For concurrent
inserts in PostgreSQL, the UNIQUE constraint on `(repo_id, number)` provides
safety — a retry loop handles the rare conflict.

### Identity resolution in responses

Issue API responses include `author` and `assignee` as `{id, username}` objects.
The handler must look up identities by ID. For list endpoints, batch the lookups
to avoid N+1 queries. A simple approach: collect unique identity IDs, fetch all
with `SELECT ... WHERE id IN (?, ?, ...)`, build a map.

### Webhook firing

Issue webhook events are fired from the REST API handlers (not the store layer).
The handler calls the webhook delivery infrastructure after a successful store
operation. Follow the existing pattern where `SendEvent` is called in Backend
methods.

### Labels handling

`SetIssueLabels` is a replace-all operation: delete existing labels, insert new
ones. This simplifies the API (send the full label list on update) and avoids
add/remove complexity.

### Cascade deletes

The `ON DELETE CASCADE` on `issues.repo_id` means deleting a repo automatically
removes all issues, labels, and comments. No additional cleanup code needed.
