# Flow Integration Design

## Context

Flow (WorkFort's workflow engine) needs to use Combine as a Git forge backend
via its GitForge adapter pattern. Flow creates and manages issues in Combine
during workflow execution, and Combine fires webhooks to Flow when events
occur (issue changes, pushes).

Most of what Flow needs already exists:
- Issue REST API (CRUD, comments, status transitions)
- Issue webhook events (opened, status_changed, closed, comment)
- Push webhook events with commit details
- Passport auth for service-to-service communication

## What Needs to Be Built

### 1. Webhook Registration REST API

**Problem:** Flow needs to programmatically register its webhook callback URL
with Combine. Currently webhooks can only be managed through the database
directly -- there is no public API.

**Solution:** Expose the existing `WebhookStore` (already implemented in both
SQLite and Postgres adapters) via five REST endpoints:

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/repos/{repo}/webhooks` | Register webhook |
| GET | `/api/v1/repos/{repo}/webhooks` | List webhooks |
| GET | `/api/v1/repos/{repo}/webhooks/{id}` | Get webhook |
| PATCH | `/api/v1/repos/{repo}/webhooks/{id}` | Update webhook |
| DELETE | `/api/v1/repos/{repo}/webhooks/{id}` | Delete webhook |

**Event name mapping:** Webhook events are stored as integers internally. The
API accepts and returns string event names (e.g., `"push"`, `"issue_opened"`).
The mapping already exists in `internal/infra/webhook/event.go` via
`ParseEvent()` and `Event.String()`.

**Request/response format:**

Create request:
```json
{
  "url": "http://flow:17200/v1/webhooks/combine",
  "secret": "shared-secret",
  "events": ["issue_opened", "issue_status_changed", "push"],
  "content_type": "json",
  "active": true
}
```

Webhook response:
```json
{
  "id": 1,
  "url": "http://flow:17200/v1/webhooks/combine",
  "events": ["issue_opened", "issue_status_changed", "push"],
  "content_type": "json",
  "active": true,
  "created_at": "2026-04-05T...",
  "updated_at": "2026-04-05T..."
}
```

Note: `secret` is write-only (accepted on create/update, never returned in
responses).

**Content type:** The existing `ContentType` in `webhook/webhook.go` supports
`json` (0) and `form` (1). Default to `json` if not specified.

**Auth:** All endpoints require Passport authentication (same middleware as
issue and repo APIs).

### 2. Push Webhook Payload -- Verify Commit Details

**Problem:** Flow's Combine adapter needs commit messages and SHAs in push
webhook payloads to detect issue references (`#N`, `fixes #N`).

**Finding:** The push webhook payload already includes full commit details.
`internal/infra/webhook/push.go` populates a `[]Commit` slice with ID
(SHA), Message, Title, Author (name, email, date), Committer, and Timestamp
for up to 20 commits per push.

**Conclusion:** No changes needed. The existing payload already satisfies
Flow's requirements. The `Commit` struct in `common.go` has all required
fields.

## Architecture Decisions

**Direct store access from handlers:** The webhook handlers access the store
directly (like issue handlers), not through the Backend. The WebhookStore
methods are straightforward CRUD with no business logic requiring Backend
orchestration.

**Route registration order:** Webhook routes use
`/repos/{repo:.+}/webhooks` which must be registered before the greedy repo
routes, same as issues and pull requests.

**No webhook URL validation beyond format:** We validate the URL is non-empty
and well-formed. SSRF protection is already handled at delivery time by
`secureHTTPClient` in `webhook.go`.

## What Is NOT In Scope

- **Commit-issue linking (LinkCommit):** Per the requirements doc, this logic
  lives in Flow's Combine adapter, not in Combine itself. Combine's only
  responsibility is providing commit data in push webhooks (already done).
- **Webhook delivery history API:** The store already supports delivery
  records but exposing them via REST is not needed for Flow integration.
- **Webhook retry/backoff:** Out of scope for initial integration.
