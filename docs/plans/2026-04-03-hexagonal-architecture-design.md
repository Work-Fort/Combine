# Hexagonal Architecture Migration

## Overview

Migrate Combine from Soft Serve's `pkg/` flat layout to the WorkFort hexagonal
architecture pattern used by Nexus and Hive. This is not just a directory
restructure -- it retrofits the codebase to use proper ports and adapters with
domain types as plain structs, store interfaces without leaked transaction
handles, and adapter-managed persistence.

## Design Principles

1. Match Nexus and Hive conventions exactly -- directory structure, naming,
   patterns, and dependency directions are transferable across all WorkFort
   Go services.
2. Domain layer has zero infrastructure imports.
3. Adapters implement domain port interfaces and manage their own transactions.
4. Application service (Backend) depends only on domain ports, not on raw DB
   connections or full config structs.

## Target Directory Structure

```
cmd/
  combine/
    main.go                  # Entry point, delegates to cmd.Execute()
  root.go                    # Root Cobra command, Viper init, logging setup
  daemon/
    daemon.go                # Daemon command: DI wiring, server lifecycle
  admin/                     # Admin CLI commands (kept from existing)
  hook/                      # Git hook handler command (kept from existing)

internal/
  config/
    config.go                # Viper + XDG paths, COMBINE_* env prefix

  domain/
    types.go                 # Repo, User, Collab, PublicKey, AccessToken,
                             # Settings, LFSObject, LFSLock, Webhook,
                             # WebhookEvent, WebhookDelivery -- all plain structs
    ports.go                 # RepoStore, UserStore, CollabStore, SettingStore,
                             # LFSStore, AccessTokenStore, WebhookStore,
                             # composite Store interface, Ping() + Close()
    errors.go                # ErrNotFound, ErrAlreadyExists, ErrRepoExist, etc.
    access.go                # AccessLevel type and constants

  app/
    backend.go               # Backend struct, constructor with domain ports
    repo.go                  # Repository management methods
    user.go                  # User management methods
    collab.go                # Collaborator methods
    auth.go                  # Password hashing, token validation
    hooks.go                 # Git hook handlers (pre-receive, update, etc.)
    cache.go                 # In-memory repo cache
    lfs.go                   # LFS helper operations

  infra/
    open.go                  # Factory: DSN -> sqlite.Open() or postgres.Open()

    sqlite/
      store.go               # Store struct, Open(), Close(), Ping(), migrations
      repo.go                # RepoStore implementation
      user.go                # UserStore implementation
      collab.go              # CollaboratorStore implementation
      settings.go            # SettingStore implementation
      lfs.go                 # LFSStore implementation
      access_token.go        # AccessTokenStore implementation
      webhook.go             # WebhookStore implementation
      errors.go              # isUniqueViolation() via string matching
      migrations/
        001_init.sql          # Goose format, embedded via //go:embed

    postgres/
      store.go               # Same structure as sqlite, pgx driver
      repo.go
      user.go
      ...
      errors.go              # isUniqueViolation() via pgconn.PgError
      migrations/
        001_init.sql

    httpapi/
      server.go              # HTTP server setup, route registration
      auth.go                # HTTP authentication (JWT, Basic, Token)
      git.go                 # Git HTTP transport handler
      git_lfs.go             # LFS protocol handler
      goget.go               # Go get meta tags
      health.go              # Health check endpoint
      context.go             # Request context middleware
      logging.go             # Request logging

    ssh/
      ssh.go                 # SSH server setup (wish)
      middleware.go          # Auth, context, command routing middleware
      cmd/
        cmd.go               # Command framework
        git.go               # Git protocol commands

    git/                     # Git operations (from top-level git/)
    webhook/                 # Webhook delivery system (from pkg/webhook/)
    lfs/                     # LFS client/endpoint (from pkg/lfs/)
    hooks/                   # Git hook generation (from pkg/hooks/)
    storage/                 # File storage abstraction (from pkg/storage/)
```

## Key Transformations

### 1. Domain types: interfaces to plain structs

Current `pkg/proto/` defines Repository and User as interfaces with getter
methods. This is inconsistent with Nexus/Hive where domain types are plain
structs.

Before (pkg/proto/repo.go):
```go
type Repository interface {
    ID() int64
    Name() string
    IsPrivate() bool
    Open() (*git.Repository, error)
    // ...
}
```

After (internal/domain/types.go):
```go
type Repo struct {
    ID          int64
    Name        string
    ProjectName string
    Description string
    IsPrivate   bool
    IsMirror    bool
    IsHidden    bool
    UserID      *int64     // nil if no owner
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

The `Open()` method moves to the application service or git adapter -- domain
types do not reference infrastructure.

The private `repo` struct in `pkg/backend/repo.go` (which wraps `models.Repo`
and implements the `proto.Repository` interface) is eliminated. Backend methods
return `*domain.Repo` directly.

### 2. Store ports: drop db.Handler parameter

Current store methods leak transaction management to callers:
```go
GetRepoByName(ctx context.Context, h db.Handler, name string) (models.Repo, error)
```

After:
```go
GetRepoByName(ctx context.Context, name string) (*domain.Repo, error)
```

Changes:
- Remove `db.Handler` from all ~50 method signatures
- Methods return domain types, not `models.*` types
- Adapters manage their own `*sql.DB` and transactions internally
- Composite Store interface includes `Ping(ctx) error` and `io.Closer`
- Compile-time check: `var _ domain.Store = (*Store)(nil)`

The granular getter/setter pattern (GetRepoIsPrivateByName,
SetRepoIsPrivateByName, etc.) simplifies to:
```go
type RepoStore interface {
    CreateRepo(ctx context.Context, repo *domain.Repo) error
    GetRepoByName(ctx context.Context, name string) (*domain.Repo, error)
    ListRepos(ctx context.Context) ([]*domain.Repo, error)
    ListUserRepos(ctx context.Context, userID int64) ([]*domain.Repo, error)
    UpdateRepo(ctx context.Context, repo *domain.Repo) error
    DeleteRepoByName(ctx context.Context, name string) error
}
```

This matches the Hive pattern -- pass the full domain struct for creates and
updates, the adapter persists changed fields.

### 3. Adapter-managed transactions

Currently Backend calls `d.db.TransactionContext()` and passes `tx` to store
methods. After the migration, adapters manage their own transactions:

```go
// internal/infra/sqlite/repo.go
func (s *Store) CreateRepo(ctx context.Context, repo *domain.Repo) error {
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO repos (name, project_name, description, private, hidden, mirror, user_id, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        repo.Name, repo.ProjectName, repo.Description,
        repo.IsPrivate, repo.IsHidden, repo.IsMirror,
        repo.UserID, repo.CreatedAt, repo.UpdatedAt,
    )
    if err != nil {
        if isUniqueViolation(err) {
            return fmt.Errorf("%w: repo %q", domain.ErrAlreadyExists, repo.Name)
        }
        return fmt.Errorf("create repo: %w", err)
    }
    return nil
}
```

For multi-step operations that need atomicity (e.g., delete repo + delete LFS
objects), the adapter provides a transaction-scoped method or the Store
interface includes a `Transaction(ctx, func(Store) error) error` method.

### 4. Backend depends only on domain ports

Before:
```go
type Backend struct {
    ctx     context.Context
    cfg     *config.Config     // full config struct
    db      *db.DB             // raw database connection
    store   store.Store
    logger  *log.Logger
    cache   *cache
    manager *task.Manager
}
```

After:
```go
type Backend struct {
    store    domain.Store
    repoDir  string            // specific config value, not full config
    dataDir  string            // specific config value
    logger   *log.Logger
    cache    *cache
}
```

Constructor takes specific values, not the full config:
```go
func New(store domain.Store, repoDir, dataDir string, logger *log.Logger) *Backend
```

The task manager (used for async imports) stays in Backend but doesn't need
`*db.DB` -- it uses the store interface.

### 5. Eliminate pkg/db/models/

Current `models.Repo`, `models.User`, etc. are persistence DTOs with `db:` tags.
These are eliminated. Adapters scan directly into domain structs (matching
Hive pattern):

```go
func (s *Store) GetRepoByName(ctx context.Context, name string) (*domain.Repo, error) {
    var r domain.Repo
    var userID sql.NullInt64
    err := s.db.QueryRowContext(ctx,
        `SELECT id, name, project_name, description, private, mirror, hidden, user_id, created_at, updated_at
         FROM repos WHERE name = ?`, name,
    ).Scan(&r.ID, &r.Name, &r.ProjectName, &r.Description,
        &r.IsPrivate, &r.IsMirror, &r.IsHidden, &userID,
        &r.CreatedAt, &r.UpdatedAt)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, fmt.Errorf("%w: repo %q", domain.ErrNotFound, name)
    }
    if err != nil {
        return nil, fmt.Errorf("get repo: %w", err)
    }
    if userID.Valid {
        r.UserID = &userID.Int64
    }
    return &r, nil
}
```

### 6. Database migration: sqlx to database/sql + Goose

Current: sqlx with Go-based migrations in `pkg/db/migrate/`.

After:
- Raw `database/sql` with `modernc.org/sqlite` and `pgx/v5` drivers
- Goose v3 with embedded SQL migrations (`//go:embed migrations/*.sql`)
- Separate migration directories per adapter with dialect-specific SQL
- Migrations run automatically in `Store.Open()`

### 7. Config: Viper + XDG

Current: `caarlos0/env` + custom YAML parsing + `SOFT_SERVE_*` env vars.

After:
- `spf13/viper` with `COMBINE_` env prefix
- XDG paths: `~/.config/combine/`, `~/.local/state/combine/`
- Dash-to-underscore env replacer: `--repo-dir` -> `COMBINE_REPO_DIR`
- Priority: flags > env > config file > defaults
- charmbracelet/log with JSON formatter to state dir

### 8. Cobra command structure

Matches Nexus/Hive pattern:

```go
// cmd/root.go
var rootCmd = &cobra.Command{
    Use:   "combine",
    Short: "A self-hostable Git forge",
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // Init dirs, load config, setup logging
    },
}

func init() {
    config.InitViper()
    rootCmd.AddCommand(daemon.NewCmd())
    rootCmd.AddCommand(admin.NewCmd())
    // ...
}
```

DI wiring lives in `cmd/daemon/daemon.go`:
```go
func run(...) error {
    store, err := infra.Open(dsn)
    defer store.Close()

    be := app.New(store, repoDir, dataDir, logger)

    // Create SSH + HTTP servers with be
    // Graceful shutdown on SIGINT/SIGTERM
}
```

## Dependency Direction

```
cmd/ (wiring)
  -> internal/app/ (application service)
       -> internal/domain/ (types, ports, errors)
  -> internal/infra/ (adapters)
       -> internal/domain/
  -> internal/config/
```

No circular dependencies. Domain imports nothing from infra or app.
Infra imports only domain. App imports only domain. Cmd wires everything.

## Migration from pkg/db/Handler pattern

The current codebase passes `db.Handler` (a `*db.Tx` or `*db.DB`) through
every store method so Backend can compose operations in a single transaction.
After migration:

1. Simple operations: adapter methods use their internal `*sql.DB` directly.
2. Multi-step atomic operations: Store interface includes:
   ```go
   Transaction(ctx context.Context, fn func(tx Store) error) error
   ```
   The adapter creates a transaction-scoped Store that uses the same `*sql.Tx`
   for all methods within `fn`. This keeps transaction management in the adapter
   while letting the application service compose operations atomically.

## What doesn't change

- Git operations logic (moved to `internal/infra/git/`, same code)
- Webhook delivery system (moved to `internal/infra/webhook/`, same code)
- LFS protocol handling (moved to `internal/infra/lfs/`, same code)
- HTTP handler logic (moved to `internal/infra/httpapi/`, same code)
- SSH server and Git transport (moved to `internal/infra/ssh/`, same code)
- Business rules in Backend methods (moved to `internal/app/`, same logic)

The logic is preserved; the boundaries and dependency directions change.

## New dependencies

Added:
- `github.com/spf13/viper` -- configuration management
- `github.com/pressly/goose/v3` -- database migrations
- `github.com/jackc/pgx/v5` -- PostgreSQL driver (replaces `lib/pq`)

Removed:
- `github.com/jmoiron/sqlx` -- replaced by raw `database/sql`
- `github.com/lib/pq` -- replaced by `pgx/v5`
- `github.com/caarlos0/env/v11` -- replaced by Viper
