# Pull Requests Design Document

## Overview

Add full pull request (PR) functionality to Combine — create PRs from branches,
review code, compute diffs, and merge with multiple strategies. PRs share a
per-repo number sequence with issues so that `#N` references are unambiguous.

## Design Decisions

### 1. Shared Number Sequence (Issues + PRs)

Issues currently assign numbers via `SELECT COALESCE(MAX(number), 0) + 1 FROM
issues WHERE repo_id = ?`. This must be replaced with an atomic counter shared
between issues and PRs.

**Approach:** Add a `repo_counters` table with one row per repo:

```sql
CREATE TABLE repo_counters (
    repo_id     INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    next_number INTEGER NOT NULL DEFAULT 1
);
```

A helper function `nextNumber(ctx, q, repoID)` atomically increments and returns
the next number. Both `createIssue` and `createPullRequest` call this instead of
computing `MAX(number) + 1`. Existing repos get a counter initialized to
`MAX(issue.number) + 1` via a data migration in the same migration file.

### 2. PR Domain Model

```go
type PullRequestStatus string

const (
    PullRequestStatusOpen   PullRequestStatus = "open"
    PullRequestStatusMerged PullRequestStatus = "merged"
    PullRequestStatusClosed PullRequestStatus = "closed"
)

type MergeMethod string

const (
    MergeMethodMerge  MergeMethod = "merge"
    MergeMethodSquash MergeMethod = "squash"
    MergeMethodRebase MergeMethod = "rebase"
)

type ReviewState string

const (
    ReviewStatePending          ReviewState = "pending"
    ReviewStateApproved         ReviewState = "approved"
    ReviewStateChangesRequested ReviewState = "changes_requested"
    ReviewStateCommented        ReviewState = "commented"
)

type PullRequest struct {
    ID           int64             // Global PK
    Number       int64             // Per-repo number (shared with issues)
    RepoID       int64
    AuthorID     string            // FK to identities.id
    Title        string
    Body         string
    SourceBranch string
    TargetBranch string
    Status       PullRequestStatus
    MergeMethod  *MergeMethod      // Set when merged
    MergedBy     *string           // Identity ID
    AssigneeID   *string
    CreatedAt    time.Time
    UpdatedAt    time.Time
    MergedAt     *time.Time
    ClosedAt     *time.Time
}

type PullRequestReview struct {
    ID        int64
    PRID      int64       // FK to pull_requests.id
    AuthorID  string
    State     ReviewState
    Body      string
    CreatedAt time.Time
}

type ReviewComment struct {
    ID        int64
    ReviewID  int64
    Path      string      // File path
    Line      int         // Line number in diff
    Side      string      // "left" or "right"
    Body      string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 3. Mergeability Check

Mergeability is computed on demand (not stored) using `git merge-tree`:

```
git merge-tree --write-tree <target-branch> <source-branch>
```

This command (Git 2.38+) performs a virtual merge without touching the working
tree or index. Exit code 0 = clean merge, non-zero = conflicts. The output
includes the tree hash and any conflict markers.

Alternatively, for older Git, use `git merge-base` + `git merge --no-commit
--no-ff` in a temporary worktree. We prefer `merge-tree` for simplicity.

### 4. Merge Execution

Since `git-module` does not expose merge operations, we shell out directly:

- **Merge commit**: `git merge --no-ff <source>` on target branch
- **Squash**: `git merge --squash <source>` + `git commit`
- **Rebase**: `git rebase <target> <source>` then fast-forward target

All operations happen on bare repos using `GIT_DIR`. For bare repos, we use
`git merge-tree` to compute the result tree, then create the merge commit
directly with `git commit-tree`.

### 5. Diff Between Branches

Use `git-module`'s existing capabilities:

1. `MergeBase(target, source)` — find common ancestor
2. `Diff(mergeBase + ".." + source)` — compute diff from base to source

For the commits list: `git log <merge-base>..<source>`.

For changed files with stats: parse the `Diff` object's `Files` field to
extract additions, deletions, and file names.

### 6. Commit Keywords

Parse commit messages and PR bodies for patterns like `closes #N`, `fixes #N`,
`resolves #N`. When a PR is merged or commits are pushed to the default branch,
auto-close the referenced issues.

Regex: `(?i)\b(close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)`

### 7. Review System

Reviews are scoped to a PR. Each review has a state (approve, request_changes,
comment) and an optional body. Review comments are attached to specific file
lines in the diff.

The aggregate review state of a PR is computed by taking the most recent review
from each reviewer — if any reviewer has `changes_requested` and hasn't
subsequently approved, the aggregate is `changes_requested`.

### 8. API Design

All endpoints under `/api/v1/repos/{repo}/pulls/...`:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/pulls` | List PRs (filter: status, author) |
| POST | `/pulls` | Create PR |
| GET | `/pulls/{number}` | Get PR (includes mergeable) |
| PATCH | `/pulls/{number}` | Update PR (title, body, assignee) |
| POST | `/pulls/{number}/merge` | Merge PR |
| GET | `/pulls/{number}/diff` | Unified diff |
| GET | `/pulls/{number}/commits` | Commits in PR |
| GET | `/pulls/{number}/files` | Changed files with stats |
| GET | `/pulls/{number}/reviews` | List reviews |
| POST | `/pulls/{number}/reviews` | Submit review |

### 9. Webhook Events

New event types (continuing from EventIssueComment = 10):

| Const | Value | Fires when |
|-------|-------|------------|
| `EventPullRequestOpened` | 11 | PR created |
| `EventPullRequestClosed` | 12 | PR closed without merge |
| `EventPullRequestMerged` | 13 | PR merged |
| `EventPullRequestReview` | 14 | Review submitted |

## Implementation Split

### Plan 7a: Shared Number Sequence + PR Domain + Store + CRUD API

- `repo_counters` table and `nextNumber()` helper
- Refactor issue creation to use shared counter
- PR domain types and `PullRequestStore` port
- SQLite + Postgres migrations (004_pull_requests.sql)
- SQLite + Postgres PR store adapters
- PR CRUD API endpoints (create, get, list, update)
- ~1200 lines

### Plan 7b: Diff, Merge, and Commit Keywords

- Git operations: diff between branches, merge-base, mergeability check
- Merge execution (merge, squash, rebase) on bare repos
- Diff/commits/files API endpoints
- Merge API endpoint
- Commit keyword parsing (`closes #N`) on merge
- PR webhook events (opened, closed, merged)
- ~1300 lines

### Plan 7c: Reviews + E2E Tests

- Review domain model and `ReviewStore` port
- Review + review comment store adapters
- Review API endpoints (list, submit)
- Line-level review comments
- Review webhook event
- Comprehensive E2E tests for all PR functionality
- ~1200 lines

## Database Schema

### New Tables (Migration 004)

```sql
-- Shared number sequence for issues and PRs
CREATE TABLE repo_counters (
    repo_id     INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    next_number INTEGER NOT NULL DEFAULT 1
);

-- Initialize from existing issues
INSERT INTO repo_counters (repo_id, next_number)
SELECT repo_id, MAX(number) + 1
FROM issues
GROUP BY repo_id;

-- Ensure all repos have a counter
INSERT OR IGNORE INTO repo_counters (repo_id, next_number)
SELECT id, 1 FROM repos;

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
```

## Risk Assessment

1. **Bare repo merge**: Git merge operations typically need a working tree.
   Mitigation: use `git merge-tree` (Git 2.38+) for virtual merges, then
   `git commit-tree` to create merge commits directly.

2. **Shared counter race conditions**: Two concurrent creates could race on the
   counter. Mitigation: the `UPDATE ... SET next_number = next_number + 1`
   followed by `SELECT` is atomic within a transaction in both SQLite and
   Postgres.

3. **Git 2.38 requirement**: `merge-tree --write-tree` requires Git 2.38+.
   Fallback: detect Git version at startup and use temp-worktree approach for
   older versions. Most modern distros ship 2.38+.
