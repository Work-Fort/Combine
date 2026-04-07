# Docker Image + Release Flow Design

**Date:** 2026-04-07
**Status:** Draft

## Overview

Production Docker image and automated GHCR publishing for Combine, matching the pattern established by Sharkfin.

## Current State

The existing `Dockerfile` is a legacy artifact from Soft Serve. It expects a pre-built `combine` binary to be copied in (no build stage), uses `alpine:latest` (unpinned), and installs unnecessary packages (openssh, bash). There are no GitHub Actions workflows and no `.dockerignore`.

## Design

### Dockerfile

Multi-stage build matching Sharkfin's pattern:

**Build stage** (`alpine:3.21`):
- Go toolchain (git is not needed in the build stage; commit metadata is passed as build args)
- Combine does not use mise, so Go is installed directly via Alpine's package or a specific version download
- Build a static binary with `CGO_ENABLED=0`, trimpath, and stripped symbols
- Inject version, commit SHA, and commit date via ldflags targeting `github.com/Work-Fort/Combine/cmd`

**Runtime stage** (`alpine:3.21`):
- `ca-certificates` for HTTPS
- `git` — required at runtime for Git operations (push, pull, clone via smart HTTP and SSH)
- `openssh` — not needed; Combine implements its own SSH server via `gliderlabs/ssh`
- The binary, a data volume, and exposed ports (SSH 23231, HTTP 23232, stats 23233)

This differs from Sharkfin (which only needs `ca-certificates` at runtime) because Combine shells out to `git` for repository operations.

### ldflags

The binary accepts three ldflags, all in `github.com/Work-Fort/Combine/cmd`:

| Variable     | ldflags target                                  |
|-------------|------------------------------------------------|
| Version     | `-X github.com/Work-Fort/Combine/cmd.Version`    |
| CommitSHA   | `-X github.com/Work-Fort/Combine/cmd.CommitSHA`  |
| CommitDate  | `-X github.com/Work-Fort/Combine/cmd.CommitDate`  |

### GitHub Actions Workflow

A single workflow `.github/workflows/release.yml` triggered on push of version tags (`v*`):

1. Checkout code
2. Set up Docker Buildx
3. Log in to GHCR (`ghcr.io`)
4. Extract metadata (tags, labels) using `docker/metadata-action`
5. Build and push using `docker/build-push-action` with the version tag passed as a build arg

Tags produced:
- `ghcr.io/work-fort/combine:v1.2.3` (exact version)
- `ghcr.io/work-fort/combine:latest` (when tagged on the default branch)

### .dockerignore

Add a `.dockerignore` to speed up context transfer:

```
.git
docs
*.md
testdata
.github
```

## Differences from Sharkfin

| Aspect | Sharkfin | Combine |
|--------|----------|---------|
| Build tooling | mise + mise tasks | Direct `go build` |
| Runtime deps | ca-certificates only | ca-certificates + git |
| Ports | None exposed | 23231, 23232, 23233 |
| Volumes | None | /combine-data (data dir) |
| Entrypoint | `sharkfin daemon` | `combine serve` |
