package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/ssh"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
)

func scanIdentity(row interface{ Scan(dest ...any) error }) (*domain.Identity, error) {
	var id domain.Identity
	if err := row.Scan(
		&id.ID,
		&id.Username,
		&id.DisplayName,
		&id.Type,
		&id.IsAdmin,
		&id.CreatedAt,
		&id.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &id, nil
}

func scanIdentities(rows *sql.Rows) ([]*domain.Identity, error) {
	var ids []*domain.Identity
	for rows.Next() {
		id, err := scanIdentity(rows)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

const identityColumns = `id, username, display_name, type, is_admin, created_at, updated_at`

func upsertIdentity(ctx context.Context, q querier, id, username, displayName, identityType string) (*domain.Identity, error) {
	row := q.QueryRowContext(ctx,
		`INSERT INTO identities (id, username, display_name, type, updated_at)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(id) DO UPDATE SET
		   username=excluded.username,
		   display_name=excluded.display_name,
		   type=excluded.type,
		   updated_at=datetime('now')
		 RETURNING `+identityColumns,
		id, username, displayName, identityType)
	return scanIdentity(row)
}

func getIdentityByID(ctx context.Context, q querier, id string) (*domain.Identity, error) {
	row := q.QueryRowContext(ctx, `SELECT `+identityColumns+` FROM identities WHERE id = ?`, id)
	ident, err := scanIdentity(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %q", domain.ErrIdentityNotFound, id)
	}
	return ident, err
}

func getIdentityByUsername(ctx context.Context, q querier, username string) (*domain.Identity, error) {
	row := q.QueryRowContext(ctx, `SELECT `+identityColumns+` FROM identities WHERE username = ?`, username)
	ident, err := scanIdentity(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %q", domain.ErrIdentityNotFound, username)
	}
	return ident, err
}

func getIdentityByPublicKey(ctx context.Context, q querier, pk ssh.PublicKey) (*domain.Identity, error) {
	ak := sshutils.MarshalAuthorizedKey(pk)
	row := q.QueryRowContext(ctx,
		`SELECT identities.id, identities.username, identities.display_name, identities.type,
		        identities.is_admin, identities.created_at, identities.updated_at
		 FROM identities
		 INNER JOIN identity_public_keys ON identities.id = identity_public_keys.identity_id
		 WHERE identity_public_keys.public_key = ?`, ak)
	ident, err := scanIdentity(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: public key", domain.ErrIdentityNotFound)
	}
	return ident, err
}

func listIdentities(ctx context.Context, q querier) ([]*domain.Identity, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+identityColumns+` FROM identities`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIdentities(rows)
}

func setIdentityAdmin(ctx context.Context, q querier, id string, isAdmin bool) error {
	_, err := q.ExecContext(ctx,
		`UPDATE identities SET is_admin = ?, updated_at = datetime('now') WHERE id = ?`,
		isAdmin, id)
	return err
}

func addIdentityPublicKey(ctx context.Context, q querier, identityID string, pk ssh.PublicKey) error {
	ak := sshutils.MarshalAuthorizedKey(pk)
	_, err := q.ExecContext(ctx,
		`INSERT INTO identity_public_keys (identity_id, public_key, updated_at) VALUES (?, ?, datetime('now'))`,
		identityID, ak)
	if err != nil && isUniqueViolation(err) {
		return fmt.Errorf("%w: public key", domain.ErrAlreadyExists)
	}
	return err
}

func removeIdentityPublicKey(ctx context.Context, q querier, identityID string, keyID int64) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM identity_public_keys WHERE identity_id = ? AND id = ?`,
		identityID, keyID)
	return err
}

func listIdentityPublicKeys(ctx context.Context, q querier, identityID string) ([]*domain.PublicKey, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, identity_id, public_key, created_at, updated_at
		 FROM identity_public_keys WHERE identity_id = ? ORDER BY id ASC`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*domain.PublicKey
	for rows.Next() {
		var k domain.PublicKey
		var identID string
		if err := rows.Scan(&k.ID, &identID, &k.PublicKey, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

// Store methods.

func (s *Store) UpsertIdentity(ctx context.Context, id, username, displayName, identityType string) (*domain.Identity, error) {
	return upsertIdentity(ctx, s.q(), id, username, displayName, identityType)
}

func (s *Store) GetIdentityByID(ctx context.Context, id string) (*domain.Identity, error) {
	return getIdentityByID(ctx, s.q(), id)
}

func (s *Store) GetIdentityByUsername(ctx context.Context, username string) (*domain.Identity, error) {
	return getIdentityByUsername(ctx, s.q(), username)
}

func (s *Store) GetIdentityByPublicKey(ctx context.Context, pk ssh.PublicKey) (*domain.Identity, error) {
	return getIdentityByPublicKey(ctx, s.q(), pk)
}

func (s *Store) ListIdentities(ctx context.Context) ([]*domain.Identity, error) {
	return listIdentities(ctx, s.q())
}

func (s *Store) SetIdentityAdmin(ctx context.Context, id string, isAdmin bool) error {
	return setIdentityAdmin(ctx, s.q(), id, isAdmin)
}

func (s *Store) AddIdentityPublicKey(ctx context.Context, identityID string, pk ssh.PublicKey) error {
	return addIdentityPublicKey(ctx, s.q(), identityID, pk)
}

func (s *Store) RemoveIdentityPublicKey(ctx context.Context, identityID string, keyID int64) error {
	return removeIdentityPublicKey(ctx, s.q(), identityID, keyID)
}

func (s *Store) ListIdentityPublicKeys(ctx context.Context, identityID string) ([]*domain.PublicKey, error) {
	return listIdentityPublicKeys(ctx, s.q(), identityID)
}

// txStore methods.

func (ts *txStore) UpsertIdentity(ctx context.Context, id, username, displayName, identityType string) (*domain.Identity, error) {
	return upsertIdentity(ctx, ts.q(), id, username, displayName, identityType)
}

func (ts *txStore) GetIdentityByID(ctx context.Context, id string) (*domain.Identity, error) {
	return getIdentityByID(ctx, ts.q(), id)
}

func (ts *txStore) GetIdentityByUsername(ctx context.Context, username string) (*domain.Identity, error) {
	return getIdentityByUsername(ctx, ts.q(), username)
}

func (ts *txStore) GetIdentityByPublicKey(ctx context.Context, pk ssh.PublicKey) (*domain.Identity, error) {
	return getIdentityByPublicKey(ctx, ts.q(), pk)
}

func (ts *txStore) ListIdentities(ctx context.Context) ([]*domain.Identity, error) {
	return listIdentities(ctx, ts.q())
}

func (ts *txStore) SetIdentityAdmin(ctx context.Context, id string, isAdmin bool) error {
	return setIdentityAdmin(ctx, ts.q(), id, isAdmin)
}

func (ts *txStore) AddIdentityPublicKey(ctx context.Context, identityID string, pk ssh.PublicKey) error {
	return addIdentityPublicKey(ctx, ts.q(), identityID, pk)
}

func (ts *txStore) RemoveIdentityPublicKey(ctx context.Context, identityID string, keyID int64) error {
	return removeIdentityPublicKey(ctx, ts.q(), identityID, keyID)
}

func (ts *txStore) ListIdentityPublicKeys(ctx context.Context, identityID string) ([]*domain.PublicKey, error) {
	return listIdentityPublicKeys(ctx, ts.q(), identityID)
}
