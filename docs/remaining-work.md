# Remaining Work — Combine

Tracks all work for the Combine Git forge. Items are roughly priority-ordered
within each section.

- [Architecture](2026-04-03-combine-architecture.md)

---

## Active: Phase 1 — Rebranding and Conventions

### Rebranding (Soft Serve → Combine)
- [x] Rename `cmd/soft/` to `cmd/combine/`, update Cobra root command
- [x] Rename env vars `SOFT_SERVE_*` → `COMBINE_*`
- [x] Update config defaults (server name, DB filename, SSH key paths)
- [x] Update metric/logging namespaces (`soft_serve` → `combine`)
- [x] Fix remaining references (Dockerfile, hook filenames, SSH description)
- [x] Replace README, run `go mod tidy`
- [Design](2026-04-03-rebranding-design.md) · [Plan](plans/2026-04-03-rebranding-plan.md)

### E2E Test Suite
- [x] Test harness (build binary, start daemon, SSH key management, Git helpers)
- [x] TestHealth — `/readyz` and `/livez`
- [x] TestSSHPushCreatesRepo — push auto-creates repo
- [x] TestSSHClone — clone back over SSH
- [x] TestSSHPushUpdate — push additional commits
- [x] TestHTTPClone — clone public repo over HTTP
- [x] TestHTTPCloneNonExistent — error on missing repo
- [x] TestSSHCloneNonExistent — error on missing repo
- [x] TestSSHPushUnauthorized — rejected with unknown key
- [x] TestLFSPushPull — LFS tracked file round-trip (skips if git-lfs not installed)
- [Design](2026-04-04-e2e-tests-design.md) · [Plan](plans/2026-04-04-e2e-tests-plan.md)

### Remove Git Daemon Protocol
- [ ] Strip git:// daemon (port 9418) code and config
- Plan pending

### Hexagonal Architecture Migration
- [ ] Phase A: Domain layer (types, errors, ports, context)
- [ ] Phase B: Viper + XDG config
- [ ] Phase C: SQLite and PostgreSQL store adapters
- [ ] Phase D: Move existing infra packages
- [ ] Phase E: Refactor Backend + adapters to domain types
- [ ] Phase F: Rewrite command layer with Nexus/Hive DI
- [ ] Phase G: Delete old packages, verify
- [Design](2026-04-03-hexagonal-architecture-design.md) · [Plan](plans/2026-04-03-hexagonal-architecture-plan.md)

---

## Planned: Phase 2 — Dual Database

- [ ] Migrate from sqlx to modernc.org/sqlite + pgx/v5 with Goose migrations

Note: This is largely covered by the hexagonal architecture migration (Phase C
of that plan creates both adapters from scratch). May just need verification
and any remaining migration work.

---

## Planned: Phase 3 — Issue Tracker

- [ ] Domain model (Issue, IssueComment)
- [ ] Store implementations (SQLite + Postgres)
- [ ] REST API routes (`/api/repos/{repo}/issues/...`)
- [ ] Webhook events (issue_opened, issue_status_changed, issue_closed, issue_comment)

---

## Planned: Phase 4 — Passport Auth

- [ ] Replace built-in auth with Passport JWT/API key validation
- [ ] Add `/ui/health` endpoint for Pylon service discovery

---

## Planned: Phase 5 — MCP Bridge

- [ ] `combine mcp-bridge` stdio-to-HTTP bridge
- [ ] MCP tools for repo management, issue CRUD

---

## Future

- Phase 6: btrfs quota support
- Phase 7: Flow integration
- Phase 8: Merge requests
- Phase 9: CI/CD
