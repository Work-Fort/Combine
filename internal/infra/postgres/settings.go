package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/Work-Fort/Combine/internal/domain"
)

func getAnonAccess(ctx context.Context, q querier) (domain.AccessLevel, error) {
	var level string
	row := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'anon_access'`)
	if err := row.Scan(&level); err != nil {
		if err == sql.ErrNoRows {
			return domain.NoAccess, fmt.Errorf("%w: anon_access", domain.ErrNotFound)
		}
		return domain.NoAccess, err
	}
	return domain.ParseAccessLevel(level), nil
}

func setAnonAccess(ctx context.Context, q querier, level domain.AccessLevel) error {
	_, err := q.ExecContext(ctx,
		`UPDATE settings SET value = $1, updated_at = NOW() WHERE key = 'anon_access'`,
		level.String())
	return err
}

func getAllowKeylessAccess(ctx context.Context, q querier) (bool, error) {
	var value string
	row := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'allow_keyless'`)
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("%w: allow_keyless", domain.ErrNotFound)
		}
		return false, err
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse allow_keyless %q: %w", value, err)
	}
	return b, nil
}

func setAllowKeylessAccess(ctx context.Context, q querier, allow bool) error {
	_, err := q.ExecContext(ctx,
		`UPDATE settings SET value = $1, updated_at = NOW() WHERE key = 'allow_keyless'`,
		strconv.FormatBool(allow))
	return err
}

// Store methods.

func (s *Store) GetAnonAccess(ctx context.Context) (domain.AccessLevel, error) {
	return getAnonAccess(ctx, s.q())
}

func (s *Store) SetAnonAccess(ctx context.Context, level domain.AccessLevel) error {
	return setAnonAccess(ctx, s.q(), level)
}

func (s *Store) GetAllowKeylessAccess(ctx context.Context) (bool, error) {
	return getAllowKeylessAccess(ctx, s.q())
}

func (s *Store) SetAllowKeylessAccess(ctx context.Context, allow bool) error {
	return setAllowKeylessAccess(ctx, s.q(), allow)
}

// txStore methods.

func (ts *txStore) GetAnonAccess(ctx context.Context) (domain.AccessLevel, error) {
	return getAnonAccess(ctx, ts.q())
}

func (ts *txStore) SetAnonAccess(ctx context.Context, level domain.AccessLevel) error {
	return setAnonAccess(ctx, ts.q(), level)
}

func (ts *txStore) GetAllowKeylessAccess(ctx context.Context) (bool, error) {
	return getAllowKeylessAccess(ctx, ts.q())
}

func (ts *txStore) SetAllowKeylessAccess(ctx context.Context, allow bool) error {
	return setAllowKeylessAccess(ctx, ts.q(), allow)
}
