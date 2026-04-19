package postgres

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
		&c.IdentityID,
		&c.RepoID,
		&c.AccessLevel,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

func getCollabByIdentityAndRepo(ctx context.Context, q querier, identityID, repo string) (*domain.Collab, error) {
	row := q.QueryRowContext(ctx,
		`SELECT collabs.id, collabs.identity_id, collabs.repo_id, collabs.access_level, collabs.created_at, collabs.updated_at
		 FROM collabs
		 INNER JOIN repos ON repos.id = collabs.repo_id
		 WHERE collabs.identity_id = $1 AND repos.name = $2`,
		identityID, repo)
	c, err := scanCollab(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: identity %q repo %q", domain.ErrCollaboratorNotFound, identityID, repo)
	}
	return c, err
}

func addCollabByIdentityAndRepo(ctx context.Context, q querier, identityID, repo string, level domain.AccessLevel) error {
	identityID = strings.TrimSpace(identityID)
	_, err := q.ExecContext(ctx,
		`INSERT INTO collabs (access_level, identity_id, repo_id, updated_at)
		 VALUES (
			$1,
			$2,
			(SELECT id FROM repos WHERE name = $3),
			NOW()
		 )`,
		level, identityID, repo)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: identity %q repo %q", domain.ErrCollaboratorExist, identityID, repo)
		}
		return err
	}
	return nil
}

func removeCollabByIdentityAndRepo(ctx context.Context, q querier, identityID, repo string) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM collabs
		 WHERE identity_id = $1
		 AND repo_id = (SELECT id FROM repos WHERE name = $2)`,
		identityID, repo)
	return err
}

func listCollabsByRepo(ctx context.Context, q querier, repo string) ([]*domain.Collab, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT collabs.id, collabs.identity_id, collabs.repo_id, collabs.access_level, collabs.created_at, collabs.updated_at
		 FROM collabs
		 INNER JOIN repos ON repos.id = collabs.repo_id
		 WHERE repos.name = $1`, repo)
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

func listCollabsByRepoAsIdentities(ctx context.Context, q querier, repo string) ([]*domain.Identity, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT identities.id, identities.username, identities.display_name, identities.type,
		        identities.is_admin, identities.created_at, identities.updated_at
		 FROM identities
		 INNER JOIN collabs ON collabs.identity_id = identities.id
		 INNER JOIN repos ON repos.id = collabs.repo_id
		 WHERE repos.name = $1`, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIdentities(rows)
}

// Store methods.

func (s *Store) GetCollabByIdentityAndRepo(ctx context.Context, identityID, repo string) (*domain.Collab, error) {
	return getCollabByIdentityAndRepo(ctx, s.q(), identityID, repo)
}

func (s *Store) AddCollabByIdentityAndRepo(ctx context.Context, identityID, repo string, level domain.AccessLevel) error {
	return addCollabByIdentityAndRepo(ctx, s.q(), identityID, repo, level)
}

func (s *Store) RemoveCollabByIdentityAndRepo(ctx context.Context, identityID, repo string) error {
	return removeCollabByIdentityAndRepo(ctx, s.q(), identityID, repo)
}

func (s *Store) ListCollabsByRepo(ctx context.Context, repo string) ([]*domain.Collab, error) {
	return listCollabsByRepo(ctx, s.q(), repo)
}

func (s *Store) ListCollabsByRepoAsIdentities(ctx context.Context, repo string) ([]*domain.Identity, error) {
	return listCollabsByRepoAsIdentities(ctx, s.q(), repo)
}

// txStore methods.

func (ts *txStore) GetCollabByIdentityAndRepo(ctx context.Context, identityID, repo string) (*domain.Collab, error) {
	return getCollabByIdentityAndRepo(ctx, ts.q(), identityID, repo)
}

func (ts *txStore) AddCollabByIdentityAndRepo(ctx context.Context, identityID, repo string, level domain.AccessLevel) error {
	return addCollabByIdentityAndRepo(ctx, ts.q(), identityID, repo, level)
}

func (ts *txStore) RemoveCollabByIdentityAndRepo(ctx context.Context, identityID, repo string) error {
	return removeCollabByIdentityAndRepo(ctx, ts.q(), identityID, repo)
}

func (ts *txStore) ListCollabsByRepo(ctx context.Context, repo string) ([]*domain.Collab, error) {
	return listCollabsByRepo(ctx, ts.q(), repo)
}

func (ts *txStore) ListCollabsByRepoAsIdentities(ctx context.Context, repo string) ([]*domain.Identity, error) {
	return listCollabsByRepoAsIdentities(ctx, ts.q(), repo)
}
