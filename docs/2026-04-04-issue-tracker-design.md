# Issue Tracker Design

## Overview

Add a lightweight issue tracker to Combine. Issues are scoped to repositories,
support markdown bodies, labels, assignees, and a shallow status model
(`open`, `in_progress`, `closed`). Comments provide threaded discussion.

This is the minimum needed for standalone viability as a Git forge and for
integration with Flow (workflow engine). When Flow is composed, it projects
richer process state onto Combine's shallow statuses.

## Domain Types

### IssueStatus

```go
type IssueStatus string

const (
    IssueStatusOpen       IssueStatus = "open"
    IssueStatusInProgress IssueStatus = "in_progress"
    IssueStatusClosed     IssueStatus = "closed"
)
```

### IssueResolution

```go
type IssueResolution string

const (
    IssueResolutionNone      IssueResolution = ""
    IssueResolutionFixed     IssueResolution = "fixed"
    IssueResolutionWontfix   IssueResolution = "wontfix"
    IssueResolutionDuplicate IssueResolution = "duplicate"
)
```

### Issue

```go
type Issue struct {
    ID         int64           // Auto-increment per repo
    RepoID     int64           // FK to repos.id
    AuthorID   string          // FK to identities.id (Passport UUID)
    Title      string
    Body       string          // Markdown
    Status     IssueStatus
    Resolution IssueResolution
    AssigneeID *string         // FK to identities.id, nullable
    Labels     []string        // Denormalized from issue_labels table
    CreatedAt  time.Time
    UpdatedAt  time.Time
    ClosedAt   *time.Time      // Set when status changes to closed
}
```

Issue IDs are auto-increment **per repo**, not globally. The `issues` table
uses a global autoincrement PK (`id`), plus a `number` column that is
per-repo. The `number` is what appears in the API and UI. The global `id` is
internal only.

Revised struct:

```go
type Issue struct {
    ID         int64           // Global autoincrement PK (internal)
    Number     int64           // Per-repo issue number (user-facing)
    RepoID     int64
    AuthorID   string
    Title      string
    Body       string
    Status     IssueStatus
    Resolution IssueResolution
    AssigneeID *string
    Labels     []string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    ClosedAt   *time.Time
}
```

### IssueComment

```go
type IssueComment struct {
    ID        int64
    IssueID   int64   // FK to issues.id (global PK)
    AuthorID  string  // FK to identities.id
    Body      string  // Markdown
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

## Store Port

```go
// IssueStore is the port for issue persistence.
type IssueStore interface {
    // Issues
    CreateIssue(ctx context.Context, issue *Issue) error
    GetIssueByNumber(ctx context.Context, repoID int64, number int64) (*Issue, error)
    ListIssues(ctx context.Context, repoID int64, opts IssueListOptions) ([]*Issue, error)
    UpdateIssue(ctx context.Context, issue *Issue) error

    // Labels
    SetIssueLabels(ctx context.Context, issueID int64, labels []string) error

    // Comments
    CreateIssueComment(ctx context.Context, comment *IssueComment) error
    ListIssueComments(ctx context.Context, issueID int64) ([]*IssueComment, error)
}

// IssueListOptions controls filtering for ListIssues.
type IssueListOptions struct {
    Status     *IssueStatus
    Label      *string
    AssigneeID *string
}
```

`CreateIssue` assigns the next per-repo `number` atomically:

```sql
INSERT INTO issues (repo_id, number, author_id, title, body, status)
VALUES (?, (SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE repo_id = ?), ?, ?, ?, 'open')
```

`IssueStore` is added to the composite `Store` interface.

## Database Schema

### issues

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK AUTOINCREMENT | Global internal PK |
| number | INTEGER NOT NULL | Per-repo issue number |
| repo_id | INTEGER NOT NULL | FK repos(id) ON DELETE CASCADE |
| author_id | TEXT NOT NULL | FK identities(id) |
| title | TEXT NOT NULL | |
| body | TEXT NOT NULL DEFAULT '' | |
| status | TEXT NOT NULL DEFAULT 'open' | |
| resolution | TEXT NOT NULL DEFAULT '' | |
| assignee_id | TEXT | FK identities(id), nullable |
| created_at | DATETIME | |
| updated_at | DATETIME | |
| closed_at | DATETIME | |

Unique constraint: `(repo_id, number)`.

Index: `(repo_id, status)` for filtered listing.

### issue_labels

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK AUTOINCREMENT | |
| issue_id | INTEGER NOT NULL | FK issues(id) ON DELETE CASCADE |
| label | TEXT NOT NULL | |

Unique constraint: `(issue_id, label)`.

### issue_comments

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER PK AUTOINCREMENT | |
| issue_id | INTEGER NOT NULL | FK issues(id) ON DELETE CASCADE |
| author_id | TEXT NOT NULL | FK identities(id) |
| body | TEXT NOT NULL | |
| created_at | DATETIME | |
| updated_at | DATETIME | |

## REST API

All endpoints under `/api/v1/` require Passport auth.

### List Issues

```
GET /api/v1/repos/{repo}/issues?status=open&label=bug&assignee=uuid
```

Response: `200 OK`
```json
[
  {
    "number": 1,
    "title": "Fix login",
    "body": "...",
    "status": "open",
    "resolution": "",
    "author": {"id": "uuid-1", "username": "alice"},
    "assignee": {"id": "uuid-2", "username": "bob"},
    "labels": ["bug"],
    "created_at": "...",
    "updated_at": "...",
    "closed_at": null
  }
]
```

### Create Issue

```
POST /api/v1/repos/{repo}/issues
```

Request:
```json
{
  "title": "Fix login",
  "body": "Details here...",
  "labels": ["bug"],
  "assignee_id": "uuid-2"
}
```

Response: `201 Created` — issue object.

### Get Issue

```
GET /api/v1/repos/{repo}/issues/{number}
```

Response: `200 OK` — issue object.

### Update Issue

```
PATCH /api/v1/repos/{repo}/issues/{number}
```

Request (all fields optional):
```json
{
  "title": "Updated title",
  "body": "Updated body",
  "status": "closed",
  "resolution": "fixed",
  "labels": ["bug", "urgent"],
  "assignee_id": "uuid-2"
}
```

Response: `200 OK` — updated issue object.

Status transitions: any status can move to any other status. When status
changes to `closed`, `closed_at` is set. When status changes away from
`closed`, `closed_at` is cleared.

### List Comments

```
GET /api/v1/repos/{repo}/issues/{number}/comments
```

Response: `200 OK`
```json
[
  {
    "id": 1,
    "author": {"id": "uuid-1", "username": "alice"},
    "body": "I can reproduce this.",
    "created_at": "...",
    "updated_at": "..."
  }
]
```

### Add Comment

```
POST /api/v1/repos/{repo}/issues/{number}/comments
```

Request:
```json
{
  "body": "I can reproduce this."
}
```

Response: `201 Created` — comment object.

## Webhook Events

New event constants added to `internal/infra/webhook/event.go`:

```go
EventIssueOpened        Event = 7
EventIssueStatusChanged Event = 8
EventIssueClosed        Event = 9
EventIssueComment       Event = 10
```

### issue_opened

```json
{
  "event": "issue_opened",
  "repository": { ... },
  "sender": {"id": "uuid-1", "username": "alice"},
  "issue": {
    "number": 1,
    "title": "Fix login",
    "body": "...",
    "status": "open",
    "author": {"id": "uuid-1", "username": "alice"},
    "labels": ["bug"]
  }
}
```

### issue_status_changed

```json
{
  "event": "issue_status_changed",
  "repository": { ... },
  "sender": {"id": "uuid-1", "username": "alice"},
  "issue": { ... },
  "old_status": "open",
  "new_status": "in_progress"
}
```

### issue_closed

```json
{
  "event": "issue_closed",
  "repository": { ... },
  "sender": {"id": "uuid-1", "username": "alice"},
  "issue": { ... },
  "resolution": "fixed"
}
```

### issue_comment

```json
{
  "event": "issue_comment",
  "repository": { ... },
  "sender": {"id": "uuid-1", "username": "alice"},
  "issue": { ... },
  "comment": {
    "id": 1,
    "body": "I can reproduce this.",
    "author": {"id": "uuid-1", "username": "alice"},
    "created_at": "..."
  }
}
```

## Relationship to Existing Models

- **Repo**: Issues belong to a repo via `repo_id`. Deleting a repo cascades
  to delete all its issues, labels, and comments.
- **Identity**: `author_id` and `assignee_id` reference `identities.id`
  (Passport UUIDs). The REST API resolves identity usernames for display.
- **Webhooks**: Issue events use the existing webhook delivery infrastructure.
  Repo webhooks that subscribe to issue events receive deliveries.

## Design Decisions

1. **Per-repo numbering via `number` column**: A global autoincrement `id` is
   the real PK for foreign keys. A separate `number` column provides
   human-friendly per-repo numbering. This avoids complex composite keys.

2. **Labels as a separate table**: Rather than a JSON array column, labels use
   a join table for efficient filtering (`WHERE EXISTS (SELECT 1 FROM
   issue_labels ...)`).

3. **Shallow status model**: Only three statuses. Flow projects richer state.
   This keeps Combine simple for standalone use.

4. **Sender in webhook payloads uses Identity**: Webhook `sender` fields use
   `{id, username}` from the Identity model, not the legacy User model.
