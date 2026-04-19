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

## 7. Pull Requests + Commit Keywords ✅

[Design](2026-04-04-pull-requests-design.md) · [Plan 7a](plans/2026-04-04-pull-requests-7a-plan.md) · [Plan 7b](plans/2026-04-04-pull-requests-7b-plan.md) · [Plan 7c](plans/2026-04-04-pull-requests-7c-plan.md)

Pull requests (GitHub-style). Shared number sequence with issues per repo.
CRUD + diff/commits/files API at `/api/v1/repos/{repo}/pulls`. Merge with
strategies (merge commit, squash, rebase). Mergeability check. PR reviews
with line-level comments (approve, request_changes, comment). Webhook events
(opened, closed, merged, review). Commit message keywords (`closes #N`,
`fixes #N`, `resolves #N`) auto-close issues on merge. 27 E2E tests.

## 8. Flow Integration ✅

[Design](2026-04-05-flow-integration-design.md) · [Plan](plans/2026-04-05-flow-integration-plan.md)

Webhook registration REST API (`/api/v1/repos/{repo}/webhooks`) for
programmatic webhook management. Five endpoints: create, list, get, update,
delete. Events specified as strings, stored as integers. Push webhook payload
already includes commit details (SHA, message, author, timestamp).
Commit-issue linking handled by Flow's Combine adapter using push webhook
data.

## 9. MCP Bridge ✅

[Design](2026-04-05-mcp-bridge-design.md) · [Plan](plans/2026-04-05-mcp-bridge-plan.md)

Standalone `combine mcp-bridge` command that runs an MCP server on stdio,
exposing Combine's REST API as MCP tools. 25 tools covering repos, issues,
pull requests, webhooks, and SSH keys. Uses `mcp-go` library. Configured
with `--server-url` and `--api-key` flags. E2E tested.

## 10. Docker Image + Release Flow ✅

[Design](2026-04-07-docker-release-design.md) · [Plan](plans/2026-04-07-docker-release-plan.md)

Multi-stage Dockerfile (golang build + alpine runtime) with version info
injected via ldflags. GitHub Actions release workflow publishes to GHCR on
`v*` tags with semver and SHA tagging. Build cache via GitHub Actions cache.

## 11. CI/CD

Build and deployment pipelines using Nexus.

---

## Deferred

### btrfs Quota Support

Per-repo btrfs subvolumes with configurable quotas. Deferred — requires
capability management and sidecar binaries (elevated permissions pattern).
Not part of the initial core feature set.

---

## Bugs / Follow-ups

- [ ] **No mise tasks** — `combine/lead` has neither `mise.toml` nor `.mise/tasks/`. Build goes through `go build` directly, which breaks the "use mise tasks" convention established for all other Go services in WorkFort. Add parity tasks (`build:release`, `build:dev`, `test`, `lint`, `docker:build`, `install:local`) matching the shape in hive/sharkfin/flow.
- [ ] **No standard `/ui/health` manifest** — Combine returns `{service, routes, version}` from `/ui/health` where Pylon expects `{name, label, route, ws_paths, ...}`. Pylon marks it connected but with empty `name`/`label`, so it shows as an unnamed dot in Scope's top nav and `pylon.ServiceByName("combine")` returns nothing. Align the response shape with the manifest Pylon consumes.

---

## Test Coverage Gaps

### Convention: every conditional `t.Skip` must be cross-referenced here

Any conditional `t.Skip` in an e2e or integration test MUST have a corresponding
entry in this section. The entry must name the test, state the condition under
which it skips, and describe the work needed to remove the skip.

A skip with no paper trail is indistinguishable from an accidental omission — and
will be treated as one during future audits. The rationale for this rule is
documented in the architecture reference:

> See `skills/lead/go-service-architecture/references/architecture-reference.md`
> §"Multi-Daemon Test Isolation (Per-Backend)" for the harness pattern and
> the anti-pattern that created this gap.

### Open: `TestLFSPushPull` skip (git-lfs not installed)

**File:** `tests/e2e/combine_test.go`  
**Condition:** `exec.LookPath("git-lfs")` returns an error (git-lfs binary absent from PATH).  
**Skip reason:** The test exercises the full LFS push/pull flow and requires the
`git-lfs` client to be installed in the test environment.  
**Work to remove:** Add `git-lfs` to the CI runner image (or Dockerfile) and ensure
it's available in the PATH when the e2e suite runs. This is an environment
provisioning gap, not a code gap.

### Open: `TestDaemonStop_KillsProcessGroup` skip (COMBINE_BINARY not set)

**File:** `tests/e2e/harness/daemon_leak_test.go`  
**Condition:** `os.Getenv("COMBINE_BINARY") == ""`.  
**Skip reason:** The test verifies that `StartDaemon` kills the daemon's process
group on cleanup. It requires a pre-built `combine` binary because it spawns the
real process and inspects OS-level PGID behavior.  
**Work to remove:** Ensure the e2e harness test runner sets `COMBINE_BINARY` before
running this test package. The `tests/e2e/` `TestMain` sets it for the main
suite; this test lives in the `harness/` sub-package and needs the same setup
wired in (or moved into the main suite where `TestMain` already provides the
binary path).

### Note: `TestValidateWebhookURL` skip field (dead code)

**File:** `internal/infra/webhook/validator_test.go`  
**Condition:** `tt.skip != ""` — but no test case in the table sets `skip`.  
**Status:** The `skip` field exists in the struct definition but is never populated,
making it dead code. Either delete the field and the `if tt.skip != ""` guard, or
document what test case should actually use it.
