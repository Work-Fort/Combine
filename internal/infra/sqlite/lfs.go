package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
)

func scanLFSObject(row interface{ Scan(dest ...any) error }) (*domain.LFSObject, error) {
	var o domain.LFSObject
	if err := row.Scan(&o.ID, &o.Oid, &o.Size, &o.RepoID, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	return &o, nil
}

func scanLFSObjects(rows *sql.Rows) ([]*domain.LFSObject, error) {
	var objs []*domain.LFSObject
	for rows.Next() {
		o, err := scanLFSObject(rows)
		if err != nil {
			return nil, err
		}
		objs = append(objs, o)
	}
	return objs, rows.Err()
}

const lfsObjectColumns = `id, oid, size, repo_id, created_at, updated_at`

func scanLFSLock(row interface{ Scan(dest ...any) error }) (*domain.LFSLock, error) {
	var l domain.LFSLock
	var refname sql.NullString
	if err := row.Scan(&l.ID, &l.RepoID, &l.IdentityID, &l.Path, &refname, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}
	if refname.Valid {
		l.Refname = refname.String
	}
	return &l, nil
}

func scanLFSLocks(rows *sql.Rows) ([]*domain.LFSLock, error) {
	var locks []*domain.LFSLock
	for rows.Next() {
		l, err := scanLFSLock(rows)
		if err != nil {
			return nil, err
		}
		locks = append(locks, l)
	}
	return locks, rows.Err()
}

const lfsLockColumns = `id, repo_id, identity_id, path, refname, created_at, updated_at`

func sanitizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	return path
}

// --- LFS Objects ---

func createLFSObject(ctx context.Context, q querier, repoID int64, oid string, size int64) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO lfs_objects (repo_id, oid, size, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		repoID, oid, size)
	if err != nil && isUniqueViolation(err) {
		return fmt.Errorf("%w: oid %q", domain.ErrAlreadyExists, oid)
	}
	return err
}

func getLFSObjectByOid(ctx context.Context, q querier, repoID int64, oid string) (*domain.LFSObject, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+lfsObjectColumns+` FROM lfs_objects WHERE repo_id = ? AND oid = ?`, repoID, oid)
	o, err := scanLFSObject(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: oid %q", domain.ErrNotFound, oid)
	}
	return o, err
}

func listLFSObjects(ctx context.Context, q querier, repoID int64) ([]*domain.LFSObject, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+lfsObjectColumns+` FROM lfs_objects WHERE repo_id = ?`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLFSObjects(rows)
}

func listLFSObjectsByName(ctx context.Context, q querier, name string) ([]*domain.LFSObject, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT lfs_objects.id, lfs_objects.oid, lfs_objects.size, lfs_objects.repo_id, lfs_objects.created_at, lfs_objects.updated_at
		 FROM lfs_objects
		 INNER JOIN repos ON lfs_objects.repo_id = repos.id
		 WHERE repos.name = ?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLFSObjects(rows)
}

func deleteLFSObjectByOid(ctx context.Context, q querier, repoID int64, oid string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM lfs_objects WHERE repo_id = ? AND oid = ?`, repoID, oid)
	return err
}

// --- LFS Locks ---

func createLFSLockForIdentity(ctx context.Context, q querier, repoID int64, identityID, path, refname string) error {
	path = sanitizePath(path)
	_, err := q.ExecContext(ctx,
		`INSERT INTO lfs_locks (repo_id, identity_id, path, refname, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		repoID, identityID, path, refname)
	if err != nil && isUniqueViolation(err) {
		return fmt.Errorf("%w: path %q", domain.ErrAlreadyExists, path)
	}
	return err
}

func listLFSLocks(ctx context.Context, q querier, repoID int64, page, limit int) ([]*domain.LFSLock, error) {
	if page <= 0 {
		page = 1
	}
	rows, err := q.QueryContext(ctx,
		`SELECT `+lfsLockColumns+` FROM lfs_locks WHERE repo_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		repoID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLFSLocks(rows)
}

func listLFSLocksWithCount(ctx context.Context, q querier, repoID int64, page, limit int) ([]*domain.LFSLock, int64, error) {
	locks, err := listLFSLocks(ctx, q, repoID, page, limit)
	if err != nil {
		return nil, 0, err
	}
	var count int64
	row := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM lfs_locks WHERE repo_id = ?`, repoID)
	if err := row.Scan(&count); err != nil {
		return nil, 0, err
	}
	return locks, count, nil
}

func listLFSLocksForIdentity(ctx context.Context, q querier, repoID int64, identityID string) ([]*domain.LFSLock, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+lfsLockColumns+` FROM lfs_locks WHERE repo_id = ? AND identity_id = ?`, repoID, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLFSLocks(rows)
}

func getLFSLockForPath(ctx context.Context, q querier, repoID int64, path string) (*domain.LFSLock, error) {
	path = sanitizePath(path)
	row := q.QueryRowContext(ctx,
		`SELECT `+lfsLockColumns+` FROM lfs_locks WHERE repo_id = ? AND path = ?`, repoID, path)
	l, err := scanLFSLock(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: path %q", domain.ErrNotFound, path)
	}
	return l, err
}

func getLFSLockForIdentityPath(ctx context.Context, q querier, repoID int64, identityID, path string) (*domain.LFSLock, error) {
	path = sanitizePath(path)
	row := q.QueryRowContext(ctx,
		`SELECT `+lfsLockColumns+` FROM lfs_locks WHERE repo_id = ? AND identity_id = ? AND path = ?`,
		repoID, identityID, path)
	l, err := scanLFSLock(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: path %q", domain.ErrNotFound, path)
	}
	return l, err
}

func getLFSLockByID(ctx context.Context, q querier, id int64) (*domain.LFSLock, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+lfsLockColumns+` FROM lfs_locks WHERE id = ?`, id)
	l, err := scanLFSLock(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: lock id %d", domain.ErrNotFound, id)
	}
	return l, err
}

func getLFSLockForIdentityByID(ctx context.Context, q querier, repoID int64, identityID string, id int64) (*domain.LFSLock, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+lfsLockColumns+` FROM lfs_locks WHERE id = ? AND identity_id = ? AND repo_id = ?`,
		id, identityID, repoID)
	l, err := scanLFSLock(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: lock id %d", domain.ErrNotFound, id)
	}
	return l, err
}

func deleteLFSLock(ctx context.Context, q querier, repoID, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM lfs_locks WHERE repo_id = ? AND id = ?`, repoID, id)
	return err
}

func deleteLFSLockForIdentityByID(ctx context.Context, q querier, repoID int64, identityID string, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM lfs_locks WHERE repo_id = ? AND identity_id = ? AND id = ?`, repoID, identityID, id)
	return err
}

// Store methods.

func (s *Store) CreateLFSObject(ctx context.Context, repoID int64, oid string, size int64) error {
	return createLFSObject(ctx, s.q(), repoID, oid, size)
}

func (s *Store) GetLFSObjectByOid(ctx context.Context, repoID int64, oid string) (*domain.LFSObject, error) {
	return getLFSObjectByOid(ctx, s.q(), repoID, oid)
}

func (s *Store) ListLFSObjects(ctx context.Context, repoID int64) ([]*domain.LFSObject, error) {
	return listLFSObjects(ctx, s.q(), repoID)
}

func (s *Store) ListLFSObjectsByName(ctx context.Context, name string) ([]*domain.LFSObject, error) {
	return listLFSObjectsByName(ctx, s.q(), name)
}

func (s *Store) DeleteLFSObjectByOid(ctx context.Context, repoID int64, oid string) error {
	return deleteLFSObjectByOid(ctx, s.q(), repoID, oid)
}

func (s *Store) CreateLFSLockForIdentity(ctx context.Context, repoID int64, identityID, path, refname string) error {
	return createLFSLockForIdentity(ctx, s.q(), repoID, identityID, path, refname)
}

func (s *Store) ListLFSLocks(ctx context.Context, repoID int64, page, limit int) ([]*domain.LFSLock, error) {
	return listLFSLocks(ctx, s.q(), repoID, page, limit)
}

func (s *Store) ListLFSLocksWithCount(ctx context.Context, repoID int64, page, limit int) ([]*domain.LFSLock, int64, error) {
	return listLFSLocksWithCount(ctx, s.q(), repoID, page, limit)
}

func (s *Store) ListLFSLocksForIdentity(ctx context.Context, repoID int64, identityID string) ([]*domain.LFSLock, error) {
	return listLFSLocksForIdentity(ctx, s.q(), repoID, identityID)
}

func (s *Store) GetLFSLockForPath(ctx context.Context, repoID int64, path string) (*domain.LFSLock, error) {
	return getLFSLockForPath(ctx, s.q(), repoID, path)
}

func (s *Store) GetLFSLockForIdentityPath(ctx context.Context, repoID int64, identityID, path string) (*domain.LFSLock, error) {
	return getLFSLockForIdentityPath(ctx, s.q(), repoID, identityID, path)
}

func (s *Store) GetLFSLockByID(ctx context.Context, id int64) (*domain.LFSLock, error) {
	return getLFSLockByID(ctx, s.q(), id)
}

func (s *Store) GetLFSLockForIdentityByID(ctx context.Context, repoID int64, identityID string, id int64) (*domain.LFSLock, error) {
	return getLFSLockForIdentityByID(ctx, s.q(), repoID, identityID, id)
}

func (s *Store) DeleteLFSLock(ctx context.Context, repoID, id int64) error {
	return deleteLFSLock(ctx, s.q(), repoID, id)
}

func (s *Store) DeleteLFSLockForIdentityByID(ctx context.Context, repoID int64, identityID string, id int64) error {
	return deleteLFSLockForIdentityByID(ctx, s.q(), repoID, identityID, id)
}

// txStore methods.

func (ts *txStore) CreateLFSObject(ctx context.Context, repoID int64, oid string, size int64) error {
	return createLFSObject(ctx, ts.q(), repoID, oid, size)
}

func (ts *txStore) GetLFSObjectByOid(ctx context.Context, repoID int64, oid string) (*domain.LFSObject, error) {
	return getLFSObjectByOid(ctx, ts.q(), repoID, oid)
}

func (ts *txStore) ListLFSObjects(ctx context.Context, repoID int64) ([]*domain.LFSObject, error) {
	return listLFSObjects(ctx, ts.q(), repoID)
}

func (ts *txStore) ListLFSObjectsByName(ctx context.Context, name string) ([]*domain.LFSObject, error) {
	return listLFSObjectsByName(ctx, ts.q(), name)
}

func (ts *txStore) DeleteLFSObjectByOid(ctx context.Context, repoID int64, oid string) error {
	return deleteLFSObjectByOid(ctx, ts.q(), repoID, oid)
}

func (ts *txStore) CreateLFSLockForIdentity(ctx context.Context, repoID int64, identityID, path, refname string) error {
	return createLFSLockForIdentity(ctx, ts.q(), repoID, identityID, path, refname)
}

func (ts *txStore) ListLFSLocks(ctx context.Context, repoID int64, page, limit int) ([]*domain.LFSLock, error) {
	return listLFSLocks(ctx, ts.q(), repoID, page, limit)
}

func (ts *txStore) ListLFSLocksWithCount(ctx context.Context, repoID int64, page, limit int) ([]*domain.LFSLock, int64, error) {
	return listLFSLocksWithCount(ctx, ts.q(), repoID, page, limit)
}

func (ts *txStore) ListLFSLocksForIdentity(ctx context.Context, repoID int64, identityID string) ([]*domain.LFSLock, error) {
	return listLFSLocksForIdentity(ctx, ts.q(), repoID, identityID)
}

func (ts *txStore) GetLFSLockForPath(ctx context.Context, repoID int64, path string) (*domain.LFSLock, error) {
	return getLFSLockForPath(ctx, ts.q(), repoID, path)
}

func (ts *txStore) GetLFSLockForIdentityPath(ctx context.Context, repoID int64, identityID, path string) (*domain.LFSLock, error) {
	return getLFSLockForIdentityPath(ctx, ts.q(), repoID, identityID, path)
}

func (ts *txStore) GetLFSLockByID(ctx context.Context, id int64) (*domain.LFSLock, error) {
	return getLFSLockByID(ctx, ts.q(), id)
}

func (ts *txStore) GetLFSLockForIdentityByID(ctx context.Context, repoID int64, identityID string, id int64) (*domain.LFSLock, error) {
	return getLFSLockForIdentityByID(ctx, ts.q(), repoID, identityID, id)
}

func (ts *txStore) DeleteLFSLock(ctx context.Context, repoID, id int64) error {
	return deleteLFSLock(ctx, ts.q(), repoID, id)
}

func (ts *txStore) DeleteLFSLockForIdentityByID(ctx context.Context, repoID int64, identityID string, id int64) error {
	return deleteLFSLockForIdentityByID(ctx, ts.q(), repoID, identityID, id)
}
