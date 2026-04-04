# Pull Requests Plan 7c: Reviews + Webhook Events + E2E Tests

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add PR review system (approve/request changes/comment with line-level comments), review webhook events, and comprehensive E2E tests covering all PR functionality.

**Architecture:** Review domain types and `ReviewStore` port in `internal/domain/`. SQLite and Postgres adapters. REST handlers for review endpoints. E2E tests in `tests/e2e/`.

**Tech Stack:** Go, SQLite, PostgreSQL, gorilla/mux, encoding/json, testing

**Prerequisites:** Plans 7a and 7b completed (PR CRUD, diff, merge, webhooks).

---

## Phase A: Review Domain Model

### Task 1: Add review domain types and store port

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/ports.go`
- Modify: `internal/domain/errors.go`

**Step 1: Add to types.go after PullRequest types:**

```go
// ReviewState represents the state of a pull request review.
type ReviewState string

const (
    ReviewStatePending          ReviewState = "pending"
    ReviewStateApproved         ReviewState = "approved"
    ReviewStateChangesRequested ReviewState = "changes_requested"
    ReviewStateCommented        ReviewState = "commented"
)

// PullRequestReview is a review on a pull request.
type PullRequestReview struct {
    ID        int64       // Global PK
    PRID      int64       // FK to pull_requests.id
    AuthorID  string      // FK to identities.id
    State     ReviewState
    Body      string
    Comments  []ReviewComment // Populated on read
    CreatedAt time.Time
}

// ReviewComment is a line-level comment on a file in a PR review.
type ReviewComment struct {
    ID        int64
    ReviewID  int64  // FK to pull_request_reviews.id
    Path      string // File path in diff
    Line      int    // Line number
    Side      string // "left" or "right"
    Body      string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Step 2: Add to errors.go:**

```go
var ErrReviewNotFound = errors.New("review not found")
```

**Step 3: Add ReviewStore port to ports.go:**

```go
// ReviewStore is the port for pull request review persistence.
type ReviewStore interface {
    CreateReview(ctx context.Context, review *PullRequestReview) error
    ListReviewsByPRID(ctx context.Context, prID int64) ([]*PullRequestReview, error)

    CreateReviewComment(ctx context.Context, comment *ReviewComment) error
    ListReviewComments(ctx context.Context, reviewID int64) ([]*ReviewComment, error)
}
```

**Step 4: Add `ReviewStore` to the composite `Store` interface:**

```go
type Store interface {
    // ... existing stores ...
    PullRequestStore
    ReviewStore  // <-- add this

    Transaction(ctx context.Context, fn func(tx Store) error) error
    Ping(ctx context.Context) error
    io.Closer
}
```

---

## Phase B: SQLite Review Store

### Task 2: Implement SQLite review store adapter

**Files:**
- Create: `internal/infra/sqlite/review.go`

```go
package sqlite

import (
    "context"
    "database/sql"

    "github.com/Work-Fort/Combine/internal/domain"
)

func createReview(ctx context.Context, q querier, review *domain.PullRequestReview) error {
    res, err := q.ExecContext(ctx,
        `INSERT INTO pull_request_reviews (pr_id, author_id, state, body) VALUES (?, ?, ?, ?)`,
        review.PRID, review.AuthorID, review.State, review.Body,
    )
    if err != nil {
        return err
    }
    id, _ := res.LastInsertId()
    review.ID = id

    // Insert review comments.
    for i := range review.Comments {
        review.Comments[i].ReviewID = id
        if err := createReviewComment(ctx, q, &review.Comments[i]); err != nil {
            return err
        }
    }

    row := q.QueryRowContext(ctx, `SELECT created_at FROM pull_request_reviews WHERE id = ?`, id)
    return row.Scan(&review.CreatedAt)
}

func listReviewsByPRID(ctx context.Context, q querier, prID int64) ([]*domain.PullRequestReview, error) {
    rows, err := q.QueryContext(ctx,
        `SELECT id, pr_id, author_id, state, body, created_at
         FROM pull_request_reviews WHERE pr_id = ? ORDER BY created_at ASC`, prID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var reviews []*domain.PullRequestReview
    for rows.Next() {
        var r domain.PullRequestReview
        if err := rows.Scan(&r.ID, &r.PRID, &r.AuthorID, &r.State, &r.Body, &r.CreatedAt); err != nil {
            return nil, err
        }
        r.Comments, err = listReviewComments(ctx, q, r.ID)
        if err != nil {
            return nil, err
        }
        reviews = append(reviews, &r)
    }
    return reviews, rows.Err()
}

func createReviewComment(ctx context.Context, q querier, comment *domain.ReviewComment) error {
    res, err := q.ExecContext(ctx,
        `INSERT INTO review_comments (review_id, path, line, side, body, updated_at)
         VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
        comment.ReviewID, comment.Path, comment.Line, comment.Side, comment.Body,
    )
    if err != nil {
        return err
    }
    id, _ := res.LastInsertId()
    comment.ID = id
    row := q.QueryRowContext(ctx, `SELECT created_at, updated_at FROM review_comments WHERE id = ?`, id)
    return row.Scan(&comment.CreatedAt, &comment.UpdatedAt)
}

func listReviewComments(ctx context.Context, q querier, reviewID int64) ([]domain.ReviewComment, error) {
    rows, err := q.QueryContext(ctx,
        `SELECT id, review_id, path, line, side, body, created_at, updated_at
         FROM review_comments WHERE review_id = ? ORDER BY path, line`, reviewID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var comments []domain.ReviewComment
    for rows.Next() {
        var c domain.ReviewComment
        if err := rows.Scan(&c.ID, &c.ReviewID, &c.Path, &c.Line, &c.Side, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
            return nil, err
        }
        comments = append(comments, c)
    }
    return comments, rows.Err()
}

// Store methods.

func (s *Store) CreateReview(ctx context.Context, review *domain.PullRequestReview) error {
    return createReview(ctx, s.q(), review)
}

func (s *Store) ListReviewsByPRID(ctx context.Context, prID int64) ([]*domain.PullRequestReview, error) {
    return listReviewsByPRID(ctx, s.q(), prID)
}

func (s *Store) CreateReviewComment(ctx context.Context, comment *domain.ReviewComment) error {
    return createReviewComment(ctx, s.q(), comment)
}

func (s *Store) ListReviewComments(ctx context.Context, reviewID int64) ([]domain.ReviewComment, error) {
    return listReviewComments(ctx, s.q(), reviewID)
}

// txStore methods.

func (ts *txStore) CreateReview(ctx context.Context, review *domain.PullRequestReview) error {
    return createReview(ctx, ts.q(), review)
}

func (ts *txStore) ListReviewsByPRID(ctx context.Context, prID int64) ([]*domain.PullRequestReview, error) {
    return listReviewsByPRID(ctx, ts.q(), prID)
}

func (ts *txStore) CreateReviewComment(ctx context.Context, comment *domain.ReviewComment) error {
    return createReviewComment(ctx, ts.q(), comment)
}

func (ts *txStore) ListReviewComments(ctx context.Context, reviewID int64) ([]domain.ReviewComment, error) {
    return listReviewComments(ctx, ts.q(), reviewID)
}
```

**Verification:** `go build ./...` compiles.

---

## Phase C: Postgres Review Store

### Task 3: Add Postgres review store adapter

**Files:**
- Create: `internal/infra/postgres/review.go`

Mirror the SQLite adapter with `$N` placeholders. The review tables are already created by migration 004 (from plan 7a).

**Verification:** `go build ./...` compiles.

---

## Phase D: Review REST API

### Task 4: Add review API endpoints

**Files:**
- Create: `internal/infra/httpapi/api_reviews.go`
- Modify: `internal/infra/httpapi/server.go`

**Step 1: Create `api_reviews.go`:**

```go
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

// RegisterReviewRoutes registers review API routes.
func RegisterReviewRoutes(r *mux.Router) {
    r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/reviews", handleListReviews).Methods("GET")
    r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/reviews", handleSubmitReview).Methods("POST")
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
        review.Comments = append(review.Comments, domain.ReviewComment{
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
```

**Step 2: Register routes in `server.go`:**

Add after `RegisterPullRequestRoutes(api)`:
```go
RegisterReviewRoutes(api)
```

**Verification:** `go build ./...` compiles.

---

## Phase E: Review Webhook Event

### Task 5: Add review webhook event constructor

**Files:**
- Modify: `internal/infra/webhook/pull_request.go`

**Step 1: Add review event type and constructor:**

```go
type ReviewPayload struct {
    ID       int64       `json:"id"`
    Author   User        `json:"author"`
    State    string      `json:"state"`
    Body     string      `json:"body"`
    Comments int         `json:"comments_count"`
}

type PullRequestReviewEvent struct {
    Common
    Sender      IdentitySender     `json:"sender"`
    PullRequest PullRequestPayload `json:"pull_request"`
    Review      ReviewPayload      `json:"review"`
}

func NewPullRequestReviewEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest, review *domain.PullRequestReview) (PullRequestReviewEvent, error) {
    return PullRequestReviewEvent{
        Common:      buildPRCommon(ctx, repo, EventPullRequestReview),
        Sender:      identitySender(identity),
        PullRequest: buildPRPayload(pr),
        Review: ReviewPayload{
            ID:       review.ID,
            Author:   User{Username: identity.Username},
            State:    string(review.State),
            Body:     review.Body,
            Comments: len(review.Comments),
        },
    }, nil
}
```

**Verification:** `go build ./...` compiles.

---

## Phase F: E2E Test Harness Extensions

### Task 6: Add PR helper methods to the E2E test client

**Files:**
- Modify: E2E test harness client (check `tests/e2e/` or the `harness` package)

Add helper methods to the API client used in E2E tests:

```go
func (c *APIClient) CreatePullRequest(t *testing.T, repo, title, body, source, target string) map[string]any {
    t.Helper()
    payload := map[string]any{
        "title":         title,
        "body":          body,
        "source_branch": source,
        "target_branch": target,
    }
    resp := c.DoJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/pulls", repo), payload)
    if resp.StatusCode != http.StatusCreated {
        t.Fatalf("create PR: status %d", resp.StatusCode)
    }
    return c.ParseJSON(t, resp)
}

func (c *APIClient) GetPullRequest(t *testing.T, repo string, number int) map[string]any {
    t.Helper()
    resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d", repo, number), nil)
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("get PR: status %d", resp.StatusCode)
    }
    return c.ParseJSON(t, resp)
}

func (c *APIClient) ListPullRequests(t *testing.T, repo string) []map[string]any {
    t.Helper()
    resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls", repo), nil)
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("list PRs: status %d", resp.StatusCode)
    }
    return c.ParseJSONArray(t, resp)
}

func (c *APIClient) MergePullRequest(t *testing.T, repo string, number int, method string) map[string]any {
    t.Helper()
    payload := map[string]any{"merge_method": method}
    resp := c.DoJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/merge", repo, number), payload)
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("merge PR: status %d", resp.StatusCode)
    }
    return c.ParseJSON(t, resp)
}

func (c *APIClient) SubmitReview(t *testing.T, repo string, number int, state, body string, comments []map[string]any) map[string]any {
    t.Helper()
    payload := map[string]any{
        "state": state,
        "body":  body,
    }
    if comments != nil {
        payload["comments"] = comments
    }
    resp := c.DoJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/reviews", repo, number), payload)
    if resp.StatusCode != http.StatusCreated {
        t.Fatalf("submit review: status %d", resp.StatusCode)
    }
    return c.ParseJSON(t, resp)
}

func (c *APIClient) GetPullRequestDiff(t *testing.T, repo string, number int) string {
    t.Helper()
    resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/diff", repo, number), nil)
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("get PR diff: status %d", resp.StatusCode)
    }
    body, _ := io.ReadAll(resp.Body)
    resp.Body.Close()
    return string(body)
}

func (c *APIClient) GetPullRequestFiles(t *testing.T, repo string, number int) []map[string]any {
    t.Helper()
    resp := c.DoRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/files", repo, number), nil)
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("get PR files: status %d", resp.StatusCode)
    }
    return c.ParseJSONArray(t, resp)
}
```

Also add a helper to push a branch:

```go
func GitCheckoutBranch(t *testing.T, dir, branch string) {
    t.Helper()
    cmd := exec.Command("git", "checkout", "-b", branch)
    cmd.Dir = dir
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("git checkout -b %s: %v\n%s", branch, err, out)
    }
}

func GitPushBranch(t *testing.T, dir, keyPath, remote, branch string) {
    t.Helper()
    cmd := exec.Command("git", "push", remote, branch)
    cmd.Dir = dir
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", keyPath))
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("git push %s %s: %v\n%s", remote, branch, err, out)
    }
}
```

**Verification:** Helpers compile.

---

## Phase G: E2E Tests

### Task 7: Write comprehensive PR E2E tests

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Add PR CRUD test:**

```go
func TestPullRequestCreate(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "pr-test", false)

    // Push main branch with initial commit.
    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "initial commit")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/pr-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    // Create feature branch with a change.
    harness.GitCheckoutBranch(t, repoDir, "feature")
    harness.GitAddCommit(t, repoDir, "feature.txt", "new feature\n", "add feature")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "feature")

    // Create PR.
    pr := client.CreatePullRequest(t, "pr-test", "Add feature", "This adds a feature", "feature", "main")
    if pr["number"] != float64(1) {
        t.Errorf("PR number = %v, want 1", pr["number"])
    }
    if pr["status"] != "open" {
        t.Errorf("PR status = %v, want open", pr["status"])
    }
    if pr["source_branch"] != "feature" {
        t.Errorf("source_branch = %v, want feature", pr["source_branch"])
    }

    // Get PR.
    got := client.GetPullRequest(t, "pr-test", 1)
    if got["title"] != "Add feature" {
        t.Errorf("title = %v", got["title"])
    }

    // List PRs.
    prs := client.ListPullRequests(t, "pr-test")
    if len(prs) != 1 {
        t.Errorf("expected 1 PR, got %d", len(prs))
    }
}
```

**Step 2: Add shared numbering test (issues + PRs):**

```go
func TestSharedNumberSequence(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "seq-test", false)

    // Push branches for PR.
    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/seq-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "feature")
    harness.GitAddCommit(t, repoDir, "f.txt", "f\n", "feature")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "feature")

    // Issue #1
    issue := client.CreateIssue(t, "seq-test", "Bug", "")
    if issue["number"] != float64(1) {
        t.Errorf("issue number = %v, want 1", issue["number"])
    }

    // PR #2
    pr := client.CreatePullRequest(t, "seq-test", "Feature", "", "feature", "main")
    if pr["number"] != float64(2) {
        t.Errorf("PR number = %v, want 2", pr["number"])
    }

    // Issue #3
    issue2 := client.CreateIssue(t, "seq-test", "Another bug", "")
    if issue2["number"] != float64(3) {
        t.Errorf("issue2 number = %v, want 3", issue2["number"])
    }
}
```

**Step 3: Add diff/files test:**

```go
func TestPullRequestDiffAndFiles(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "diff-test", false)

    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/diff-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "changes")
    harness.GitAddCommit(t, repoDir, "new-file.txt", "content\n", "add new file")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "changes")

    client.CreatePullRequest(t, "diff-test", "Changes", "", "changes", "main")

    // Check diff.
    diff := client.GetPullRequestDiff(t, "diff-test", 1)
    if !strings.Contains(diff, "new-file.txt") {
        t.Errorf("diff should mention new-file.txt, got:\n%s", diff)
    }

    // Check files.
    files := client.GetPullRequestFiles(t, "diff-test", 1)
    if len(files) != 1 {
        t.Fatalf("expected 1 changed file, got %d", len(files))
    }
    if files[0]["filename"] != "new-file.txt" {
        t.Errorf("filename = %v", files[0]["filename"])
    }
    if files[0]["status"] != "added" {
        t.Errorf("status = %v, want added", files[0]["status"])
    }
}
```

**Step 4: Add merge test:**

```go
func TestPullRequestMerge(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "merge-test", false)

    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/merge-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "feature")
    harness.GitAddCommit(t, repoDir, "feature.txt", "new\n", "feature commit")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "feature")

    client.CreatePullRequest(t, "merge-test", "Feature", "", "feature", "main")

    // Merge.
    merged := client.MergePullRequest(t, "merge-test", 1, "merge")
    if merged["status"] != "merged" {
        t.Errorf("status = %v, want merged", merged["status"])
    }
    if merged["merged_at"] == nil {
        t.Error("merged_at should be set")
    }

    // Clone and verify the merge landed.
    cloneDir := filepath.Join(t.TempDir(), "clone")
    harness.GitCloneSSH(t, sshURL, d.PrivKeyPath, cloneDir)
    if _, err := os.Stat(filepath.Join(cloneDir, "feature.txt")); os.IsNotExist(err) {
        t.Error("feature.txt should exist after merge")
    }
}
```

**Step 5: Add squash merge test:**

```go
func TestPullRequestSquashMerge(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "squash-test", false)

    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/squash-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "multi")
    harness.GitAddCommit(t, repoDir, "a.txt", "a\n", "commit 1")
    harness.GitAddCommit(t, repoDir, "b.txt", "b\n", "commit 2")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "multi")

    client.CreatePullRequest(t, "squash-test", "Multi", "", "multi", "main")
    merged := client.MergePullRequest(t, "squash-test", 1, "squash")
    if merged["status"] != "merged" {
        t.Errorf("status = %v", merged["status"])
    }

    // Verify squash produced fewer commits.
    cloneDir := filepath.Join(t.TempDir(), "clone")
    harness.GitCloneSSH(t, sshURL, d.PrivKeyPath, cloneDir)
    log := harness.GitLog(t, cloneDir)
    // Squash should result in init + squash = 2 commits, not init + 2 feature = 3.
    if count := strings.Count(log, "commit "); count != 2 {
        t.Errorf("expected 2 commits after squash, got %d:\n%s", count, log)
    }
}
```

**Step 6: Add review test:**

```go
func TestPullRequestReview(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "review-test", false)

    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/review-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "feature")
    harness.GitAddCommit(t, repoDir, "feature.txt", "code\n", "add code")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "feature")

    client.CreatePullRequest(t, "review-test", "Feature", "", "feature", "main")

    // Submit review with line comment.
    review := client.SubmitReview(t, "review-test", 1, "approved", "LGTM", []map[string]any{
        {"path": "feature.txt", "line": 1, "body": "nice line"},
    })
    if review["state"] != "approved" {
        t.Errorf("state = %v", review["state"])
    }

    // List reviews.
    resp := client.DoRequest(t, "GET", "/api/v1/repos/review-test/pulls/1/reviews", nil)
    var reviews []map[string]any
    json.NewDecoder(resp.Body).Decode(&reviews)
    resp.Body.Close()
    if len(reviews) != 1 {
        t.Errorf("expected 1 review, got %d", len(reviews))
    }
}
```

**Step 7: Add close/reopen test:**

```go
func TestPullRequestCloseReopen(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "close-test", false)

    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/close-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "feature")
    harness.GitAddCommit(t, repoDir, "f.txt", "f\n", "feat")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "feature")

    client.CreatePullRequest(t, "close-test", "Feature", "", "feature", "main")

    // Close.
    resp := client.DoJSON(t, "PATCH", "/api/v1/repos/close-test/pulls/1", map[string]any{"status": "closed"})
    var closed map[string]any
    json.NewDecoder(resp.Body).Decode(&closed)
    resp.Body.Close()
    if closed["status"] != "closed" {
        t.Errorf("status = %v, want closed", closed["status"])
    }
    if closed["closed_at"] == nil {
        t.Error("closed_at should be set")
    }

    // Reopen.
    resp = client.DoJSON(t, "PATCH", "/api/v1/repos/close-test/pulls/1", map[string]any{"status": "open"})
    var reopened map[string]any
    json.NewDecoder(resp.Body).Decode(&reopened)
    resp.Body.Close()
    if reopened["status"] != "open" {
        t.Errorf("status = %v, want open", reopened["status"])
    }
    if reopened["closed_at"] != nil {
        t.Error("closed_at should be nil on reopen")
    }
}
```

**Step 8: Add commit keywords test:**

```go
func TestPullRequestAutoCloseIssue(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)
    client := d.APIClient(t, "testuser")
    client.CreateRepo(t, "autoclose-test", false)

    // Create issue #1.
    client.CreateIssue(t, "autoclose-test", "Bug to fix", "")

    // Push branches.
    repoDir := t.TempDir()
    harness.GitInit(t, repoDir)
    harness.GitAddCommit(t, repoDir, "readme.txt", "hello\n", "init")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/autoclose-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, repoDir, "origin", sshURL)
    harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

    harness.GitCheckoutBranch(t, repoDir, "fix")
    harness.GitAddCommit(t, repoDir, "fix.txt", "fixed\n", "fix the bug")
    harness.GitPushBranch(t, repoDir, d.PrivKeyPath, "origin", "fix")

    // PR #2 with "fixes #1" in body.
    client.CreatePullRequest(t, "autoclose-test", "Fix bug", "fixes #1", "fix", "main")
    client.MergePullRequest(t, "autoclose-test", 2, "merge")

    // Verify issue #1 is closed.
    issue := client.GetIssue(t, "autoclose-test", 1)
    if issue["status"] != "closed" {
        t.Errorf("issue status = %v, want closed", issue["status"])
    }
}
```

**Verification:** Run `go test -v ./tests/e2e/ -run TestPullRequest` — all tests pass.

---

## Summary

After completing Plan 7c:
- Review system: submit reviews with approve/changes_requested/commented states
- Line-level review comments attached to file paths and line numbers
- Review webhook event fires on review submission
- Comprehensive E2E tests covering:
  - PR CRUD (create, get, list, update)
  - Shared number sequence (issues + PRs interleaved)
  - Diff, commits, and files endpoints
  - Merge (merge commit strategy)
  - Squash merge
  - Close/reopen
  - Review submission and listing
  - Auto-close issues via commit keywords
