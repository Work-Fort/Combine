package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
)

func scanCollab(row interface{ Scan(dest ...any) error }) (*domain.Collab, error) {
	var c domain.Collab
	if err := row.Scan(
		&c.ID,
		&c.UserID,
		&c.RepoID,
		&c.AccessLevel,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

func getCollabByUsernameAndRepo(ctx context.Context, q querier, username, repo string) (*domain.Collab, error) {
	username = strings.ToLower(username)
	row := q.QueryRowContext(ctx,
		`SELECT collabs.id, collabs.user_id, collabs.repo_id, collabs.access_level, collabs.created_at, collabs.updated_at
		 FROM collabs
		 INNER JOIN users ON users.id = collabs.user_id
		 INNER JOIN repos ON repos.id = collabs.repo_id
		 WHERE users.username = ? AND repos.name = ?`,
		username, repo)
	c, err := scanCollab(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: user %q repo %q", domain.ErrCollaboratorNotFound, username, repo)
	}
	return c, err
}

func addCollabByUsernameAndRepo(ctx context.Context, q querier, username, repo string, level domain.AccessLevel) error {
	username = strings.ToLower(username)
	_, err := q.ExecContext(ctx,
		`INSERT INTO collabs (access_level, user_id, repo_id, updated_at)
		 VALUES (
			?,
			(SELECT id FROM users WHERE username = ?),
			(SELECT id FROM repos WHERE name = ?),
			CURRENT_TIMESTAMP
		 )`,
		level, username, repo)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: user %q repo %q", domain.ErrCollaboratorExist, username, repo)
		}
		return err
	}
	return nil
}

func removeCollabByUsernameAndRepo(ctx context.Context, q querier, username, repo string) error {
	username = strings.ToLower(username)
	_, err := q.ExecContext(ctx,
		`DELETE FROM collabs
		 WHERE user_id = (SELECT id FROM users WHERE username = ?)
		 AND repo_id = (SELECT id FROM repos WHERE name = ?)`,
		username, repo)
	return err
}

func listCollabsByRepo(ctx context.Context, q querier, repo string) ([]*domain.Collab, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT collabs.id, collabs.user_id, collabs.repo_id, collabs.access_level, collabs.created_at, collabs.updated_at
		 FROM collabs
		 INNER JOIN repos ON repos.id = collabs.repo_id
		 WHERE repos.name = ?`, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collabs []*domain.Collab
	for rows.Next() {
		c, err := scanCollab(rows)
		if err != nil {
			return nil, err
		}
		collabs = append(collabs, c)
	}
	return collabs, rows.Err()
}

func listCollabsByRepoAsUsers(ctx context.Context, q querier, repo string) ([]*domain.User, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT users.id, users.username, users.admin, users.password, users.created_at, users.updated_at
		 FROM users
		 INNER JOIN collabs ON collabs.user_id = users.id
		 INNER JOIN repos ON repos.id = collabs.repo_id
		 WHERE repos.name = ?`, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

// Store methods.

func (s *Store) GetCollabByUsernameAndRepo(ctx context.Context, username, repo string) (*domain.Collab, error) {
	return getCollabByUsernameAndRepo(ctx, s.q(), username, repo)
}

func (s *Store) AddCollabByUsernameAndRepo(ctx context.Context, username, repo string, level domain.AccessLevel) error {
	return addCollabByUsernameAndRepo(ctx, s.q(), username, repo, level)
}

func (s *Store) RemoveCollabByUsernameAndRepo(ctx context.Context, username, repo string) error {
	return removeCollabByUsernameAndRepo(ctx, s.q(), username, repo)
}

func (s *Store) ListCollabsByRepo(ctx context.Context, repo string) ([]*domain.Collab, error) {
	return listCollabsByRepo(ctx, s.q(), repo)
}

func (s *Store) ListCollabsByRepoAsUsers(ctx context.Context, repo string) ([]*domain.User, error) {
	return listCollabsByRepoAsUsers(ctx, s.q(), repo)
}

// txStore methods.

func (ts *txStore) GetCollabByUsernameAndRepo(ctx context.Context, username, repo string) (*domain.Collab, error) {
	return getCollabByUsernameAndRepo(ctx, ts.q(), username, repo)
}

func (ts *txStore) AddCollabByUsernameAndRepo(ctx context.Context, username, repo string, level domain.AccessLevel) error {
	return addCollabByUsernameAndRepo(ctx, ts.q(), username, repo, level)
}

func (ts *txStore) RemoveCollabByUsernameAndRepo(ctx context.Context, username, repo string) error {
	return removeCollabByUsernameAndRepo(ctx, ts.q(), username, repo)
}

func (ts *txStore) ListCollabsByRepo(ctx context.Context, repo string) ([]*domain.Collab, error) {
	return listCollabsByRepo(ctx, ts.q(), repo)
}

func (ts *txStore) ListCollabsByRepoAsUsers(ctx context.Context, repo string) ([]*domain.User, error) {
	return listCollabsByRepoAsUsers(ctx, ts.q(), repo)
}
