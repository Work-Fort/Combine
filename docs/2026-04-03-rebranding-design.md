# Rebranding: Soft Serve to Combine

## Overview

Rename all Soft Serve references to Combine throughout the codebase. This is a
mechanical, low-risk change that establishes Combine's identity before the
hexagonal architecture migration rewrites import paths.

## Scope

### Binary and directory

- Rename `cmd/soft/` directory to `cmd/combine/`
- Cobra root command: `Use: "soft"` -> `Use: "combine"`
- Update `Short` and `Long` descriptions to reference Combine
- Subcommand descriptions referencing "Soft Serve" (hook, serve, server)

### Environment variables

42+ references across 11 files. Central location is `pkg/config/config.go`
`Environ()` method (lines 181-222).

- `SOFT_SERVE_*` -> `COMBINE_*` everywhere
- Key files: `pkg/config/config.go`, `cmd/soft/serve/serve.go`,
  `cmd/soft/hook/hook.go`, `pkg/backend/hooks.go`, `pkg/web/git.go`,
  `pkg/ssh/cmd/git.go`, `Dockerfile`

### Config defaults

In `pkg/config/config.go`:
- Server name: `"Soft Serve"` -> `"Combine"`
- Database filename: `"soft-serve.db"` -> `"combine.db"`
- SSH key paths: `"soft_serve_host_ed25519"` -> `"combine_host_ed25519"`,
  `"soft_serve_client_ed25519"` -> `"combine_client_ed25519"`

### Metric/logging namespaces

13+ references across SSH and web packages:
- `Namespace: "soft_serve"` -> `Namespace: "combine"` in `pkg/ssh/cmd/git.go`,
  `pkg/ssh/middleware.go`, `pkg/ssh/ssh.go`, `pkg/web/git.go`,
  `pkg/web/goget.go`

### Config template

- `pkg/config/file.go` header comment
- `pkg/config/testdata/config.yaml` header

### Tests

- `pkg/config/config_test.go` assertions checking `cfg.Name == "Soft Serve"`

### Dockerfile

- `SOFT_SERVE_DATA_PATH` and `SOFT_SERVE_INITIAL_ADMIN_KEYS` env vars

### README

Replace the upstream Soft Serve README with a Combine-specific one covering:
- What Combine is (self-hostable Git forge for WorkFort)
- Setup instructions with `combine serve`
- Environment variable reference (`COMBINE_*`)
- License (MIT, inherited from Soft Serve)

### Dependency cleanup

Run `go mod tidy` to remove stale indirect dependencies from the TUI removal
(bubbletea, lipgloss). These are no longer imported anywhere in the codebase.

## Files affected

~25 files total. No business logic changes, purely naming and branding.

## Ordering

This plan executes before the hexagonal architecture migration. Renaming first
means the hexagonal migration moves files with correct names, avoiding a second
rename pass.
