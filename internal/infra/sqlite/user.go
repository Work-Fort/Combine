package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
	"golang.org/x/crypto/ssh"
)

func scanUser(row interface{ Scan(dest ...any) error }) (*domain.User, error) {
	var u domain.User
	var password sql.NullString
	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.Admin,
		&password,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if password.Valid {
		u.Password = password.String
	}
	return &u, nil
}

func scanUsers(rows *sql.Rows) ([]*domain.User, error) {
	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

const userColumns = `id, username, admin, password, created_at, updated_at`

func getUserByID(ctx context.Context, q querier, id int64) (*domain.User, error) {
	row := q.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: id %d", domain.ErrUserNotFound, id)
	}
	return u, err
}

func getUserByUsername(ctx context.Context, q querier, username string) (*domain.User, error) {
	username = strings.ToLower(username)
	row := q.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %q", domain.ErrUserNotFound, username)
	}
	return u, err
}

func getUserByPublicKey(ctx context.Context, q querier, pk ssh.PublicKey) (*domain.User, error) {
	ak := sshutils.MarshalAuthorizedKey(pk)
	row := q.QueryRowContext(ctx,
		`SELECT users.id, users.username, users.admin, users.password, users.created_at, users.updated_at
		 FROM users
		 INNER JOIN public_keys ON users.id = public_keys.user_id
		 WHERE public_keys.public_key = ?`, ak)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: public key", domain.ErrUserNotFound)
	}
	return u, err
}

func getUserByAccessToken(ctx context.Context, q querier, token string) (*domain.User, error) {
	row := q.QueryRowContext(ctx,
		`SELECT users.id, users.username, users.admin, users.password, users.created_at, users.updated_at
		 FROM users
		 INNER JOIN access_tokens ON users.id = access_tokens.user_id
		 WHERE access_tokens.token = ?`, token)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: access token", domain.ErrUserNotFound)
	}
	return u, err
}

func listUsers(ctx context.Context, q querier) ([]*domain.User, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+userColumns+` FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func createUser(ctx context.Context, q querier, username string, isAdmin bool, pks []ssh.PublicKey) error {
	username = strings.ToLower(username)
	row := q.QueryRowContext(ctx,
		`INSERT INTO users (username, admin, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) RETURNING id`,
		username, isAdmin)
	var userID int64
	if err := row.Scan(&userID); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %q", domain.ErrAlreadyExists, username)
		}
		return err
	}

	for _, pk := range pks {
		ak := sshutils.MarshalAuthorizedKey(pk)
		_, err := q.ExecContext(ctx,
			`INSERT INTO public_keys (user_id, public_key, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
			userID, ak)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteUserByUsername(ctx context.Context, q querier, username string) error {
	username = strings.ToLower(username)
	_, err := q.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	return err
}

func updateUser(ctx context.Context, q querier, user *domain.User) error {
	_, err := q.ExecContext(ctx,
		`UPDATE users SET username = ?, admin = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		user.Username, user.Admin, user.ID)
	return err
}

func addPublicKeyByUsername(ctx context.Context, q querier, username string, pk ssh.PublicKey) error {
	username = strings.ToLower(username)
	row := q.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, username)
	var userID int64
	if err := row.Scan(&userID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: %q", domain.ErrUserNotFound, username)
		}
		return err
	}
	ak := sshutils.MarshalAuthorizedKey(pk)
	_, err := q.ExecContext(ctx,
		`INSERT INTO public_keys (user_id, public_key, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		userID, ak)
	if err != nil && isUniqueViolation(err) {
		return fmt.Errorf("%w: public key", domain.ErrAlreadyExists)
	}
	return err
}

func removePublicKeyByUsername(ctx context.Context, q querier, username string, pk ssh.PublicKey) error {
	username = strings.ToLower(username)
	ak := sshutils.MarshalAuthorizedKey(pk)
	_, err := q.ExecContext(ctx,
		`DELETE FROM public_keys
		 WHERE user_id = (SELECT id FROM users WHERE username = ?)
		 AND public_key = ?`,
		username, ak)
	return err
}

func listPublicKeysByUserID(ctx context.Context, q querier, id int64) ([]ssh.PublicKey, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT public_key FROM public_keys WHERE user_id = ? ORDER BY id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pks []ssh.PublicKey
	for rows.Next() {
		var ak string
		if err := rows.Scan(&ak); err != nil {
			return nil, err
		}
		pk, _, err := sshutils.ParseAuthorizedKey(ak)
		if err != nil {
			return nil, err
		}
		pks = append(pks, pk)
	}
	return pks, rows.Err()
}

func listPublicKeysByUsername(ctx context.Context, q querier, username string) ([]ssh.PublicKey, error) {
	username = strings.ToLower(username)
	rows, err := q.QueryContext(ctx,
		`SELECT public_keys.public_key FROM public_keys
		 INNER JOIN users ON users.id = public_keys.user_id
		 WHERE users.username = ?
		 ORDER BY public_keys.id ASC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pks []ssh.PublicKey
	for rows.Next() {
		var ak string
		if err := rows.Scan(&ak); err != nil {
			return nil, err
		}
		pk, _, err := sshutils.ParseAuthorizedKey(ak)
		if err != nil {
			return nil, err
		}
		pks = append(pks, pk)
	}
	return pks, rows.Err()
}

func setUserPassword(ctx context.Context, q querier, userID int64, password string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE users SET password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		password, userID)
	return err
}

// Store methods.

func (s *Store) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	return getUserByID(ctx, s.q(), id)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	return getUserByUsername(ctx, s.q(), username)
}

func (s *Store) GetUserByPublicKey(ctx context.Context, pk ssh.PublicKey) (*domain.User, error) {
	return getUserByPublicKey(ctx, s.q(), pk)
}

func (s *Store) GetUserByAccessToken(ctx context.Context, token string) (*domain.User, error) {
	return getUserByAccessToken(ctx, s.q(), token)
}

func (s *Store) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return listUsers(ctx, s.q())
}

func (s *Store) CreateUser(ctx context.Context, username string, isAdmin bool, pks []ssh.PublicKey) error {
	return createUser(ctx, s.q(), username, isAdmin, pks)
}

func (s *Store) DeleteUserByUsername(ctx context.Context, username string) error {
	return deleteUserByUsername(ctx, s.q(), username)
}

func (s *Store) UpdateUser(ctx context.Context, user *domain.User) error {
	return updateUser(ctx, s.q(), user)
}

func (s *Store) AddPublicKeyByUsername(ctx context.Context, username string, pk ssh.PublicKey) error {
	return addPublicKeyByUsername(ctx, s.q(), username, pk)
}

func (s *Store) RemovePublicKeyByUsername(ctx context.Context, username string, pk ssh.PublicKey) error {
	return removePublicKeyByUsername(ctx, s.q(), username, pk)
}

func (s *Store) ListPublicKeysByUserID(ctx context.Context, id int64) ([]ssh.PublicKey, error) {
	return listPublicKeysByUserID(ctx, s.q(), id)
}

func (s *Store) ListPublicKeysByUsername(ctx context.Context, username string) ([]ssh.PublicKey, error) {
	return listPublicKeysByUsername(ctx, s.q(), username)
}

func (s *Store) SetUserPassword(ctx context.Context, userID int64, password string) error {
	return setUserPassword(ctx, s.q(), userID, password)
}

// txStore methods.

func (ts *txStore) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	return getUserByID(ctx, ts.q(), id)
}

func (ts *txStore) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	return getUserByUsername(ctx, ts.q(), username)
}

func (ts *txStore) GetUserByPublicKey(ctx context.Context, pk ssh.PublicKey) (*domain.User, error) {
	return getUserByPublicKey(ctx, ts.q(), pk)
}

func (ts *txStore) GetUserByAccessToken(ctx context.Context, token string) (*domain.User, error) {
	return getUserByAccessToken(ctx, ts.q(), token)
}

func (ts *txStore) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return listUsers(ctx, ts.q())
}

func (ts *txStore) CreateUser(ctx context.Context, username string, isAdmin bool, pks []ssh.PublicKey) error {
	return createUser(ctx, ts.q(), username, isAdmin, pks)
}

func (ts *txStore) DeleteUserByUsername(ctx context.Context, username string) error {
	return deleteUserByUsername(ctx, ts.q(), username)
}

func (ts *txStore) UpdateUser(ctx context.Context, user *domain.User) error {
	return updateUser(ctx, ts.q(), user)
}

func (ts *txStore) AddPublicKeyByUsername(ctx context.Context, username string, pk ssh.PublicKey) error {
	return addPublicKeyByUsername(ctx, ts.q(), username, pk)
}

func (ts *txStore) RemovePublicKeyByUsername(ctx context.Context, username string, pk ssh.PublicKey) error {
	return removePublicKeyByUsername(ctx, ts.q(), username, pk)
}

func (ts *txStore) ListPublicKeysByUserID(ctx context.Context, id int64) ([]ssh.PublicKey, error) {
	return listPublicKeysByUserID(ctx, ts.q(), id)
}

func (ts *txStore) ListPublicKeysByUsername(ctx context.Context, username string) ([]ssh.PublicKey, error) {
	return listPublicKeysByUsername(ctx, ts.q(), username)
}

func (ts *txStore) SetUserPassword(ctx context.Context, userID int64, password string) error {
	return setUserPassword(ctx, ts.q(), userID, password)
}
