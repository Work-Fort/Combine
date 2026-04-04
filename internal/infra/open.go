package infra

import (
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/postgres"
	"github.com/Work-Fort/Combine/internal/infra/sqlite"
)

// Open returns a domain.Store backed by the database identified by dsn.
// PostgreSQL DSNs (postgres:// or postgresql://) use the Postgres adapter;
// everything else falls through to SQLite.
func Open(dsn string) (domain.Store, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return postgres.Open(dsn)
	}
	return sqlite.Open(dsn)
}
