# Flow Integration Requirements

## Purpose

Enable Flow (workflow engine) to use Combine as a Git forge backend via the
GitForge adapter pattern. Flow creates and manages issues in Combine as part of
workflow execution, and Combine fires webhooks to Flow when issue state changes.

## What Exists Today

Combine already has most of what Flow needs:

**Issue REST API (all under `/api/v1`):**
- `GET /repos/{repo}/issues` — list (filters: status, label, assignee)
- `POST /repos/{repo}/issues` — create (title, body, labels, assignee_id)
- `GET /repos/{repo}/issues/{number}` — get by number
- `PATCH /repos/{repo}/issues/{number}` — update (title, body, status, resolution, labels, assignee_id)
- `GET /repos/{repo}/issues/{number}/comments` — list comments
- `POST /repos/{repo}/issues/{number}/comments` — create comment

**Issue webhook events:**
- `issue_opened`, `issue_status_changed`, `issue_closed`, `issue_comment`

**Issue model:**
- Number (per-repo), Title, Body, Status (open/in_progress/closed), Resolution
  (fixed/wontfix/duplicate), Labels, AssigneeID, AuthorID

## What Flow Needs (GitForge Port)

Flow's GitForge adapter calls these operations during workflow transitions:

| Flow Port Method | Combine Endpoint | Status |
|---|---|---|
| `CreateIssue(repo, title, body, labels)` | `POST /repos/{repo}/issues` | Ready |
| `UpdateIssueStatus(repo, issueID, status)` | `PATCH /repos/{repo}/issues/{number}` | Ready |
| `GetIssue(repo, issueID)` | `GET /repos/{repo}/issues/{number}` | Ready |
| `ListIssues(repo, filters)` | `GET /repos/{repo}/issues` | Ready |
| `LinkCommit(repo, issueID, commitSHA)` | — | **Missing** |
| `RegisterWebhook(repo, events, callbackURL)` | — | **Missing** |

## What Needs to Be Built

### 1. Webhook Registration REST API

**What:** Expose the existing `WebhookStore` (CreateWebhook, UpdateWebhook,
DeleteWebhook) via REST endpoints so external services can register webhook
callbacks programmatically.

**Why:** Flow needs to register its own webhook callback URL with Combine so
it receives issue events. Currently webhooks can only be managed through the
database — there is no public API.

**Endpoints:**

- `POST /api/v1/repos/{repo}/webhooks` — register a webhook
  ```json
  {
    "url": "http://flow:17200/v1/webhooks/combine",
    "secret": "shared-secret",
    "events": ["issue_opened", "issue_status_changed", "issue_closed", "push"],
    "active": true
  }
  ```
  Response: 201 with webhook object (id, url, events, active, created_at)

- `GET /api/v1/repos/{repo}/webhooks` — list registered webhooks

- `GET /api/v1/repos/{repo}/webhooks/{id}` — get webhook details

- `PATCH /api/v1/repos/{repo}/webhooks/{id}` — update webhook (url, events,
  active, secret)

- `DELETE /api/v1/repos/{repo}/webhooks/{id}` — delete webhook

### 2. Commit-Issue Linking

**What:** Associate commits with issues so Flow can track which commits
relate to a work item's Combine issue.

**Why:** When a developer pushes a commit referencing an issue (e.g.,
`fixes #42`), Flow needs to know about the association. This also enables
"close issue on merge" patterns.

**Approach — Adapter-side convention parsing:**

Each GitForge adapter owns commit message parsing because keyword
conventions differ per forge (GitHub uses `fixes #N`, GitLab uses
`/close` quick actions, cross-project refs vary). The adapter receives
push webhooks, parses commit messages using its own forge's rules, and
calls Flow's `LinkCommit` port method with the resolved associations.

For Combine's adapter: parse commit messages for `#N`, `fixes #N`,
`closes #N` references. This logic lives in Flow's Combine adapter,
not in Combine itself.

Combine's responsibility: ensure the push webhook payload includes
commit messages and SHAs so the adapter has the raw data to parse.

### 3. Push Webhook Payload — Commit Details

**What:** Ensure the `push` webhook event payload includes commit messages
and SHAs, not just the ref update.

**Why:** Flow needs commit messages to detect issue references (`#N`) and
to display commit context in work item history.

**Verify:** Check that the existing push webhook payload includes:
```json
{
  "event": "push",
  "ref": "refs/heads/main",
  "commits": [
    {
      "sha": "abc123",
      "message": "fix: resolve login bug\n\nFixes #42",
      "author": "...",
      "timestamp": "..."
    }
  ]
}
```

If commits are not included in the push payload, add them.

## How Flow Uses These

When the SDLC workflow transitions to Implementation:
1. Flow's integration hook fires `git-forge.create_issue`
2. Flow's Combine adapter calls `POST /repos/{repo}/issues`
3. Flow stores the returned issue number as an `ExternalLink` on the work item

When a developer pushes code:
1. Combine fires `push` webhook to Flow
2. Flow's webhook handler parses commit messages for `#N` references
3. Flow updates the work item's external links

When the workflow transitions to Done:
1. Flow's integration hook fires `git-forge.close_issue`
2. Flow's Combine adapter calls `PATCH /repos/{repo}/issues/{number}`
   with `{"status": "closed", "resolution": "fixed"}`

When an issue status changes directly in Combine:
1. Combine fires `issue_status_changed` webhook to Flow
2. Flow's webhook handler reconciles — Flow's process state takes
   precedence, but the event is logged

## Authentication

Flow authenticates to Combine's API using a Passport service token
(`Authorization: Bearer <token>`). This is the same pattern used by all
WorkFort service-to-service communication.

## Implementation Order

1. **Webhook registration API** (blocker — Flow needs to register its callback)
2. **Verify push payload includes commits** (blocker — adapter needs commit
   messages and SHAs for issue reference parsing)
