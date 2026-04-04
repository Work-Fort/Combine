# Remaining Features

Tracking document for planned Combine features.

## 1. Rebranding ✅

[Design](2026-04-03-rebranding-design.md) · [Plan](plans/2026-04-03-rebranding-plan.md)

Rename all Soft Serve references to Combine: binary (`soft` → `combine`),
env vars (`SOFT_SERVE_*` → `COMBINE_*`), config defaults, metric namespaces,
Dockerfile, README.

## 2. E2E Test Suite ✅

[Design](2026-04-04-e2e-tests-design.md) · [Plan](plans/2026-04-04-e2e-tests-plan.md)

Separate Go test module at `tests/e2e/` that builds the binary, spawns it as
a subprocess, and exercises Git operations across SSH and HTTP transports.
9 test scenarios covering health, push, clone, updates, error cases,
unauthorized access, and LFS.

## 3. Remove Git Daemon Protocol ✅

Strip the git:// daemon (port 9418). Legacy read-only unauthenticated protocol
not needed for Combine's target use cases.

## 4. Hexagonal Architecture Migration ✅

[Design](2026-04-03-hexagonal-architecture-design.md) · [Plan](plans/2026-04-03-hexagonal-architecture-plan.md)

Migrate from Soft Serve's `pkg/` flat layout to `internal/domain/` +
`internal/infra/` + `internal/app/` matching Nexus and Hive conventions.
Domain types as plain structs, port interfaces without leaked transaction
handles, adapter-managed persistence, Viper + XDG config, Goose migrations,
raw `database/sql` with modernc.org/sqlite and pgx/v5.

Note: Legacy config replaced with Viper. Daemon command added.

## 5. Passport Auth + Repo REST API ✅

[Design](2026-04-04-passport-auth-repo-api-design.md) · [Plan](plans/2026-04-04-passport-auth-repo-api-plan.md)

Integrate Passport for REST API authentication. Add repo management REST API
(`/api/v1/repos`) and SSH key management (`/api/v1/user/keys`). Replace
`users` table with `identities` (Passport UUID primary key, auto-provisioned).
Add `/v1/health` and `/ui/health` endpoints. Standalone mode (no Passport)
remains functional for SSH-only use.

## 6. Issue Tracker ✅

[Design](2026-04-04-issue-tracker-design.md) · [Plan](plans/2026-04-04-issue-tracker-plan.md)

Lightweight issue tracker for standalone viability and Flow integration.
Domain model (Issue, IssueComment), store implementations (SQLite + Postgres),
REST API at `/api/v1/repos/{repo}/issues`, webhook events (issue_opened,
issue_status_changed, issue_closed, issue_comment). Per-repo issue numbering.
Intentionally shallow status model (`open`, `in_progress`, `closed`) — Flow
projects richer state when composed.

## 7. MCP Bridge

`combine mcp-bridge` stdio-to-HTTP bridge for Claude Code. MCP tools for
repo management, issue CRUD, webhook configuration.

## 8. btrfs Quota Support

Per-repo btrfs subvolumes with configurable quotas. Transparent enforcement
on Git operations with clear error messages. Quota usage exposed via API.
Reuses patterns from Nexus's btrfs tooling.

## 9. Flow Integration

Combine becomes Flow's Git forge adapter. Bidirectional webhooks, status
projection (Flow owns process state, Combine owns code state). Issue status
in Combine is a projection of Flow's process state.

## 10. Merge Requests

Lightweight merge/pull request model. Diff rendering, conflict detection,
merge logic. Deferred because implementation scope is large and Flow v1 only
needs issues.

## 11. CI/CD

Build and deployment pipelines using Nexus.
