package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
)

func scanAccessToken(row interface{ Scan(dest ...any) error }) (*domain.AccessToken, error) {
	var t domain.AccessToken
	var expiresAt sql.NullTime
	if err := row.Scan(
		&t.ID,
		&t.Token,
		&t.Name,
		&t.UserID,
		&expiresAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		t.ExpiresAt = &expiresAt.Time
	}
	return &t, nil
}

func scanAccessTokens(rows *sql.Rows) ([]*domain.AccessToken, error) {
	var tokens []*domain.AccessToken
	for rows.Next() {
		t, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

const accessTokenColumns = `id, token, name, user_id, expires_at, created_at, updated_at`

func getAccessToken(ctx context.Context, q querier, id int64) (*domain.AccessToken, error) {
	row := q.QueryRowContext(ctx, `SELECT `+accessTokenColumns+` FROM access_tokens WHERE id = ?`, id)
	t, err := scanAccessToken(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: id %d", domain.ErrTokenNotFound, id)
	}
	return t, err
}

func getAccessTokenByToken(ctx context.Context, q querier, token string) (*domain.AccessToken, error) {
	row := q.QueryRowContext(ctx, `SELECT `+accessTokenColumns+` FROM access_tokens WHERE token = ?`, token)
	t, err := scanAccessToken(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: token", domain.ErrTokenNotFound)
	}
	return t, err
}

func listAccessTokensByUserID(ctx context.Context, q querier, userID int64) ([]*domain.AccessToken, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+accessTokenColumns+` FROM access_tokens WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccessTokens(rows)
}

func createAccessToken(ctx context.Context, q querier, name string, userID int64, token string, expiresAt time.Time) (*domain.AccessToken, error) {
	var res sql.Result
	var err error
	if expiresAt.IsZero() {
		res, err = q.ExecContext(ctx,
			`INSERT INTO access_tokens (name, user_id, token, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
			name, userID, token)
	} else {
		res, err = q.ExecContext(ctx,
			`INSERT INTO access_tokens (name, user_id, token, expires_at, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			name, userID, token, expiresAt.UTC())
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: token", domain.ErrAlreadyExists)
		}
		return nil, err
	}

	id, _ := res.LastInsertId()
	return getAccessToken(ctx, q, id)
}

func deleteAccessToken(ctx context.Context, q querier, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM access_tokens WHERE id = ?`, id)
	return err
}

func deleteAccessTokenForUser(ctx context.Context, q querier, userID, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM access_tokens WHERE user_id = ? AND id = ?`, userID, id)
	return err
}

// Store methods.

func (s *Store) GetAccessToken(ctx context.Context, id int64) (*domain.AccessToken, error) {
	return getAccessToken(ctx, s.q(), id)
}

func (s *Store) GetAccessTokenByToken(ctx context.Context, token string) (*domain.AccessToken, error) {
	return getAccessTokenByToken(ctx, s.q(), token)
}

func (s *Store) ListAccessTokensByUserID(ctx context.Context, userID int64) ([]*domain.AccessToken, error) {
	return listAccessTokensByUserID(ctx, s.q(), userID)
}

func (s *Store) CreateAccessToken(ctx context.Context, name string, userID int64, token string, expiresAt time.Time) (*domain.AccessToken, error) {
	return createAccessToken(ctx, s.q(), name, userID, token, expiresAt)
}

func (s *Store) DeleteAccessToken(ctx context.Context, id int64) error {
	return deleteAccessToken(ctx, s.q(), id)
}

func (s *Store) DeleteAccessTokenForUser(ctx context.Context, userID, id int64) error {
	return deleteAccessTokenForUser(ctx, s.q(), userID, id)
}

// txStore methods.

func (ts *txStore) GetAccessToken(ctx context.Context, id int64) (*domain.AccessToken, error) {
	return getAccessToken(ctx, ts.q(), id)
}

func (ts *txStore) GetAccessTokenByToken(ctx context.Context, token string) (*domain.AccessToken, error) {
	return getAccessTokenByToken(ctx, ts.q(), token)
}

func (ts *txStore) ListAccessTokensByUserID(ctx context.Context, userID int64) ([]*domain.AccessToken, error) {
	return listAccessTokensByUserID(ctx, ts.q(), userID)
}

func (ts *txStore) CreateAccessToken(ctx context.Context, name string, userID int64, token string, expiresAt time.Time) (*domain.AccessToken, error) {
	return createAccessToken(ctx, ts.q(), name, userID, token, expiresAt)
}

func (ts *txStore) DeleteAccessToken(ctx context.Context, id int64) error {
	return deleteAccessToken(ctx, ts.q(), id)
}

func (ts *txStore) DeleteAccessTokenForUser(ctx context.Context, userID, id int64) error {
	return deleteAccessTokenForUser(ctx, ts.q(), userID, id)
}
