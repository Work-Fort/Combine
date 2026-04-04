# Hexagonal Architecture Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate Combine from Soft Serve's `pkg/` flat layout to WorkFort hexagonal architecture matching Nexus and Hive conventions.

**Architecture:** Bottom-up migration — build the new domain layer first, then adapters, then application service, then rewire commands. Each task produces a compilable codebase. Old packages are deleted only after all consumers are migrated.

**Tech Stack:** Go, database/sql, modernc.org/sqlite, pgx/v5, pressly/goose/v3, spf13/viper, spf13/cobra, charmbracelet/log

**Prerequisites:** The rebranding plan (2026-04-03-rebranding-plan.md) must be completed first.

---

## Phase A: Domain Layer

Build `internal/domain/` with zero infrastructure dependencies. This is the foundation everything else depends on.

### Task 1: Create domain types

**Files:**
- Create: `internal/domain/types.go`

**Step 1: Write domain type definitions**

Convert the interface-based types from `pkg/proto/` and the model structs from `pkg/db/models/` into plain structs. Convert `pkg/access/` types into the same file.

```go
package domain

import (
    "encoding"
    "errors"
    "time"

    "golang.org/x/crypto/ssh"
    "github.com/google/uuid"
)

// AccessLevel is the level of access allowed to a repo.
type AccessLevel int

const (
    NoAccess        AccessLevel = iota
    ReadOnlyAccess
    ReadWriteAccess
    AdminAccess
)

// String returns the string representation of the access level.
func (a AccessLevel) String() string {
    switch a {
    case NoAccess:
        return "no-access"
    case ReadOnlyAccess:
        return "read-only"
    case ReadWriteAccess:
        return "read-write"
    case AdminAccess:
        return "admin-access"
    default:
        return "unknown"
    }
}

// ParseAccessLevel parses an access level string.
func ParseAccessLevel(s string) AccessLevel {
    switch s {
    case "no-access":
        return NoAccess
    case "read-only":
        return ReadOnlyAccess
    case "read-write":
        return ReadWriteAccess
    case "admin-access":
        return AdminAccess
    default:
        return AccessLevel(-1)
    }
}

var (
    _ encoding.TextMarshaler   = AccessLevel(0)
    _ encoding.TextUnmarshaler = (*AccessLevel)(nil)
)

// ErrInvalidAccessLevel is returned when an invalid access level is provided.
var ErrInvalidAccessLevel = errors.New("invalid access level")

// UnmarshalText implements encoding.TextUnmarshaler.
func (a *AccessLevel) UnmarshalText(text []byte) error {
    l := ParseAccessLevel(string(text))
    if l < 0 {
        return ErrInvalidAccessLevel
    }
    *a = l
    return nil
}

// MarshalText implements encoding.TextMarshaler.
func (a AccessLevel) MarshalText() (text []byte, err error) {
    return []byte(a.String()), nil
}

// Repo is a Git repository.
type Repo struct {
    ID          int64
    Name        string
    ProjectName string
    Description string
    IsPrivate   bool
    IsMirror    bool
    IsHidden    bool
    UserID      *int64
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// RepoOptions are options for creating or importing a repository.
type RepoOptions struct {
    Private     bool
    Description string
    ProjectName string
    Mirror      bool
    Hidden      bool
    LFS         bool
    LFSEndpoint string
}

// User represents a user.
type User struct {
    ID        int64
    Username  string
    IsAdmin   bool
    Password  string // hashed
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Collab represents a repository collaborator.
type Collab struct {
    ID          int64
    RepoID      int64
    UserID      int64
    AccessLevel AccessLevel
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// PublicKey represents an SSH public key.
type PublicKey struct {
    ID        int64
    UserID    int64
    Key       string
    CreatedAt time.Time
    UpdatedAt time.Time
}

// AccessToken represents a user access token.
type AccessToken struct {
    ID        int64
    Name      string
    UserID    int64
    Token     string // hashed
    ExpiresAt *time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Settings holds server-level settings.
type Settings struct {
    AnonAccess        AccessLevel
    AllowKeylessAccess bool
}

// LFSObject is a Git LFS object.
type LFSObject struct {
    ID        int64
    Oid       string
    Size      int64
    RepoID    int64
    CreatedAt time.Time
    UpdatedAt time.Time
}

// LFSLock is a Git LFS lock.
type LFSLock struct {
    ID        int64
    Path      string
    UserID    int64
    RepoID    int64
    Refname   string
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Webhook is a repository webhook.
type Webhook struct {
    ID          int64
    RepoID      int64
    URL         string
    Secret      string
    ContentType int
    Active      bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// WebhookEvent is a webhook event subscription.
type WebhookEvent struct {
    ID        int64
    WebhookID int64
    Event     int
    CreatedAt time.Time
}

// WebhookDelivery is a webhook delivery record.
type WebhookDelivery struct {
    ID              uuid.UUID
    WebhookID       int64
    Event           int
    RequestURL      string
    RequestMethod   string
    RequestError    *string
    RequestHeaders  string
    RequestBody     string
    ResponseStatus  int
    ResponseHeaders string
    ResponseBody    string
    CreatedAt       time.Time
}
```

**Step 2: Verify it compiles**

```bash
go build ./internal/domain/
```

**Step 3: Commit**

```bash
git add internal/domain/types.go
git commit -m "feat: add domain types for hexagonal architecture"
```

---

### Task 2: Create domain errors

**Files:**
- Create: `internal/domain/errors.go`

**Step 1: Write domain error definitions**

Consolidate errors from `pkg/proto/errors.go` and `pkg/db/errors.go`:

```go
package domain

import "errors"

var (
    ErrNotFound            = errors.New("not found")
    ErrAlreadyExists       = errors.New("already exists")
    ErrUnauthorized        = errors.New("unauthorized")
    ErrRepoNotFound        = errors.New("repository not found")
    ErrRepoExist           = errors.New("repository already exists")
    ErrUserNotFound        = errors.New("user not found")
    ErrTokenNotFound       = errors.New("token not found")
    ErrTokenExpired        = errors.New("token expired")
    ErrCollaboratorNotFound = errors.New("collaborator not found")
    ErrCollaboratorExist   = errors.New("collaborator already exists")
    ErrFileNotFound        = errors.New("file not found")
)
```

**Step 2: Verify**

```bash
go build ./internal/domain/
```

**Step 3: Commit**

```bash
git add internal/domain/errors.go
git commit -m "feat: add domain errors"
```

---

### Task 3: Create domain port interfaces

**Files:**
- Create: `internal/domain/ports.go`

**Step 1: Write port interfaces**

Convert from `pkg/store/*.go` interfaces. Key changes:
- Remove `db.Handler` parameter from all methods
- Return domain types instead of `models.*` types
- Simplify granular getter/setter methods into CRUD with full struct
- Add `Transaction`, `Ping`, `Close` to composite Store
- Add compile-time interface check comment

```go
package domain

import (
    "context"
    "io"
    "time"

    "golang.org/x/crypto/ssh"
    "github.com/google/uuid"
)

// RepoStore persists repository metadata.
type RepoStore interface {
    CreateRepo(ctx context.Context, repo *Repo) error
    GetRepoByName(ctx context.Context, name string) (*Repo, error)
    ListRepos(ctx context.Context) ([]*Repo, error)
    ListUserRepos(ctx context.Context, userID int64) ([]*Repo, error)
    UpdateRepo(ctx context.Context, repo *Repo) error
    DeleteRepoByName(ctx context.Context, name string) error
}

// UserStore persists user accounts and SSH keys.
type UserStore interface {
    CreateUser(ctx context.Context, username string, isAdmin bool, pks []ssh.PublicKey) error
    GetUserByID(ctx context.Context, id int64) (*User, error)
    GetUserByUsername(ctx context.Context, username string) (*User, error)
    GetUserByPublicKey(ctx context.Context, pk ssh.PublicKey) (*User, error)
    GetUserByAccessToken(ctx context.Context, token string) (*User, error)
    ListUsers(ctx context.Context) ([]*User, error)
    DeleteUser(ctx context.Context, username string) error
    SetUsername(ctx context.Context, username string, newUsername string) error
    SetAdmin(ctx context.Context, username string, isAdmin bool) error
    SetPassword(ctx context.Context, userID int64, password string) error
    SetPasswordByUsername(ctx context.Context, username string, password string) error
    AddPublicKey(ctx context.Context, username string, pk ssh.PublicKey) error
    RemovePublicKey(ctx context.Context, username string, pk ssh.PublicKey) error
    ListPublicKeysByUserID(ctx context.Context, id int64) ([]ssh.PublicKey, error)
    ListPublicKeysByUsername(ctx context.Context, username string) ([]ssh.PublicKey, error)
}

// CollabStore persists repository collaborator relationships.
type CollabStore interface {
    GetCollab(ctx context.Context, username string, repo string) (*Collab, error)
    AddCollab(ctx context.Context, username string, repo string, level AccessLevel) error
    RemoveCollab(ctx context.Context, username string, repo string) error
    ListCollabsByRepo(ctx context.Context, repo string) ([]*Collab, error)
    ListCollabUsersByRepo(ctx context.Context, repo string) ([]*User, error)
}

// SettingStore persists server-level settings.
type SettingStore interface {
    GetAnonAccess(ctx context.Context) (AccessLevel, error)
    SetAnonAccess(ctx context.Context, level AccessLevel) error
    GetAllowKeylessAccess(ctx context.Context) (bool, error)
    SetAllowKeylessAccess(ctx context.Context, allow bool) error
}

// AccessTokenStore persists user access tokens.
type AccessTokenStore interface {
    GetAccessToken(ctx context.Context, id int64) (*AccessToken, error)
    GetAccessTokenByToken(ctx context.Context, token string) (*AccessToken, error)
    ListAccessTokensByUserID(ctx context.Context, userID int64) ([]*AccessToken, error)
    CreateAccessToken(ctx context.Context, name string, userID int64, token string, expiresAt time.Time) (*AccessToken, error)
    DeleteAccessToken(ctx context.Context, id int64) error
    DeleteAccessTokenForUser(ctx context.Context, userID int64, id int64) error
}

// LFSStore persists Git LFS objects and locks.
type LFSStore interface {
    CreateLFSObject(ctx context.Context, repoID int64, oid string, size int64) error
    GetLFSObjectByOid(ctx context.Context, repoID int64, oid string) (*LFSObject, error)
    ListLFSObjects(ctx context.Context, repoID int64) ([]*LFSObject, error)
    ListLFSObjectsByRepoName(ctx context.Context, name string) ([]*LFSObject, error)
    DeleteLFSObject(ctx context.Context, repoID int64, oid string) error

    CreateLFSLock(ctx context.Context, repoID int64, userID int64, path string, refname string) error
    GetLFSLockByID(ctx context.Context, id int64) (*LFSLock, error)
    GetLFSLockForPath(ctx context.Context, repoID int64, path string) (*LFSLock, error)
    ListLFSLocks(ctx context.Context, repoID int64, page int, limit int) ([]*LFSLock, error)
    ListLFSLocksWithCount(ctx context.Context, repoID int64, page int, limit int) ([]*LFSLock, int64, error)
    ListLFSLocksForUser(ctx context.Context, repoID int64, userID int64) ([]*LFSLock, error)
    GetLFSLockForUserPath(ctx context.Context, repoID int64, userID int64, path string) (*LFSLock, error)
    GetLFSLockForUserByID(ctx context.Context, repoID int64, userID int64, id int64) (*LFSLock, error)
    DeleteLFSLock(ctx context.Context, repoID int64, id int64) error
    DeleteLFSLockForUserByID(ctx context.Context, repoID int64, userID int64, id int64) error
}

// WebhookStore persists webhooks, events, and delivery records.
type WebhookStore interface {
    GetWebhookByID(ctx context.Context, repoID int64, id int64) (*Webhook, error)
    ListWebhooksByRepoID(ctx context.Context, repoID int64) ([]*Webhook, error)
    ListWebhooksByRepoIDAndEvents(ctx context.Context, repoID int64, events []int) ([]*Webhook, error)
    CreateWebhook(ctx context.Context, repoID int64, url string, secret string, contentType int, active bool) (int64, error)
    UpdateWebhook(ctx context.Context, repoID int64, id int64, url string, secret string, contentType int, active bool) error
    DeleteWebhook(ctx context.Context, id int64) error
    DeleteWebhookForRepo(ctx context.Context, repoID int64, id int64) error

    GetWebhookEventByID(ctx context.Context, id int64) (*WebhookEvent, error)
    ListWebhookEvents(ctx context.Context, webhookID int64) ([]*WebhookEvent, error)
    CreateWebhookEvents(ctx context.Context, webhookID int64, events []int) error
    DeleteWebhookEvents(ctx context.Context, ids []int64) error

    GetWebhookDeliveryByID(ctx context.Context, webhookID int64, id uuid.UUID) (*WebhookDelivery, error)
    ListWebhookDeliveries(ctx context.Context, webhookID int64) ([]*WebhookDelivery, error)
    ListWebhookDeliveriesSummary(ctx context.Context, webhookID int64) ([]*WebhookDelivery, error)
    CreateWebhookDelivery(ctx context.Context, id uuid.UUID, webhookID int64, event int, url string, method string, requestError error, requestHeaders string, requestBody string, responseStatus int, responseHeaders string, responseBody string) error
    DeleteWebhookDelivery(ctx context.Context, webhookID int64, id uuid.UUID) error
}

// Store is the composite port combining all sub-stores.
// Implementations: internal/infra/sqlite, internal/infra/postgres
type Store interface {
    RepoStore
    UserStore
    CollabStore
    SettingStore
    AccessTokenStore
    LFSStore
    WebhookStore

    // Transaction executes fn within a database transaction.
    // The Store passed to fn uses the same underlying transaction for all
    // method calls, providing atomicity for multi-step operations.
    //
    // NOTE: This is a Combine-specific deviation from the Nexus/Hive convention,
    // where neither service exposes Transaction on their Store interface. Combine
    // needs this because Backend performs cross-store atomic operations (e.g.,
    // DeleteRepository deletes the repo record + LFS objects in one transaction).
    // Nexus and Hive handle transactions entirely within the adapter layer because
    // their operations don't span multiple store sub-interfaces.
    Transaction(ctx context.Context, fn func(tx Store) error) error

    Ping(ctx context.Context) error
    io.Closer
}
```

**Step 2: Verify**

```bash
go build ./internal/domain/
```

**Step 3: Commit**

```bash
git add internal/domain/ports.go
git commit -m "feat: add domain port interfaces"
```

---

### Task 4: Create domain context helpers

**Files:**
- Create: `internal/domain/context.go`

**Step 1: Write context helpers**

Port from `pkg/proto/context.go` and `pkg/store/context.go` and
`pkg/access/context.go`, using struct types instead of interfaces:

```go
package domain

import "context"

type contextKey struct{ name string }

var (
    repoContextKey   = &contextKey{"repository"}
    userContextKey   = &contextKey{"user"}
    storeContextKey  = &contextKey{"store"}
    accessContextKey = &contextKey{"access"}
)

func RepoFromContext(ctx context.Context) *Repo {
    if r, ok := ctx.Value(repoContextKey).(*Repo); ok {
        return r
    }
    return nil
}

func WithRepoContext(ctx context.Context, r *Repo) context.Context {
    return context.WithValue(ctx, repoContextKey, r)
}

func UserFromContext(ctx context.Context) *User {
    if u, ok := ctx.Value(userContextKey).(*User); ok {
        return u
    }
    return nil
}

func WithUserContext(ctx context.Context, u *User) context.Context {
    return context.WithValue(ctx, userContextKey, u)
}

func StoreFromContext(ctx context.Context) Store {
    if s, ok := ctx.Value(storeContextKey).(Store); ok {
        return s
    }
    return nil
}

func WithStoreContext(ctx context.Context, s Store) context.Context {
    return context.WithValue(ctx, storeContextKey, s)
}

func AccessLevelFromContext(ctx context.Context) AccessLevel {
    if ac, ok := ctx.Value(accessContextKey).(AccessLevel); ok {
        return ac
    }
    return AccessLevel(-1)
}

func WithAccessLevelContext(ctx context.Context, ac AccessLevel) context.Context {
    return context.WithValue(ctx, accessContextKey, ac)
}
```

**Step 2: Verify**

```bash
go build ./internal/domain/
```

**Step 3: Commit**

```bash
git add internal/domain/context.go
git commit -m "feat: add domain context helpers"
```

---

## Phase B: Infrastructure — Config

### Task 5: Create Viper-based config

**Files:**
- Create: `internal/config/config.go`

**Step 1: Write the Viper + XDG config module**

Follow the Nexus/Hive pattern exactly:

```go
package config

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/pflag"
    "github.com/spf13/viper"
)

const (
    EnvPrefix      = "COMBINE"
    ConfigFileName = "config"
    ConfigType     = "yaml"
)

// Paths holds XDG-compliant directory paths.
type Paths struct {
    ConfigDir string
    StateDir  string
}

// GlobalPaths is initialized once during startup.
var GlobalPaths *Paths

func init() {
    GlobalPaths = GetPaths()
}

// GetPaths returns XDG-compliant config and state directories.
func GetPaths() *Paths {
    home, _ := os.UserHomeDir()

    configHome := os.Getenv("XDG_CONFIG_HOME")
    if configHome == "" {
        configHome = filepath.Join(home, ".config")
    }

    stateHome := os.Getenv("XDG_STATE_HOME")
    if stateHome == "" {
        stateHome = filepath.Join(home, ".local", "state")
    }

    return &Paths{
        ConfigDir: filepath.Join(configHome, "combine"),
        StateDir:  filepath.Join(stateHome, "combine"),
    }
}

// InitDirs ensures config and state directories exist.
func InitDirs() error {
    if err := os.MkdirAll(GlobalPaths.ConfigDir, 0o755); err != nil {
        return fmt.Errorf("create config dir: %w", err)
    }
    if err := os.MkdirAll(GlobalPaths.StateDir, 0o755); err != nil {
        return fmt.Errorf("create state dir: %w", err)
    }
    return nil
}

// InitViper sets defaults and config search paths.
func InitViper() {
    viper.SetDefault("name", "Combine")
    viper.SetDefault("data-path", "data")
    viper.SetDefault("log-level", "debug")

    // SSH defaults
    viper.SetDefault("ssh.enabled", true)
    viper.SetDefault("ssh.listen-addr", ":23231")
    viper.SetDefault("ssh.public-url", "ssh://localhost:23231")
    viper.SetDefault("ssh.key-path", filepath.Join("ssh", "combine_host_ed25519"))
    viper.SetDefault("ssh.client-key-path", filepath.Join("ssh", "combine_client_ed25519"))
    viper.SetDefault("ssh.max-timeout", 0)
    viper.SetDefault("ssh.idle-timeout", 600)

    // Git daemon defaults
    viper.SetDefault("git.enabled", true)
    viper.SetDefault("git.listen-addr", ":9418")
    viper.SetDefault("git.max-timeout", 0)
    viper.SetDefault("git.idle-timeout", 3)
    viper.SetDefault("git.max-connections", 32)

    // HTTP defaults
    viper.SetDefault("http.enabled", true)
    viper.SetDefault("http.listen-addr", ":23232")
    viper.SetDefault("http.public-url", "http://localhost:23232")

    // Stats defaults
    viper.SetDefault("stats.enabled", true)
    viper.SetDefault("stats.listen-addr", "localhost:23233")

    // DB defaults
    viper.SetDefault("db", "")

    // LFS defaults
    viper.SetDefault("lfs.enabled", true)
    viper.SetDefault("lfs.ssh-enabled", false)

    // Jobs
    viper.SetDefault("jobs.mirror-pull", "@every 10m")

    viper.SetConfigName(ConfigFileName)
    viper.SetConfigType(ConfigType)
    viper.AddConfigPath(GlobalPaths.ConfigDir)

    viper.SetEnvPrefix(EnvPrefix)
    viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
    viper.AutomaticEnv()
}

// LoadConfig reads the config file if it exists.
func LoadConfig() error {
    if err := viper.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); ok {
            return nil // config file is optional
        }
        return fmt.Errorf("read config: %w", err)
    }
    return nil
}

// BindFlags binds persistent flags to Viper keys.
func BindFlags(flags *pflag.FlagSet) error {
    flagsToBind := []string{"log-level", "data-path"}
    for _, name := range flagsToBind {
        if f := flags.Lookup(name); f != nil {
            if err := viper.BindPFlag(name, f); err != nil {
                return fmt.Errorf("bind flag %s: %w", name, err)
            }
        }
    }
    return nil
}
```

**Step 2: Add viper dependency**

```bash
go get github.com/spf13/viper@latest
```

**Step 3: Verify**

```bash
go build ./internal/config/
```

**Step 4: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add Viper + XDG config module"
```

---

## Phase C: Infrastructure — SQLite Store Adapter

Build the first store adapter. This is the largest single task — it implements
all domain port interfaces for SQLite.

### Task 6: Create SQLite store scaffold with migrations

**Files:**
- Create: `internal/infra/sqlite/store.go`
- Create: `internal/infra/sqlite/errors.go`
- Create: `internal/infra/sqlite/migrations/001_init.sql`

**Step 1: Write the migration**

Port the existing schema from `pkg/db/migrate/` into Goose SQL format.
Read the existing migration files in `pkg/db/migrate/` to get the exact
schema, then write as Goose SQL.

The migration must create all tables: repos, users, public_keys, collabs,
settings, access_tokens, lfs_objects, lfs_locks, webhooks, webhook_events,
webhook_deliveries.


**Step 2: Write store.go**

```go
package sqlite

import (
    "context"
    "database/sql"
    "embed"
    "fmt"

    "github.com/pressly/goose/v3"
    _ "modernc.org/sqlite"

    "github.com/Work-Fort/Combine/internal/domain"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

var _ domain.Store = (*Store)(nil)

// Store implements domain.Store for SQLite.
type Store struct {
    db *sql.DB
}

// Open creates a new SQLite store and runs migrations.
func Open(dsn string) (*Store, error) {
    if dsn == "" {
        dsn = ":memory:"
    }

    db, err := sql.Open("sqlite", dsn+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(wal)")
    if err != nil {
        return nil, fmt.Errorf("open sqlite: %w", err)
    }

    db.SetMaxOpenConns(1) // SQLite single-writer

    goose.SetBaseFS(embedMigrations)
    if err := goose.SetDialect("sqlite3"); err != nil {
        db.Close()
        return nil, fmt.Errorf("set dialect: %w", err)
    }

    if err := goose.Up(db, "migrations"); err != nil {
        db.Close()
        return nil, fmt.Errorf("run migrations: %w", err)
    }

    return &Store{db: db}, nil
}

func (s *Store) Ping(ctx context.Context) error {
    return s.db.PingContext(ctx)
}

func (s *Store) Close() error {
    return s.db.Close()
}

// Transaction executes fn within a database transaction.
func (s *Store) Transaction(ctx context.Context, fn func(tx domain.Store) error) error {
    sqlTx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin transaction: %w", err)
    }

    txStore := &txStore{tx: sqlTx}
    if err := fn(txStore); err != nil {
        if rerr := sqlTx.Rollback(); rerr != nil {
            return fmt.Errorf("rollback failed: %s: %w", err.Error(), rerr)
        }
        return err
    }

    if err := sqlTx.Commit(); err != nil {
        return fmt.Errorf("commit: %w", err)
    }
    return nil
}

// txStore wraps a sql.Tx and implements domain.Store for transactional use.
type txStore struct {
    tx *sql.Tx
}

func (t *txStore) Ping(ctx context.Context) error { return nil }
func (t *txStore) Close() error                   { return nil }
func (t *txStore) Transaction(ctx context.Context, fn func(tx domain.Store) error) error {
    // Already in a transaction, just execute directly
    return fn(t)
}
```

**Step 3: Write errors.go**

```go
package sqlite

import "strings"

func isUniqueViolation(err error) bool {
    return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
```

**Step 4: Add goose dependency**

```bash
go get github.com/pressly/goose/v3@latest
```

**Step 5: Verify**

```bash
go build ./internal/infra/sqlite/
```

**Step 6: Commit**

```bash
git add internal/infra/sqlite/ go.mod go.sum
git commit -m "feat: add SQLite store scaffold with Goose migrations"
```

---

### Task 7: Implement SQLite RepoStore

**Files:**
- Create: `internal/infra/sqlite/repo.go`

**Step 1: Write the RepoStore implementation**

Port queries from `pkg/store/database/repo.go`, but:
- Use raw `database/sql` instead of sqlx
- Scan into `*domain.Repo` directly (no `models.Repo`)
- No `db.Handler` parameter
- Use `s.db` for non-transactional, `t.tx` for transactional calls
- Wrap errors with `domain.ErrNotFound`, `domain.ErrAlreadyExists`

Both `Store` and `txStore` need to implement these methods. Use a shared
helper type or duplicate — the agent implementing this should read the existing
query implementations in `pkg/store/database/repo.go` and port them.

**Null handling notes:** Some domain struct fields use Go native types where the
database columns are nullable:
- `domain.Repo.UserID` is `*int64`, database is `user_id INTEGER` (nullable) — use `sql.NullInt64` for Scan, convert
- `domain.User.Password` is `string`, database is `password TEXT` (nullable via `sql.NullString`) — scan into `sql.NullString`, convert to empty string if null
- `domain.AccessToken.ExpiresAt` is `*time.Time`, database is `expires_at` (nullable) — use `sql.NullTime` for Scan
- `domain.WebhookDelivery.RequestError` is `*string`, database is `request_error` (nullable) — use `sql.NullString` for Scan

**Step 2: Write a basic test**

Create `internal/infra/sqlite/repo_test.go` with tests for CreateRepo,
GetRepoByName, ListRepos, UpdateRepo, DeleteRepoByName. Use an in-memory
SQLite database (empty DSN).

**Step 3: Run tests**

```bash
go test ./internal/infra/sqlite/ -run TestRepo -v
```

**Step 4: Commit**

```bash
git add internal/infra/sqlite/repo.go internal/infra/sqlite/repo_test.go
git commit -m "feat: implement SQLite RepoStore"
```

---

### Task 8: Implement SQLite UserStore

**Files:**
- Create: `internal/infra/sqlite/user.go`
- Create: `internal/infra/sqlite/user_test.go`

Same pattern as Task 7. Port from `pkg/store/database/user.go`. Key concern:
public key handling — the existing code stores SSH public keys as authorized_key
format strings. Keep the same storage format.

**Step 1: Write implementation**
**Step 2: Write tests**
**Step 3: Run tests and verify**
**Step 4: Commit**

```bash
git commit -m "feat: implement SQLite UserStore"
```

---

### Task 9: Implement remaining SQLite sub-stores

**Files:**
- Create: `internal/infra/sqlite/collab.go`
- Create: `internal/infra/sqlite/settings.go`
- Create: `internal/infra/sqlite/access_token.go`
- Create: `internal/infra/sqlite/lfs.go`
- Create: `internal/infra/sqlite/webhook.go`
- Create: tests for each

Port from `pkg/store/database/` equivalents. Same pattern as Tasks 7-8.

**Step 1: Implement each sub-store one at a time**
**Step 2: Write tests for each**
**Step 3: Run all store tests**

```bash
go test ./internal/infra/sqlite/ -v
```

**Step 4: Commit per sub-store or as a batch**

```bash
git commit -m "feat: implement remaining SQLite sub-stores"
```

---

### Task 10: Create PostgreSQL store adapter

**Files:**
- Create: `internal/infra/postgres/store.go`
- Create: `internal/infra/postgres/errors.go`
- Create: `internal/infra/postgres/migrations/001_init.sql`
- Create: `internal/infra/postgres/repo.go` (and all other sub-stores)

Same port interfaces as SQLite but with PostgreSQL-specific SQL:
- `$1, $2` placeholders instead of `?`
- `TIMESTAMPTZ` instead of `DATETIME`
- `pgx/v5` driver instead of `modernc.org/sqlite`
- `pgconn.PgError` for unique violation detection

**Step 1: Add pgx dependency**

```bash
go get github.com/jackc/pgx/v5@latest
```

**Step 2: Write store.go, errors.go, migrations, and all sub-stores**

Mirror the SQLite adapter exactly, changing only SQL syntax and driver calls.

**Step 3: Commit**

```bash
git commit -m "feat: add PostgreSQL store adapter"
```

---

### Task 11: Create infra.Open() factory

**Files:**
- Create: `internal/infra/open.go`

**Step 1: Write the factory**

```go
package infra

import (
    "strings"

    "github.com/Work-Fort/Combine/internal/domain"
    "github.com/Work-Fort/Combine/internal/infra/postgres"
    "github.com/Work-Fort/Combine/internal/infra/sqlite"
)

// Open auto-detects database backend from DSN.
// postgres:// or postgresql:// -> PostgreSQL, otherwise -> SQLite.
func Open(dsn string) (domain.Store, error) {
    if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
        return postgres.Open(dsn)
    }
    return sqlite.Open(dsn)
}
```

**Step 2: Verify**

```bash
go build ./internal/infra/
```

**Step 3: Commit**

```bash
git add internal/infra/open.go
git commit -m "feat: add infra.Open() DSN-based store factory"
```

---

## Phase D: Infrastructure — Move Existing Adapters

Move the existing HTTP, SSH, Git, webhook, LFS, hooks, and storage packages
into `internal/infra/`. These packages keep their existing logic but get new
import paths. They will be updated to use domain types in Phase E.

### Task 12: Move infrastructure packages

**Step 1: Move packages with git mv**

```bash
mkdir -p internal/infra
git mv pkg/web internal/infra/httpapi
git mv pkg/ssh internal/infra/ssh
git mv git internal/infra/git
git mv pkg/git internal/infra/gitutil
git mv pkg/webhook internal/infra/webhook
git mv pkg/lfs internal/infra/lfs
git mv pkg/hooks internal/infra/hooks
git mv pkg/storage internal/infra/storage
git mv pkg/sshutils internal/infra/sshutils
git mv pkg/stats internal/infra/stats
git mv pkg/cron internal/infra/cron
git mv pkg/sync internal/infra/sync
git mv pkg/utils internal/infra/utils
git mv pkg/task internal/infra/task
git mv pkg/jobs internal/infra/jobs
git mv pkg/jwk internal/infra/jwk
git mv pkg/log internal/infra/log
git mv pkg/version internal/infra/version
git mv pkg/test internal/infra/testutil
```

Note: `pkg/git/` (LFS auth, git service code) is distinct from top-level `git/`
(low-level git operations). Both are moved but to different targets:
- `git/` -> `internal/infra/git/`
- `pkg/git/` -> `internal/infra/gitutil/`

**Step 2: Update all import paths**

Find and replace across the entire codebase:
- `github.com/Work-Fort/Combine/pkg/web` -> `github.com/Work-Fort/Combine/internal/infra/httpapi`
- `github.com/Work-Fort/Combine/pkg/ssh` -> `github.com/Work-Fort/Combine/internal/infra/ssh`
- `github.com/Work-Fort/Combine/git` -> `github.com/Work-Fort/Combine/internal/infra/git`
- `github.com/Work-Fort/Combine/pkg/git` -> `github.com/Work-Fort/Combine/internal/infra/gitutil`
- `github.com/Work-Fort/Combine/pkg/webhook` -> `github.com/Work-Fort/Combine/internal/infra/webhook`
- `github.com/Work-Fort/Combine/pkg/lfs` -> `github.com/Work-Fort/Combine/internal/infra/lfs`
- `github.com/Work-Fort/Combine/pkg/hooks` -> `github.com/Work-Fort/Combine/internal/infra/hooks`
- `github.com/Work-Fort/Combine/pkg/storage` -> `github.com/Work-Fort/Combine/internal/infra/storage`
- `github.com/Work-Fort/Combine/pkg/sshutils` -> `github.com/Work-Fort/Combine/internal/infra/sshutils`
- `github.com/Work-Fort/Combine/pkg/stats` -> `github.com/Work-Fort/Combine/internal/infra/stats`
- `github.com/Work-Fort/Combine/pkg/cron` -> `github.com/Work-Fort/Combine/internal/infra/cron`
- `github.com/Work-Fort/Combine/pkg/sync` -> `github.com/Work-Fort/Combine/internal/infra/sync`
- `github.com/Work-Fort/Combine/pkg/utils` -> `github.com/Work-Fort/Combine/internal/infra/utils`
- `github.com/Work-Fort/Combine/pkg/task` -> `github.com/Work-Fort/Combine/internal/infra/task`
- `github.com/Work-Fort/Combine/pkg/jobs` -> `github.com/Work-Fort/Combine/internal/infra/jobs`
- `github.com/Work-Fort/Combine/pkg/jwk` -> `github.com/Work-Fort/Combine/internal/infra/jwk`
- `github.com/Work-Fort/Combine/pkg/log` -> `github.com/Work-Fort/Combine/internal/infra/log`
- `github.com/Work-Fort/Combine/pkg/version` -> `github.com/Work-Fort/Combine/internal/infra/version`
- `github.com/Work-Fort/Combine/pkg/test` -> `github.com/Work-Fort/Combine/internal/infra/testutil`

**Step 3: Verify**

```bash
go build ./...
```

This will likely fail because some packages still import `pkg/backend`,
`pkg/store`, `pkg/proto`, `pkg/db`, `pkg/config`, `pkg/access`, etc.
That's expected — those are migrated in Phase E.

**Step 4: Commit**

```bash
git add -A && git commit -m "refactor: move infrastructure packages to internal/infra/"
```

---

## Phase E: Application Service and Adapter Migration

This is the most complex phase. We need to:
1. Move Backend to `internal/app/`
2. Update Backend to use domain types instead of proto interfaces
3. Update Backend to use `domain.Store` instead of `store.Store` + `db.DB`
4. Update HTTP and SSH adapters to use the new types
5. Delete old packages

### Task 13: Move backend to internal/app/

**Step 1: Move the package**

```bash
git mv pkg/backend internal/app
```

**Step 2: Update import paths**

Replace `github.com/Work-Fort/Combine/pkg/backend` with
`github.com/Work-Fort/Combine/internal/app` across the codebase.

**Step 3: Commit (may not compile yet)**

```bash
git add -A && git commit -m "refactor: move backend to internal/app"
```

---

### Task 14: Refactor Backend to use domain types

This is the core refactor. The Backend struct changes to depend only on
`domain.Store`, and all methods are updated to work with `*domain.Repo`,
`*domain.User`, etc. instead of `proto.Repository`, `proto.User`.

**Files:**
- Modify: `internal/app/backend.go`
- Modify: `internal/app/repo.go`
- Modify: `internal/app/user.go`
- Modify: `internal/app/collab.go`
- Modify: `internal/app/auth.go`
- Modify: `internal/app/hooks.go`
- Modify: all other files in `internal/app/`

**Key changes:**

1. Backend struct drops `*db.DB` and `*config.Config`, but retains the
   specific config values it actually uses (identified by feasibility assessment):
   ```go
   // BackendConfig holds the specific config values Backend needs.
   // This avoids importing the full config package into the app layer.
   type BackendConfig struct {
       RepoDir         string           // filepath.Join(dataDir, "repos")
       DataDir         string           // base data directory
       AdminKeys       []ssh.PublicKey   // initial admin public keys
       SSHClientKeyPath string          // for git clone over SSH (ImportRepository)
       SSHKnownHostsPath string         // for git clone over SSH (ImportRepository)
   }

   type Backend struct {
       store   domain.Store
       cfg     BackendConfig
       logger  *log.Logger
       cache   *cache
   }
   ```

2. Constructor takes the config struct:
   ```go
   func New(store domain.Store, cfg BackendConfig, logger *log.Logger) *Backend
   ```

3. All methods replace `d.db.TransactionContext(ctx, func(tx *db.Tx)` with
   `d.store.Transaction(ctx, func(tx domain.Store)`:
   ```go
   // Before
   if err := d.db.TransactionContext(ctx, func(tx *db.Tx) error {
       if err := d.store.CreateRepo(ctx, tx, name, ...); err != nil {
   // After
   if err := d.store.Transaction(ctx, func(tx domain.Store) error {
       if err := tx.CreateRepo(ctx, repo); err != nil {
   ```

4. Return types change from `proto.Repository` to `*domain.Repo`:
   ```go
   // Before
   func (d *Backend) Repository(ctx context.Context, name string) (proto.Repository, error)
   // After
   func (d *Backend) Repository(ctx context.Context, name string) (*domain.Repo, error)
   ```

5. The private `repo` struct that wraps `models.Repo` and implements
   `proto.Repository` is eliminated entirely.

6. Git operations (`Open()`) move to a helper that takes a repo path:
   ```go
   func (d *Backend) openRepo(name string) (*git.Repository, error) {
       return git.Open(d.repoPath(name))
   }
   ```

7. Refactor `StoreRepoMissingLFSObjects`: This is currently a package-level
   function in `pkg/backend/lfs.go` with signature:
   ```go
   func StoreRepoMissingLFSObjects(ctx context.Context, repo proto.Repository, dbx *db.DB, store store.Store, lfsClient lfs.Client) error
   ```
   It directly takes `*db.DB` and `store.Store` and calls `dbx.TransactionContext`.
   It is called from `ImportRepository` (repo.go) and `pkg/jobs/mirror.go`.
   Refactor it to a Backend method that uses `domain.Store.Transaction`:
   ```go
   func (d *Backend) StoreRepoMissingLFSObjects(ctx context.Context, repoName string, lfsClient lfs.Client) error
   ```
   The `pkg/jobs/mirror.go` call site will need to receive Backend and call the
   method instead of the package-level function.

**This task requires reading every file in internal/app/ and making targeted
changes.** The implementing agent should work through one file at a time,
starting with backend.go, then repo.go, then user.go, etc.

**Step 1: Refactor backend.go constructor**
**Step 2: Refactor repo.go** (largest file — CreateRepository, DeleteRepository, etc.)
**Step 3: Refactor user.go**
**Step 4: Refactor collab.go**
**Step 5: Refactor auth.go**
**Step 6: Refactor hooks.go**
**Step 7: Refactor lfs.go** (StoreRepoMissingLFSObjects → Backend method)
**Step 8: Refactor remaining files**
**Step 8: Verify it compiles** (may still fail due to adapter references)

```bash
go build ./internal/app/
```

**Step 9: Commit**

```bash
git add -A && git commit -m "refactor: backend uses domain types and domain.Store"
```

---

### Task 15: Update HTTP adapter to use domain types

**Files:**
- Modify: all files in `internal/infra/httpapi/`

**Key changes:**
- Replace `proto.Repository` with `*domain.Repo`
- Replace `proto.User` with `*domain.User`
- Replace `proto.UserFromContext` with `domain.UserFromContext`
- Replace `store.FromContext` with `domain.StoreFromContext`
- Replace `access.FromContext` with `domain.AccessLevelFromContext`
- Replace `backend.FromContext` with the new Backend context accessor
- Remove `db.FromContext` usage — the HTTP layer no longer touches the DB
- Update import paths

**Step 1: Work through each file, updating type references**
**Step 2: Update context middleware to inject new types**
**Step 3: Verify compilation**

```bash
go build ./internal/infra/httpapi/
```

**Step 4: Commit**

```bash
git commit -m "refactor: HTTP adapter uses domain types"
```

---

### Task 16: Update SSH adapter to use domain types

**Files:**
- Modify: all files in `internal/infra/ssh/`

Same pattern as Task 15.

**Step 1: Update type references and imports**
**Step 2: Verify compilation**
**Step 3: Commit**

```bash
git commit -m "refactor: SSH adapter uses domain types"
```

---

### Task 17: Update remaining infrastructure packages

**Files:**
- Modify: `internal/infra/webhook/` — use domain types
- Modify: `internal/infra/lfs/` — use domain types
- Modify: `internal/infra/hooks/` — use domain types (also takes `*config.Config`, needs specific values instead)
- Modify: `internal/infra/gitutil/` — use domain types (depends on config, db, models, proto, store, lfs, storage, jwk)
- Modify: `internal/infra/jobs/` — use domain types (depends on backend, db, store, config; mirror.go calls StoreRepoMissingLFSObjects)
- Modify: `internal/infra/task/` — if it references old types

These packages reference `proto.*`, `store.*`, `db.*`, and `config.*` types.
Update them to use `domain.*` types and the new config approach.

Key concerns:
- `internal/infra/gitutil/` has heavy deps on the old type system — this is
  the most complex package to update after Backend itself
- `internal/infra/jobs/mirror.go` calls the old `StoreRepoMissingLFSObjects`
  package-level function — update to call Backend method instead
- `internal/infra/hooks/gen.go` takes `*config.Config` — pass specific values
  (binary path, data path) instead of the full config struct

**Step 1: Update each package**
**Step 2: Verify full build**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git commit -m "refactor: remaining infra packages use domain types"
```

---

## Phase F: Command Layer

### Task 18: Rewrite command layer with Nexus/Hive patterns

**Files:**
- Create: `cmd/root.go`
- Create: `cmd/daemon/daemon.go`
- Modify: `cmd/combine/main.go` (simplify to just call cmd.Execute())
- Move: `cmd/combine/admin/` -> `cmd/admin/`
- Move: `cmd/combine/hook/` -> `cmd/hook/`
- Delete: `cmd/combine/serve/` (replaced by cmd/daemon/)
- Delete: `cmd/cmd.go` (InitBackendContext replaced by daemon DI)

**Step 1: Write cmd/root.go**

Follow the Nexus/Hive pattern with PersistentPreRunE for config loading and
logging setup.

**Step 2: Write cmd/daemon/daemon.go**

This replaces `cmd/combine/serve/`. It does all DI wiring:
```go
func run(...) error {
    store, err := infra.Open(dsn)
    defer store.Close()

    beCfg := app.BackendConfig{
        RepoDir:           filepath.Join(dataDir, "repos"),
        DataDir:           dataDir,
        AdminKeys:         adminKeys,
        SSHClientKeyPath:  viper.GetString("ssh.client-key-path"),
        SSHKnownHostsPath: filepath.Join(dataDir, "ssh", "known_hosts"),
    }
    be := app.New(store, beCfg, logger)

    // Start SSH + HTTP servers
    // Graceful shutdown
}
```

**Step 3: Simplify main.go**

```go
func main() {
    cmd.Execute()
}
```

**Step 4: Move admin and hook commands**
**Step 5: Verify full build and run**

```bash
go build ./cmd/combine/
./combine --help
```

**Step 6: Commit**

```bash
git commit -m "refactor: command layer with Nexus/Hive DI pattern"
```

---

## Phase G: Cleanup

### Task 19: Delete old packages

**Files:**
- Delete: `pkg/` (entire directory)
- Delete: `cmd/cmd.go` (if not already removed)

**Step 1: Verify nothing imports the old packages**

```bash
grep -r '"github.com/Work-Fort/Combine/pkg/' --include="*.go" .
```

Should return zero results.

**Step 2: Delete**

```bash
rm -rf pkg/
```

**Step 3: Verify full build and tests**

```bash
go build ./...
go test ./...
```

**Step 4: Run go mod tidy**

```bash
go mod tidy
```

This should remove `jmoiron/sqlx`, `lib/pq`, `caarlos0/env` and other
unused dependencies.

**Step 5: Commit**

```bash
git add -A && git commit -m "refactor: remove legacy pkg/ directory"
```

---

### Task 20: Final verification

**Step 1: Verify directory structure matches target**

```bash
find internal/ -type f -name "*.go" | head -50
ls cmd/
```

Should match the target structure from the design doc.

**Step 2: Verify dependency directions**

```bash
# Domain should import nothing from infra or app
grep -r '"github.com/Work-Fort/Combine/internal/infra' internal/domain/
grep -r '"github.com/Work-Fort/Combine/internal/app' internal/domain/
```

Both should return zero results.

**Step 3: Full build and test**

```bash
go build ./...
go test ./...
```

**Step 4: Verify the binary runs**

```bash
go build -o combine ./cmd/combine/
./combine --help
./combine daemon --help
```

**Step 5: Commit any fixes**

```bash
git add -A && git commit -m "chore: final cleanup after hexagonal migration"
```
