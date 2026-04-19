---
type: plan
step: "mise-ci-dual-backend"
title: "Combine: mise toolchain + CI dual-backend matrix"
status: complete
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: "mise-ci-dual-backend"
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: "2026-04-19"
related_plans:
  - "2026-04-19-e2e-harness-orphan-leak-hardening.md"
---

# Combine: mise toolchain + CI dual-backend matrix

## Overview

Combine has none of the constellation's standard scaffolding: no `mise.toml`,
no `.mise/tasks/`, no `.github/workflows/ci.yml`. Lint, test, and e2e are
invoked by hand. The e2e harness only runs against SQLite; `internal/infra/postgres/`
ships untested end-to-end. Three lint failures and one auto-skipped e2e test
sit in the tree.

This plan brings Combine in line with Sharkfin / Hive / Flow:

- `mise.toml` (tools only) and `.mise/tasks/{lint,test,e2e,ci,build/dev,build/release,clean}` matching Sharkfin's shell-script-per-task layout.
- `.github/workflows/ci.yml` with parallel jobs `ci` (lint + unit tests) and `e2e-postgres` (e2e against a Postgres service container). SQLite e2e runs as part of `ci` via `mise run e2e` with default backend.
- E2E harness Postgres support: backend selection via `COMBINE_DB_DRIVER` + `COMBINE_DB_DATA_SOURCE` env vars, per-test schema reset, `AltDB(t)` helper for tests that need a second DB.
- Lint fixes in `internal/infra/httpapi/passport_test.go`.
- `git-lfs` installed in CI image so `TestLFSPushPull` runs (skip removed).
- E2E timeout bumped to `600s` in the mise task — full SSH suite takes ~190s today, leaves headroom.

The work is broken into 9 discrete tasks in dependency order. Lint fixes land
first so the first CI workflow run is green.

## Prerequisites

- Combine builds locally: `go build ./cmd/combine`.
- E2E suite passes against SQLite: `cd tests/e2e && go test -v ./...`.
- `git-lfs` is available locally for the developer running `TestLFSPushPull` (Arch: `pacman -S git-lfs`).
- Developer has read Sharkfin's `.mise/tasks/*` and `.github/workflows/ci.yaml` for reference.

## Conventions adopted

- **No SPDX headers on shell scripts.** Combine is a Soft-Serve fork inheriting Charm's MIT license; existing Go files carry no per-file SPDX header. The new `.mise/tasks/*` scripts follow the same convention — no header.
- **Go toolchain `1.26.0`.** Pinned in `mise.toml` to match the majority of the constellation (hive, flow, nexus). Bumps Combine's `go.mod` from `1.25.0` to `1.26.0` (Task 2 covers the bump).
- **`golangci-lint` 2.11.4 via aqua** — constellation-wide standard.
- **e2e timeout 600s** baked into the e2e mise task — full SSH suite takes ~190s, gives ~3x headroom for slow CI.
- **`AltDB(t)` helper kept in tree** — matches Sharkfin's pattern of co-locating the helper with the harness, and Postgres e2e runs added in this plan are the first consumers via the harness's per-test schema reset.
- **CI workflow filename `ci.yaml`** — matches the constellation majority (sharkfin, flow, nexus, pylon).

## Task breakdown

### Task 1: Fix `passport_test.go` lint failures

**Why first:** the CI workflow added in Task 7 will fail on `mise run lint` if these aren't fixed. Lint fixes are isolated and trivial.

**Files:**
- Modify: `internal/infra/httpapi/passport_test.go:39` (gofumpt — single-line struct literal must be expanded)
- Modify: `internal/infra/httpapi/passport_test.go:109` (noctx — `httptest.NewRequest` is fine; the actual issue is likely missing `context.Background()` somewhere; verify with `golangci-lint run`)
- Modify: `internal/infra/httpapi/passport_test.go:136` (same as 109)

**Step 1: Reproduce the lint failures locally**

Run: `golangci-lint run ./internal/infra/httpapi/...`

Expected output: three findings — one `gofumpt` at line 39, two `noctx` at lines 109 and 136. Note the exact messages (the noctx fix depends on which call the linter flags).

**Step 2: Fix the gofumpt finding at line 39**

Current code at line 39:

```go
		var body struct{ Key string `json:"key"` }
```

Replacement:

```go
		var body struct {
			Key string `json:"key"`
		}
```

**Step 3: Fix the noctx findings at lines 109 and 136**

`httptest.NewRequest` itself is fine; `noctx` flags HTTP requests made with the default context. Inspect the linter output — likely it's flagging that the request should use `httptest.NewRequestWithContext` or `req = req.WithContext(context.Background())`. Apply the minimal change suggested by the linter message.

If both lines are `req := httptest.NewRequest("GET", "/v1/x", nil)`, replace each with:

```go
		req := httptest.NewRequestWithContext(context.Background(), "GET", "/v1/x", nil)
```

(`context` is already imported.)

**Step 4: Verify lint passes**

Run: `golangci-lint run ./internal/infra/httpapi/...`
Expected: no findings.

Run: `gofmt -l internal/infra/httpapi/passport_test.go`
Expected: empty output.

**Step 5: Verify tests still pass**

Run: `go test ./internal/infra/httpapi/...`
Expected: PASS.

**Step 6: Commit**

```bash
git commit -m "$(cat <<'EOF'
fix(httpapi): resolve passport_test lint findings

Expand inline struct literal at line 39 (gofumpt) and pass
context.Background() to httptest.NewRequest at lines 109 and 136
(noctx). Required so the upcoming CI workflow's lint job is green
on its first run.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Add `mise.toml` and bump `go.mod` to 1.26.0

**Depends on:** none.

**Files:**
- Create: `mise.toml`
- Modify: `go.mod` (line 3: `go 1.25.0` → `go 1.26.0`)
- Modify: `tests/e2e/go.mod` (same `go` directive bump if present)

**Step 1: Write `mise.toml`**

```toml
[tools]
go = "1.26.0"
"aqua:golangci/golangci-lint" = "2.11.4"
```

(Matches Hive and Flow exactly.)

**Step 2: Bump `go.mod` and `tests/e2e/go.mod`**

In `go.mod`, change line 3 from `go 1.25.0` to `go 1.26.0`.

If `tests/e2e/go.mod` has its own `go` directive, bump it the same way. Confirm by reading the first 5 lines of that file.

**Step 3: Verify mise picks it up**

Run: `mise install`
Expected: installs Go 1.26.0 and golangci-lint 2.11.4 (no errors).

Run: `mise exec -- go version`
Expected: `go version go1.26.0 linux/amd64`.

**Step 4: Verify the build still works under 1.26**

Run: `go build ./cmd/combine`
Expected: builds cleanly. If 1.26 surfaces a new vet warning or compile error, fix it (or escalate to Team Lead — toolchain bumps occasionally trip new diagnostics).

Run: `go test ./...`
Expected: PASS (no toolchain-version-related regressions).

**Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
chore(mise): add mise.toml and bump Go to 1.26.0

Pin go=1.26.0 in mise.toml and bump the go directive in go.mod and
tests/e2e/go.mod to match. Aligns Combine with the constellation
majority (hive, flow, nexus). Pin aqua:golangci/golangci-lint=2.11.4
to match every other constellation repo. Foundation for the
.mise/tasks/ scripts and CI workflow that follow.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Add `.mise/tasks/lint`, `.mise/tasks/test`, `.mise/tasks/clean`

**Depends on:** Task 2 (mise toolchain).

**Files:**
- Create: `.mise/tasks/lint`
- Create: `.mise/tasks/test`
- Create: `.mise/tasks/clean`

**Step 1: Write `.mise/tasks/lint`**

```bash
#!/usr/bin/env bash
#MISE description="Run linters"
set -euo pipefail

UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then
  echo "Unformatted files:"
  echo "$UNFORMATTED"
  exit 1
fi

go vet ./...
golangci-lint run ./...
```

`chmod +x .mise/tasks/lint`.

**Step 2: Write `.mise/tasks/test`**

```bash
#!/usr/bin/env bash
#MISE description="Run unit tests with race detection and coverage"
set -euo pipefail

BUILD_DIR=build
mkdir -p "$BUILD_DIR"

go test -v -race -coverprofile="$BUILD_DIR/coverage.out" ./...
```

`chmod +x .mise/tasks/test`.

**Step 3: Write `.mise/tasks/clean`**

```bash
#!/usr/bin/env bash
#MISE description="Clean build artifacts"
set -euo pipefail

rm -rf build
```

`chmod +x .mise/tasks/clean`.

**Step 4: Verify**

Run: `mise run lint`
Expected: PASS (Task 1 fixed all known lint issues).

Run: `mise run test`
Expected: all unit tests PASS, `build/coverage.out` exists.

Run: `mise run clean && ls build 2>&1`
Expected: `ls: cannot access 'build': No such file or directory`.

**Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
chore(mise): add lint, test, clean tasks

Match the Sharkfin/Hive/Flow .mise/tasks/ shell-script layout: one
executable per task, MISE description in a special comment, set
-euo pipefail for safety. Lint runs gofmt + go vet + golangci-lint;
test runs go test with -race and writes coverage to build/.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Add `.mise/tasks/build/dev` and `.mise/tasks/build/release`

**Depends on:** Task 2.

Combine's binary entry point is `./cmd/combine` (per `Dockerfile`). The
build tasks mirror Sharkfin's layout with the correct ldflags package path.

**Files:**
- Create: `.mise/tasks/build/dev`
- Create: `.mise/tasks/build/release`

**Step 1: Write `.mise/tasks/build/dev`**

```bash
#!/usr/bin/env bash
#MISE description="Build combine (dev, dynamic)"
#MISE sources=["**/*.go", "**/*.sql", "go.mod", "go.sum"]
#MISE outputs=["build/combine"]
set -euo pipefail

BUILD_DIR=build
BINARY_NAME=combine
GIT_SHORT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION="${VERSION:-dev-$GIT_SHORT_SHA}"

mkdir -p "$BUILD_DIR"

go build -ldflags "-X github.com/Work-Fort/Combine/cmd.Version=$VERSION" \
  -o "$BUILD_DIR/$BINARY_NAME" ./cmd/combine
echo "Built $BUILD_DIR/$BINARY_NAME"
```

`chmod +x .mise/tasks/build/dev`.

**Step 2: Write `.mise/tasks/build/release`**

```bash
#!/usr/bin/env bash
#MISE description="Build combine (release, static, stripped)"
set -euo pipefail

BUILD_DIR=build
BINARY_NAME=combine
GIT_SHORT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DATE=$(git log -1 --format=%cI 2>/dev/null || echo "unknown")
VERSION="${VERSION:-dev-$GIT_SHORT_SHA}"

mkdir -p "$BUILD_DIR"
CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w \
      -X github.com/Work-Fort/Combine/cmd.Version=$VERSION \
      -X github.com/Work-Fort/Combine/cmd.CommitSHA=$GIT_SHORT_SHA \
      -X github.com/Work-Fort/Combine/cmd.CommitDate=$GIT_DATE" \
    -o "$BUILD_DIR/$BINARY_NAME" ./cmd/combine
echo "Built $BUILD_DIR/$BINARY_NAME (release)"
```

`chmod +x .mise/tasks/build/release`.

**Step 3: Verify**

Run: `mise run build:dev`
Expected: `Built build/combine`, file exists.

Run: `./build/combine --version`
Expected: prints `combine version dev-<sha>`.

Run: `mise run build:release`
Expected: `Built build/combine (release)`. Re-run produces a smaller binary than dev.

**Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
chore(mise): add build:dev and build:release tasks

build:dev produces a development binary with sources/outputs
metadata so mise caches across no-op runs. build:release produces
a CGO-disabled, trimpath, stripped binary suitable for the
container build. Both stamp version, commit SHA, and commit date
into github.com/Work-Fort/Combine/cmd via -ldflags. Mirrors the
Sharkfin layout.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Wire e2e harness for backend selection via env vars

**Depends on:** Task 2 (so the developer can run `mise run` while iterating).

Today the harness ignores `COMBINE_DB_*` entirely — every daemon spins up
SQLite under `dataDir`. Add a forwarding path so `COMBINE_DB_DRIVER` and
`COMBINE_DB_DATA_SOURCE` propagate to the daemon, plus the per-test
Postgres schema reset that Sharkfin uses.

**Files:**
- Modify: `tests/e2e/harness/harness.go` (around `StartDaemon`, lines 84–145)
- Modify: `tests/e2e/go.mod` (add `github.com/jackc/pgx/v5` if not already required transitively)

**Step 1: Inspect current daemon env construction**

Read `tests/e2e/harness/harness.go:105-115` — note the explicit `cmd.Env` block. Backend env vars must be appended *after* `os.Environ()` but before the call to `cmd.Start()`.

**Step 2: Add Postgres schema reset to `StartDaemon`**

Insert before `cmd.Start()` (after env construction):

```go
	// Backend selection: forward COMBINE_DB_* if set; reset Postgres schema
	// per test so each daemon starts from a clean state.
	if dsn := os.Getenv("COMBINE_DB_DATA_SOURCE"); dsn != "" {
		driver := os.Getenv("COMBINE_DB_DRIVER")
		if driver == "postgres" {
			if err := resetPostgres(dsn); err != nil {
				stderrFile.Close()
				os.Remove(stderrFile.Name())
				t.Fatalf("reset postgres: %v", err)
			}
		}
		// COMBINE_DB_DATA_SOURCE and COMBINE_DB_DRIVER already inherited
		// via os.Environ(); no need to re-append.
	}
```

The `cmd.Env` block already includes `os.Environ()`, so `COMBINE_DB_DRIVER` and `COMBINE_DB_DATA_SOURCE` from the test's environment are forwarded automatically. The reset is the only new wiring.

**Step 3: Add the `resetPostgres` helper**

Append to `tests/e2e/harness/harness.go`:

```go
import (
	// add to existing import block:
	"database/sql"
	"net/url"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// resetPostgres drops and recreates the public schema so each test
// starts from a clean database. Goose migrations re-run on daemon startup.
func resetPostgres(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec("DROP SCHEMA public CASCADE"); err != nil {
		return fmt.Errorf("drop schema: %w", err)
	}
	if _, err := db.Exec("CREATE SCHEMA public"); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}
```

**Step 4: Add `AltDB(t)` helper (per Open Question 5)**

Append to `tests/e2e/harness/harness.go`:

```go
// AltDB returns a second DB DSN for use by a second daemon in the same test.
// Under SQLite (COMBINE_DB_DATA_SOURCE not set or driver != postgres) it
// returns a fresh tempfile path inside t.TempDir() — the file does not yet
// exist, so the daemon will seed it on first open. Under Postgres it
// constructs a sibling database DSN by appending "_b" to the database name,
// resets its schema, and registers a t.Cleanup that resets it again.
func AltDB(t *testing.T) string {
	t.Helper()
	if os.Getenv("COMBINE_DB_DRIVER") != "postgres" {
		return filepath.Join(t.TempDir(), "alt.db")
	}
	envDSN := os.Getenv("COMBINE_DB_DATA_SOURCE")
	if envDSN == "" {
		t.Fatalf("AltDB: COMBINE_DB_DRIVER=postgres but COMBINE_DB_DATA_SOURCE is empty")
	}

	u, err := url.Parse(envDSN)
	if err != nil {
		t.Fatalf("AltDB: parse COMBINE_DB_DATA_SOURCE %q: %v", envDSN, err)
	}
	origDB := strings.TrimPrefix(u.Path, "/")
	siblingDB := origDB + "_b"
	u.Path = "/" + siblingDB
	siblingDSN := u.String()

	adminDB, err := sql.Open("pgx", envDSN)
	if err != nil {
		t.Fatalf("AltDB: open admin connection: %v", err)
	}
	defer adminDB.Close()

	var exists bool
	if err := adminDB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", siblingDB,
	).Scan(&exists); err != nil {
		t.Fatalf("AltDB: check sibling db existence: %v", err)
	}
	if !exists {
		if _, err := adminDB.Exec("CREATE DATABASE " + siblingDB); err != nil {
			t.Fatalf("AltDB: create sibling db %q: %v", siblingDB, err)
		}
	}

	if err := resetPostgres(siblingDSN); err != nil {
		t.Fatalf("AltDB: reset sibling postgres %q: %v", siblingDSN, err)
	}
	t.Cleanup(func() {
		if err := resetPostgres(siblingDSN); err != nil {
			t.Logf("AltDB cleanup: reset sibling postgres: %v", err)
		}
	})
	return siblingDSN
}
```

**Step 5: Wire pgx driver into `tests/e2e/go.mod`**

Run: `cd tests/e2e && go mod tidy`
Expected: `github.com/jackc/pgx/v5` added (or moved from indirect to direct).

**Step 6: Verify SQLite path still works**

Run: `mise run build:dev && cd tests/e2e && go test -v -timeout 600s ./...`
Expected: all currently-passing tests still pass (SQLite default path, no Postgres env set).

**Step 7: Verify Postgres path works locally**

Start a local Postgres (developer choice — `docker run -d --rm --name pg-combine -e POSTGRES_DB=combine_test -e POSTGRES_USER=combine -e POSTGRES_PASSWORD=combine -p 5432:5432 postgres:17` or equivalent).

Run:

```bash
cd tests/e2e
COMBINE_DB_DRIVER=postgres \
  COMBINE_DB_DATA_SOURCE="postgres://combine:combine@localhost:5432/combine_test?sslmode=disable" \
  go test -v -timeout 600s ./...
```

Expected: tests pass against Postgres. Any test that currently relies on SQLite-specific behaviour (e.g. file-path assertions) will surface here — fix or skip-with-rationale before proceeding.

**Step 8: Commit**

```bash
git commit -m "$(cat <<'EOF'
test(e2e): wire harness for Postgres backend via COMBINE_DB_*

StartDaemon now resets the Postgres schema (DROP/CREATE public)
when COMBINE_DB_DRIVER=postgres so each test starts clean. The
COMBINE_DB_DATA_SOURCE and COMBINE_DB_DRIVER env vars propagate
to the daemon naturally via os.Environ() inheritance.

Adds AltDB(t) helper mirroring Sharkfin's pattern: returns a
sibling DSN (tempfile under SQLite, "_b"-suffixed database under
Postgres) so future tests can spin up two daemons sharing or
isolating their backends as needed.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Add `.mise/tasks/e2e` and `.mise/tasks/ci`

**Depends on:** Task 4 (build:dev), Task 5 (Postgres-aware harness).

**Files:**
- Create: `.mise/tasks/e2e`
- Create: `.mise/tasks/ci`

**Step 1: Write `.mise/tasks/e2e`**

```bash
#!/usr/bin/env bash
#MISE description="Run end-to-end tests (backend selected by COMBINE_DB_* env vars)"
#MISE depends=["build:dev"]
#MISE dir="tests/e2e"
set -euo pipefail

export COMBINE_BINARY="${MISE_PROJECT_ROOT}/build/combine"
go test -v -race -timeout 600s ./...
```

`chmod +x .mise/tasks/e2e`.

Note: `COMBINE_BINARY` is exported even though the current harness reads
the binary path from `combine_test.go`'s `combineBin` variable (set in
`TestMain`). Inspect `tests/e2e/combine_test.go` and `tests/e2e/harness/main_test.go`
to confirm — if the convention is a different env var, use that name. The
intent is: same convention Sharkfin uses (`SHARKFIN_BINARY` in its e2e task).

**Step 2: Write `.mise/tasks/ci`**

```bash
#!/usr/bin/env bash
#MISE description="Run all CI checks (lint + unit tests + e2e against default backend)"
#MISE depends=["lint", "test", "e2e"]
set -euo pipefail
```

`chmod +x .mise/tasks/ci`.

**Step 3: Verify**

Run: `mise run e2e`
Expected: builds the daemon, runs the SQLite e2e suite, all pass within 600s.

Run: `mise run ci`
Expected: lint → test → e2e in sequence (mise resolves the depends graph), all pass.

**Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
chore(mise): add e2e and ci tasks

e2e depends on build:dev, runs from tests/e2e, uses -timeout 600s
(full SSH suite takes ~190s; gives ~3x headroom). Backend selection
is via COMBINE_DB_DRIVER + COMBINE_DB_DATA_SOURCE — unset means
SQLite, "postgres" plus a DSN means Postgres. ci composes lint +
test + e2e via mise depends. Mirrors Sharkfin's task layout.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Add `.github/workflows/ci.yaml`

**Depends on:** Task 1 (lint clean), Task 6 (mise tasks exist).

Mirror Sharkfin's structure: one `ci` job that runs `mise run ci` (covers
lint + unit + SQLite e2e), one `e2e-postgres` job with a Postgres service
container.

**Files:**
- Create: `.github/workflows/ci.yaml`

**Step 1: Write `ci.yaml`**

```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v3
      - run: sudo apt-get update && sudo apt-get install -y git-lfs
      - run: mise run ci

  e2e-postgres:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_DB: combine_test
          POSTGRES_USER: combine
          POSTGRES_PASSWORD: combine
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 5s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v6
      - uses: jdx/mise-action@v3
      - run: sudo apt-get update && sudo apt-get install -y git-lfs
      - run: mise run build:dev
      - run: mise run e2e
        env:
          COMBINE_DB_DRIVER: postgres
          COMBINE_DB_DATA_SOURCE: postgres://combine:combine@localhost:5432/combine_test?sslmode=disable
```

Layout decisions and rationale:

- **Two parallel jobs.** Sharkfin uses this exact split: `ci` (everything by default) and `e2e-postgres` (e2e job with Postgres service container). Combine matches. SQLite e2e runs as part of `ci` because `mise run ci` depends on `e2e` and `e2e` defaults to SQLite when no `COMBINE_DB_*` env is set.
- **`git-lfs` install in both jobs.** The `ci` job runs SQLite e2e which now includes `TestLFSPushPull` (skip removed in Task 8). The `e2e-postgres` job runs the same suite against Postgres.
- **No matrix.** Sharkfin doesn't use a matrix — it uses two named jobs. Easier to read in the Actions UI.
- **`actions/checkout@v6` and `jdx/mise-action@v3`.** Match Sharkfin/Flow versions exactly.
- **No explicit Go setup step.** `mise install` (run automatically by `mise-action`) installs Go from `mise.toml`.

**Step 2: Push branch and open draft PR (developer task — not in plan)**

This is the verification gate: the workflow must run green. The plan author should not push; the developer running this plan does.

**Step 3: Verify CI is green**

After push, watch the GitHub Actions run. Both `ci` and `e2e-postgres` jobs must succeed.

If `e2e-postgres` fails on a Postgres-only bug in `internal/infra/postgres/`, file follow-up tasks — do NOT merge with red CI.

**Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
ci: add ci.yaml with parallel SQLite + Postgres e2e jobs

The ci job runs mise run ci (lint + unit tests + SQLite e2e via the
default backend). The e2e-postgres job spins up a Postgres 17
service container and re-runs e2e with COMBINE_DB_DRIVER=postgres
so internal/infra/postgres/ gets exercised end-to-end. Both jobs
install git-lfs so TestLFSPushPull no longer skips. Mirrors
Sharkfin's two-job layout exactly.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Remove `TestLFSPushPull` skip

**Depends on:** Task 7 (CI now installs git-lfs in both jobs).

**Files:**
- Modify: `tests/e2e/combine_test.go:244-248`

**Step 1: Remove the skip block**

Delete lines 244–248 inclusive — the comment, the `LookPath` check, and the `t.Skip`. The function body remains unchanged from line 250 onwards.

Replace:

```go
func TestLFSPushPull(t *testing.T) {
	// Check if git-lfs is installed
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("git-lfs not installed")
	}

	d := harness.StartDaemon(t, combineBin)
```

With:

```go
func TestLFSPushPull(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)
```

**Step 2: Drop the `os/exec` import if unused elsewhere**

After the edit, run `goimports -w tests/e2e/combine_test.go` (or rely on `gofmt` + manual edit). If `exec` is still referenced by other tests in the file, leave the import.

**Step 3: Verify locally (with git-lfs installed)**

Run: `git lfs version`
Expected: prints version (developer install gate).

Run: `mise run build:dev && cd tests/e2e && go test -v -run TestLFSPushPull -timeout 60s ./...`
Expected: PASS — push and clone of LFS-tracked binary succeeds.

**Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
test(e2e): remove TestLFSPushPull skip

CI now installs git-lfs in both the ci and e2e-postgres jobs, so
the LookPath skip is no longer needed. Local developers must have
git-lfs installed (Arch: pacman -S git-lfs) — the test will fail
fast at git lfs install if missing, which is the right signal.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Update `README.md` with mise-based developer commands

**Depends on:** Tasks 2–8 all merged.

**Files:**
- Modify: `README.md` (developer-quickstart / contributing section)

**Step 1: Read current README structure**

Read `README.md` end-to-end to find the right insertion point. If there's no existing "Development" or "Contributing" section, add one near the bottom before the License section.

**Step 2: Add a Development section**

```markdown
## Development

Combine uses [mise](https://mise.jdx.dev/) to manage the Go and
golangci-lint toolchain. With mise installed:

```bash
mise install              # install pinned toolchain
mise run lint             # gofmt + go vet + golangci-lint
mise run test             # unit tests with -race and coverage
mise run build:dev        # build ./build/combine
mise run e2e              # build then run e2e tests against SQLite
mise run ci               # lint + test + e2e (full default-backend run)
```

### E2E against Postgres

The e2e harness selects its backend via env vars. Default (unset)
is SQLite; setting both runs against Postgres:

```bash
COMBINE_DB_DRIVER=postgres \
  COMBINE_DB_DATA_SOURCE="postgres://combine:combine@localhost:5432/combine_test?sslmode=disable" \
  mise run e2e
```

The harness drops and recreates the `public` schema before each
test so runs are isolated.

### git-lfs

`TestLFSPushPull` requires `git-lfs` on the developer's PATH.
On Arch: `pacman -S git-lfs`.
```

**Step 3: Verify**

Render the README locally (any markdown previewer) and check formatting.

**Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
docs(readme): document mise-based developer workflow

Add a Development section covering mise install/run commands,
COMBINE_DB_DRIVER/COMBINE_DB_DATA_SOURCE for Postgres e2e, and the
git-lfs prerequisite for TestLFSPushPull. Replaces tribal-knowledge
"just run go test" guidance now that mise tasks and CI exist.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Verification checklist

After all tasks land:

- [ ] `mise install` succeeds on a clean checkout.
- [ ] `mise run lint` exits 0 with zero findings.
- [ ] `mise run test` exits 0; `build/coverage.out` exists.
- [ ] `mise run build:dev` produces `build/combine`; `./build/combine --version` prints a version stamp.
- [ ] `mise run build:release` produces a smaller, stripped `build/combine`.
- [ ] `mise run e2e` (default backend, SQLite) exits 0; `TestLFSPushPull` runs and passes.
- [ ] `COMBINE_DB_DRIVER=postgres COMBINE_DB_DATA_SOURCE=... mise run e2e` exits 0 against a local Postgres.
- [ ] `mise run ci` exits 0 (composes lint + test + e2e).
- [ ] GitHub Actions: `ci` job green on push to a PR branch.
- [ ] GitHub Actions: `e2e-postgres` job green on the same PR.
- [ ] No `t.Skip("git-lfs not installed")` remains in `tests/e2e/`.
- [ ] README documents the mise commands and Postgres e2e env vars.
- [ ] `internal/infra/postgres/` is now exercised end-to-end by every CI run.

## Risks and mitigations

- **Postgres-only bugs surface for the first time.** Likely. Whatever the `e2e-postgres` job catches becomes follow-up tasks. Don't merge with red CI even if "the SQLite job is green".
- **Go 1.26.0 bump may surface new vet/compile diagnostics.** Task 2 Step 4 verifies the build under the new toolchain; any new finding is fixed in that same task before proceeding.
- **e2e timeout of 600s masks slow tests.** True. Future work: per-test timeouts inside the e2e suite, profiling the SSH suite's ~190s figure to see where the time goes. Out of scope here.
