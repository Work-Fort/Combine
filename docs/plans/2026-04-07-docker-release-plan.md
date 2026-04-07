# Docker Image + Release Flow — Implementation Plan

**Date:** 2026-04-07
**Design:** [docs/2026-04-07-docker-release-design.md](../2026-04-07-docker-release-design.md)
**Estimated size:** ~150 lines across 3 files

## Tasks

### 1. Replace Dockerfile with multi-stage build

Replace the existing `Dockerfile` at repo root.

```dockerfile
FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown") && \
    GIT_DATE=$(git log -1 --format=%cI 2>/dev/null || echo "unknown") && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w \
        -X github.com/Work-Fort/Combine/cmd.Version=${VERSION} \
        -X github.com/Work-Fort/Combine/cmd.CommitSHA=${GIT_SHA} \
        -X github.com/Work-Fort/Combine/cmd.CommitDate=${GIT_DATE}" \
      -o /combine ./cmd/combine

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git
COPY --from=build /combine /usr/local/bin/combine
VOLUME /combine-data
ENV COMBINE_DATA_PATH="/combine-data"
EXPOSE 23231/tcp 23232/tcp 23233/tcp
ENTRYPOINT ["combine"]
CMD ["serve"]
```

Key decisions:
- Use `golang:1.25-alpine` for build stage (includes Go, simpler than installing mise)
- Runtime needs `git` (Combine shells out to it)
- Data volume at `/combine-data` (not `/combine`, which conflicts with the binary name)
- All three ldflags populated from git metadata at build time

**Files:** `Dockerfile`

### 2. Add .dockerignore

Create `.dockerignore`:

```
.git
docs
*.md
testdata
.github
```

**Files:** `.dockerignore`

### 3. Add GitHub Actions release workflow

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: read
  packages: write

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha

      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ github.ref_name }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**Files:** `.github/workflows/release.yml`

## Verification

1. `docker build -t combine:test .` — confirm multi-stage build succeeds
2. `docker run --rm combine:test --version` — confirm version/commit info is baked in
3. `docker run --rm combine:test serve --help` — confirm runtime deps (git) are available
4. Tag a test release to verify the workflow triggers correctly

## Order of Operations

All three tasks are independent and can be implemented in a single commit.
