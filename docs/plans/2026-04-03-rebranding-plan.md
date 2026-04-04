# Rebranding Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rename all Soft Serve references to Combine throughout the codebase.

**Architecture:** Mechanical find-and-replace across ~25 files. No business logic changes. Binary renamed from `soft` to `combine`, env vars from `SOFT_SERVE_*` to `COMBINE_*`, config defaults updated.

**Tech Stack:** Go, Cobra CLI, caarlos0/env

---

### Task 1: Rename cmd/soft/ directory to cmd/combine/

**Files:**
- Rename: `cmd/soft/` -> `cmd/combine/`
- Modify: `cmd/combine/main.go` (update import paths)
- Modify: `cmd/cmd.go` (update import paths)

**Step 1: Rename the directory**

```bash
git mv cmd/soft cmd/combine
```

**Step 2: Update import paths in cmd/combine/main.go**

Replace all occurrences of `cmd/soft/` with `cmd/combine/` in the import block:
```go
"github.com/Work-Fort/Combine/cmd/combine/admin"
"github.com/Work-Fort/Combine/cmd/combine/hook"
"github.com/Work-Fort/Combine/cmd/combine/serve"
```

**Step 3: Update the Cobra root command**

In `cmd/combine/main.go`, change:
```go
rootCmd = &cobra.Command{
    Use:          "combine",
    Short:        "A self-hostable Git forge",
    Long:         "Combine is a self-hostable Git forge for the WorkFort platform.",
```

**Step 4: Update the man page copyright**

In `cmd/combine/main.go`, change the copyright to:
```go
manPage = manPage.WithSection("Copyright", "(C) 2021-2023 Charmbracelet, Inc.\n"+
    "(C) 2026 WorkFort\n"+
    "Released under MIT license.")
```

**Step 5: Verify it compiles**

```bash
go build ./cmd/combine/
```

**Step 6: Commit**

```bash
git add -A && git commit -m "refactor: rename cmd/soft to cmd/combine"
```

---

### Task 2: Rename environment variables SOFT_SERVE_* to COMBINE_*

**Files:**
- Modify: `pkg/config/config.go` (central env var definitions)
- Modify: `pkg/config/config_test.go`
- Modify: `cmd/combine/serve/serve.go`
- Modify: `cmd/combine/hook/hook.go`
- Modify: `pkg/backend/hooks.go`
- Modify: `pkg/web/git.go`
- Modify: `pkg/ssh/cmd/git.go`
- Modify: `pkg/hooks/gen.go`
- Modify: `Dockerfile`

**Step 1: Update pkg/config/config.go**

This is the central file. Changes needed:

1. Line 17: `var binPath = "soft"` -> `var binPath = "combine"`

2. Lines 180-223: In `Environ()`, replace all `SOFT_SERVE_` prefixes with
   `COMBINE_`:
   ```go
   fmt.Sprintf("COMBINE_BIN_PATH=%s", binPath),
   fmt.Sprintf("COMBINE_CONFIG_LOCATION=%s", c.ConfigPath()),
   fmt.Sprintf("COMBINE_DATA_PATH=%s", c.DataPath),
   // ... all 42 env vars
   ```

3. Lines 229-238: Update `IsDebug()` and `IsVerbose()`:
   ```go
   func IsDebug() bool {
       debug, _ := strconv.ParseBool(os.Getenv("COMBINE_DEBUG"))
       return debug
   }
   func IsVerbose() bool {
       verbose, _ := strconv.ParseBool(os.Getenv("COMBINE_VERBOSE"))
       return IsDebug() && verbose
   }
   ```

4. Lines 269-276: Update `parseEnv()`:
   ```go
   if err := env.ParseWithOptions(cfg, env.Options{
       Prefix: "COMBINE_",
   }); err != nil {
   ```
   And:
   ```go
   if initialAdminKeysEnv := os.Getenv("COMBINE_INITIAL_ADMIN_KEYS"); initialAdminKeysEnv != "" {
   ```

5. Lines 314-319: Update `DefaultDataPath()`:
   ```go
   func DefaultDataPath() string {
       dp := os.Getenv("COMBINE_DATA_PATH")
   ```

6. Lines 325-328: Update `ConfigPath()`:
   ```go
   if path := os.Getenv("COMBINE_CONFIG_LOCATION"); exist(path) {
   ```

**Step 2: Update all other files with SOFT_SERVE_ references**

In each file, replace `SOFT_SERVE_` with `COMBINE_`:

- `cmd/combine/serve/serve.go`: `SOFT_SERVE_TESTRUN` -> `COMBINE_TESTRUN`
- `cmd/combine/hook/hook.go`: `SOFT_SERVE_REPO_NAME` -> `COMBINE_REPO_NAME`
- `pkg/backend/hooks.go`: `SOFT_SERVE_PUBLIC_KEY`, `SOFT_SERVE_USERNAME` ->
  `COMBINE_PUBLIC_KEY`, `COMBINE_USERNAME`
- `pkg/web/git.go`: `SOFT_SERVE_REPO_NAME`, `SOFT_SERVE_REPO_PATH`,
  `SOFT_SERVE_LOG_PATH` -> `COMBINE_REPO_NAME`, `COMBINE_REPO_PATH`,
  `COMBINE_LOG_PATH`
- `pkg/ssh/cmd/git.go`: All `SOFT_SERVE_*` env vars in git hook setup
- `pkg/hooks/gen.go`: `SOFT_SERVE_BIN_PATH`, `SOFT_SERVE_REPO_NAME`

**Step 3: Update Dockerfile**

Replace:
```dockerfile
SOFT_SERVE_DATA_PATH -> COMBINE_DATA_PATH
SOFT_SERVE_INITIAL_ADMIN_KEYS -> COMBINE_INITIAL_ADMIN_KEYS
```

**Step 4: Update tests**

In `pkg/config/config_test.go`, update all `SOFT_SERVE_*` env var references
to `COMBINE_*` and the name assertion:
```go
assert(cfg.Name == "Combine")
```

**Step 5: Verify**

```bash
go build ./...
go test ./pkg/config/...
# Grep for any remaining SOFT_SERVE references:
grep -r "SOFT_SERVE" --include="*.go" .
grep -r "SOFT_SERVE" Dockerfile
```

The grep should return zero results.

**Step 6: Commit**

```bash
git add -A && git commit -m "refactor: rename env vars from SOFT_SERVE_* to COMBINE_*"
```

---

### Task 3: Update config defaults and branding strings

**Files:**
- Modify: `pkg/config/config.go` (defaults)
- Modify: `pkg/config/file.go` (template)
- Modify: `pkg/config/testdata/config.yaml`
- Modify: `cmd/combine/serve/server.go` (comments)
- Modify: `cmd/combine/hook/hook.go` (description)
- Modify: `cmd/combine/serve/serve.go` (hook example string)

**Step 1: Update DefaultConfig() in pkg/config/config.go**

```go
return &Config{
    Name:     "Combine",
    // ...
    SSH: SSHConfig{
        // ...
        KeyPath:       filepath.Join("ssh", "combine_host_ed25519"),
        ClientKeyPath: filepath.Join("ssh", "combine_client_ed25519"),
    },
    // ...
    DB: DBConfig{
        Driver: "sqlite",
        DataSource: "combine.db" +
            "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
    },
}
```

**Step 2: Update config template in pkg/config/file.go**

Replace `# Soft Serve Server configurations` with
`# Combine Server configurations`

**Step 3: Update testdata**

In `pkg/config/testdata/config.yaml`, replace header comment.

**Step 4: Update branding in command descriptions**

- `cmd/combine/serve/server.go`: Comments referencing "Soft Serve"
- `cmd/combine/hook/hook.go`: `Long: "Handles Soft Serve git server hooks."` ->
  `Long: "Handles Combine git server hooks."`
- `cmd/combine/serve/serve.go`: `"Hi from Soft Serve update hook!"` ->
  `"Hi from Combine update hook!"`

**Step 5: Verify**

```bash
go build ./...
go test ./pkg/config/...
# Check for remaining branding:
grep -ri "soft.serve\|soft_serve\|softserve" --include="*.go" .
```

**Step 6: Commit**

```bash
git add -A && git commit -m "refactor: update config defaults and branding to Combine"
```

---

### Task 4: Update metric/logging namespaces

**Files:**
- Modify: `pkg/ssh/cmd/git.go`
- Modify: `pkg/ssh/middleware.go`
- Modify: `pkg/ssh/ssh.go`
- Modify: `pkg/web/git.go`
- Modify: `pkg/web/goget.go`

**Step 1: Replace all namespace strings**

In each file, replace `Namespace: "soft_serve"` with `Namespace: "combine"`.

Use find-and-replace across all five files.

**Step 2: Verify**

```bash
go build ./...
grep -r '"soft_serve"' --include="*.go" .
```

Should return zero results.

**Step 3: Commit**

```bash
git add -A && git commit -m "refactor: update metric namespaces to combine"
```

---

### Task 5: Replace README and clean up dependencies

**Files:**
- Rewrite: `README.md`
- Modify: `go.mod` / `go.sum` (via go mod tidy)

**Step 1: Write new README**

Replace the entire README.md with a Combine-specific one:

```markdown
# Combine

A self-hostable Git forge for the WorkFort platform.

Combine provides Git hosting over SSH, HTTP, and the Git protocol, with
Git LFS support, access control, and webhooks. It is forked from
[Soft Serve](https://github.com/charmbracelet/soft-serve) by Charm.

## Quick Start

```bash
# Build
go build -o combine ./cmd/combine/

# Run (creates a data/ directory for repos, keys, and database)
COMBINE_INITIAL_ADMIN_KEYS="$(cat ~/.ssh/id_ed25519.pub)" ./combine serve
```

## Configuration

Configuration is loaded from `data/config.yaml` and can be overridden with
environment variables prefixed with `COMBINE_`.

| Variable | Description | Default |
|----------|-------------|---------|
| `COMBINE_DATA_PATH` | Data directory | `data` |
| `COMBINE_NAME` | Server name | `Combine` |
| `COMBINE_SSH_LISTEN_ADDR` | SSH listen address | `:23231` |
| `COMBINE_HTTP_LISTEN_ADDR` | HTTP listen address | `:23232` |
| `COMBINE_DB_DRIVER` | Database driver (`sqlite` or `postgres`) | `sqlite` |
| `COMBINE_INITIAL_ADMIN_KEYS` | Admin SSH public keys | |

## License

[MIT](LICENSE) (inherited from Soft Serve)
```

**Step 2: Clean up stale dependencies**

```bash
go mod tidy
```

This should remove unused indirect dependencies from the TUI removal
(bubbletea, lipgloss).

**Step 3: Verify the build**

```bash
go build ./...
go test ./...
```

**Step 4: Commit**

```bash
git add -A && git commit -m "docs: replace Soft Serve README with Combine, run go mod tidy"
```

---

### Task 6: Final verification

**Step 1: Full grep for any remaining Soft Serve references**

```bash
grep -ri "soft.serve" --include="*.go" --include="*.yaml" --include="*.md" --include="Dockerfile" .
grep -ri "SOFT_SERVE" --include="*.go" --include="*.yaml" --include="*.md" --include="Dockerfile" .
grep -ri '"soft"' --include="*.go" . | grep -v "_test.go" | grep -v vendor
```

Review any hits -- some may be legitimate (e.g., comments about the fork
origin in the architecture doc).

**Step 2: Full build and test**

```bash
go build ./...
go test ./...
```

**Step 3: Verify binary name**

```bash
go build -o combine ./cmd/combine/
./combine --version
./combine --help
```

The help output should show "combine" as the command name and "Combine" in
descriptions.
