package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/pressly/goose/v3"

	// SQLite driver.
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// querier is a common interface satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Compile-time checks.
var (
	_ domain.Store = (*Store)(nil)
	_ domain.Store = (*txStore)(nil)
)

// Store is the SQLite implementation of domain.Store.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite database at the given DSN and runs migrations.
// Pass an empty string for an in-memory database.
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = ":memory:"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// SQLite pragmas for correctness and performance.
	for _, pragma := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite pragma %q: %w", pragma, err)
		}
	}

	db.SetMaxOpenConns(1)

	// Run Goose migrations.
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	return &Store{db: db}, nil
}

// Ping checks that the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Transaction runs fn inside a database transaction.
func (s *Store) Transaction(ctx context.Context, fn func(tx domain.Store) error) error {
	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	ts := &txStore{tx: sqlTx}
	if err := fn(ts); err != nil {
		_ = sqlTx.Rollback()
		return err
	}

	return sqlTx.Commit()
}

// q returns the querier for non-transactional use.
func (s *Store) q() querier { return s.db }

// -----------------------------------------------------------------------
// txStore wraps an active transaction and implements domain.Store.
// -----------------------------------------------------------------------

type txStore struct {
	tx *sql.Tx
}

// Transaction on a txStore is a no-op nesting — it just calls fn with itself.
func (ts *txStore) Transaction(_ context.Context, fn func(tx domain.Store) error) error {
	return fn(ts)
}

// Ping is a no-op inside a transaction.
func (ts *txStore) Ping(_ context.Context) error { return nil }

// Close is a no-op inside a transaction.
func (ts *txStore) Close() error { return nil }

// q returns the querier for transactional use.
func (ts *txStore) q() querier { return ts.tx }
