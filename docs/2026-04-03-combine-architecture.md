# Combine Architecture

## Overview

Combine is a self-hostable Git forge for the WorkFort platform, forked from
[Soft Serve](https://github.com/charmbracelet/soft-serve) by Charm. It provides
Git hosting over SSH, HTTP, and the Git protocol, with Git LFS support, access
control, and webhooks.

Combine extends Soft Serve with issue tracking and deeper integration into the
WorkFort ecosystem — particularly with Flow (workflow engine) and Passport
(authentication).

**License**: MIT (inherited from Soft Serve)

## Design Principles

1. **Standalone viable.** Combine is a fully functional Git forge on its own.
   Someone can deploy Combine without any other WorkFort service and have a
   working alternative to Gitea, Forgejo, or GitHub.
2. **Platform enhanced.** When composed with Flow, Combine's issues and events
   integrate into structured business processes. Flow projects process state onto
   Combine's lightweight issue model.
3. **Upstream compatible.** Maintain compatibility with Soft Serve's existing
   features. Customizations are additive — new models, new API routes, new
   webhook events — not modifications to existing behavior.

## Current State (Soft Serve Baseline)

### Existing Models

| Model | Description |
|-------|-------------|
| **Repo** | Git repository with name, description, visibility, mirror flag |
| **User** | Username, admin flag, SSH keys, password auth |
| **Collab** | User-repo access relationships with access levels |
| **Access Token** | Per-user tokens with name, expiration, hashed storage |
| **Webhook** | Per-repo webhooks with URL, secret, content-type, delivery tracking |
| **Public Key** | SSH public keys for user authentication |
| **Settings** | Server-level configuration |
| **LFS** | Git LFS objects and locks |

### Existing Webhook Events

| Event | Fires when |
|-------|------------|
| `push` | Commits pushed |
| `branch_tag_create` | Ref created |
| `branch_tag_delete` | Ref deleted |
| `collaborator` | User added/removed from repo |
| `repository` | Repo created/deleted/renamed |
| `repository_visibility_change` | Repo visibility changed |

### Architecture

- **SSH server** (charm.land/wish) — Git operations over SSH
- **HTTP server** (gorilla/mux) — Git HTTP transport, LFS, health checks
- **Database** — SQLite or PostgreSQL via sqlx, with migration support
- **Git hooks** — extensible hook system via `.d/` directories
- **Config** — YAML + environment variables (SOFT_SERVE_*)

**Note:** Soft Serve includes a TUI accessible over SSH. This should be removed
as part of the migration — Combine users interact via the web UI (Scope) and
Git. The SSH server remains for Git transport only.

## Planned Additions

### Phase 1: Issue Tracker (v1)

Add a lightweight issue tracker to Combine. This is the minimum needed for
standalone viability as a Git forge and for integration with Flow.

#### Issue Model

| Field | Type | Description |
|-------|------|-------------|
| ID | int | Auto-increment per repo |
| RepoID | int | Parent repository |
| AuthorID | int | User who created it |
| Title | string | Short description |
| Body | string | Full details (markdown) |
| Status | enum | `open`, `in_progress`, `closed` |
| Resolution | enum | `fixed`, `wontfix`, `duplicate`, `null` |
| Labels | []string | Categorization tags |
| AssigneeID | int | Assigned user (optional) |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |
| ClosedAt | timestamp | When status changed to closed (optional) |

Design notes:
- Status model is intentionally shallow (`open`, `in_progress`, `closed`). When
  Flow is composed, it projects richer process state onto these statuses. When
  standalone, users manage status directly.
- Resolution is tracked separately from status so closed issues carry context.

#### Issue Comments

| Field | Type | Description |
|-------|------|-------------|
| ID | int | Auto-increment |
| IssueID | int | Parent issue |
| AuthorID | int | User who wrote it |
| Body | string | Comment content (markdown) |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |

#### New Webhook Events

| Event | Payload | Fires when |
|-------|---------|------------|
| `issue_opened` | Issue | New issue created |
| `issue_status_changed` | Issue + old/new status | Status transitions |
| `issue_closed` | Issue + resolution | Issue closed |
| `issue_comment` | Comment + Issue | New comment on issue |

#### API Routes (additions)

Issue management over HTTP:

- `GET /api/repos/{repo}/issues` — list issues (filterable by status, label, assignee)
- `POST /api/repos/{repo}/issues` — create issue
- `GET /api/repos/{repo}/issues/{id}` — get issue
- `PATCH /api/repos/{repo}/issues/{id}` — update issue
- `GET /api/repos/{repo}/issues/{id}/comments` — list comments
- `POST /api/repos/{repo}/issues/{id}/comments` — add comment

### Phase 2: Merge Requests (v2, deferred)

Add a lightweight merge/pull request model. Deferred because:
- Implementation scope is large (diff rendering, conflict detection, merge logic)
- Flow v1 only needs issues from the GitForge port
- Standalone Combine is useful with just issues initially

#### Merge Request Model (draft)

| Field | Type | Description |
|-------|------|-------------|
| ID | int | Auto-increment per repo |
| RepoID | int | Parent repository |
| AuthorID | int | User who created it |
| Title | string | Short description |
| Body | string | Full details (markdown) |
| SourceBranch | string | Branch with changes |
| TargetBranch | string | Branch to merge into |
| Status | enum | `open`, `merged`, `closed` |
| CreatedAt | timestamp | |
| UpdatedAt | timestamp | |
| MergedAt | timestamp | When merged (optional) |

New webhook events: `merge_request_opened`, `merge_request_merged`,
`merge_request_closed`.

### Phase 3: Passport Integration

Replace Soft Serve's built-in auth with Passport:
- JWT/API key validation via `Work-Fort/Passport/go/service-auth`
- Passport manages user identities; Combine trusts Passport tokens
- SSH key management may remain in Combine (SSH-specific concern)
- Service identity for Combine registered in Passport during seed

### Phase 4: Flow Integration

When deployed alongside Flow, Combine becomes Flow's Git forge adapter:
- Flow creates/updates issues via Combine's API
- Combine fires webhooks to Flow on issue and push events
- Flow is authoritative for process state; Combine's issue status is a projection
- Combine's issue status changes from direct user action are reported to Flow
  via webhook for reconciliation

### Future: OpenSpec Recognition

Combine could recognize OpenSpec directory structures (`openspec/` with
`proposal.md`, `specs/`, `design.md`, `tasks.md`) within repositories and
surface them in the TUI and HTTP UI. When composed with Flow, a new OpenSpec
in a repo could trigger a webhook that auto-creates a work item at the Planning
step of an SDLC workflow.

## Relationship to Flow

```
Flow (workflow engine)
  |
  |-- GitForge port interface
  |     |
  |     +-- Combine adapter (REST API + webhooks)
  |     +-- GitHub adapter (future)
  |
  |-- Inbound webhooks from Combine:
  |     push, issue_opened, issue_status_changed, issue_closed
  |
  |-- Outbound calls to Combine:
  |     create_issue, update_issue_status, close_issue, link_commit
  |
  +-- State ownership:
        Flow owns process state (which workflow step a work item is at)
        Combine owns code state (repo, branches, commits, issue details)
        Issue status in Combine is a projection of Flow's process state
```

### State Ownership Rules

| Scenario | Authority | Behavior |
|----------|-----------|----------|
| Combine standalone | Combine | Users manage issue status directly |
| Combine + Flow | Flow | Flow projects status onto Combine issues |
| Direct Combine status change while Flow is active | Combine notifies Flow | Flow reconciles — its process state takes precedence |

## Adopting WorkFort Conventions

Soft Serve's codebase predates WorkFort's architectural conventions. A key part
of the roadmap is migrating Combine to match the patterns used across all other
Go services (Hive, Sharkfin, Pylon, Flow, Nexus).

### Hexagonal Architecture (Ports & Adapters)

Soft Serve currently mixes infrastructure and domain logic. Migrate to the
standard WorkFort project layout:

```
cmd/
  daemon/         -- HTTP + SSH server, systemd service
  mcp-bridge/     -- stdio-to-HTTP MCP bridge
  admin/          -- CLI admin commands
domain/           -- Core types, port interfaces, business rules (zero infra deps)
infra/
  sqlite/         -- SQLite store implementation
  postgres/       -- PostgreSQL store implementation
  httpapi/        -- REST API handlers
  ssh/            -- SSH server for Git transport (no TUI)
  git/            -- Git operations adapter
  storage/        -- Repository storage (btrfs-aware)
  mcp/            -- MCP tool handlers
```

The domain layer defines port interfaces (RepoStore, IssueStore, UserStore,
WebhookEmitter, etc.) with zero infrastructure dependencies. The infra layer
provides implementations.

### Dual Database Support

Soft Serve currently uses sqlx with either SQLite or PostgreSQL. Migrate to the
WorkFort pattern:

- **SQLite**: via `modernc.org/sqlite` (pure Go, BSD-3-Clause)
- **PostgreSQL**: via `pgx/v5`
- **Migrations**: Goose (consistent with Hive, Flow, Nexus)
- Both implementations satisfy the same domain port interfaces

### Repository Storage and btrfs Quota Support

Repositories are stored on btrfs filesystems — both locally (Nexus VMs use btrfs)
and in cloud (Kubernetes volumes mounted as btrfs). Combine should leverage btrfs
subvolumes for per-repo storage isolation and quota enforcement:

- Each repository gets its own btrfs subvolume
- Quotas are configurable per repo (e.g., 100MB, 500MB, 1GB)
- Quota enforcement is transparent — Git operations fail gracefully when quota
  is exceeded, with clear error messages
- Quota usage is exposed via API for monitoring and billing
- Nexus already has btrfs quota tooling (`nexus-quota`, `nexus-btrfs`) — reuse
  patterns and potentially shared libraries

This enables:
- Hard storage limits per repo
- Preventing a single large repo from consuming all available storage
- Snapshot and clone operations via btrfs (fast, copy-on-write)

### MCP Bridge

Add MCP support following the standard pattern:

- `combine mcp-bridge` — stdio-to-HTTP MCP bridge for Claude Code
- MCP tools for repo management, issue CRUD, webhook configuration
- Enables AI agents to interact with Combine programmatically

### Standard Service Integration

- **Passport auth**: JWT/API key via `Work-Fort/Passport/go/service-auth`
- **Pylon discovery**: `/ui/health` endpoint for service registry
- **Sharkfin bot**: Bot identity for posting repo events to channels
- **Viper config**: XDG paths, YAML, env vars (`COMBINE_*`)
- **Cobra CLI**: Command factory pattern
- **charmbracelet/log**: JSON structured logging to file
- **mise.toml**: Go version + build task management

## Roadmap

1. **Rebranding and conventions** — rename binary/config, migrate to hexagonal
   layout, adopt Cobra/Viper/charmbracelet patterns, add mise.toml
2. **Dual database** — migrate from sqlx to modernc.org/sqlite + pgx/v5 with
   Goose migrations
3. **Issue tracker** — domain model, store implementations, REST API, webhook events
4. **Passport auth** — replace built-in auth with service-auth, add `/ui/health`
5. **MCP bridge** — repo and issue management tools for agents
6. **btrfs quota support** — per-repo subvolumes with configurable quotas
7. **Flow integration** — GitForge adapter, bidirectional webhooks, status projection
8. **Merge requests** — data model, API, webhook events
9. **CI/CD** — build and deployment pipelines using Nexus

## Rebranding Notes

The codebase is forked from Soft Serve. Rebranding tasks:
- Module path: `github.com/Work-Fort/Combine` (done)
- README: needs replacement (currently upstream Soft Serve README)
- Binary name: `soft` → `combine`
- Config env vars: `SOFT_SERVE_*` → `COMBINE_*`
- Service discovery: add `/ui/health` endpoint for Pylon
- Project layout: migrate from `pkg/` to `domain/` + `infra/` hexagonal layout
- Remove TUI: strip SSH TUI code (pkg/ssh/ TUI components, bubbletea
  dependencies). SSH server remains for Git transport only.
