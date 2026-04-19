package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Work-Fort/Combine/internal/domain"
)

func scanRepo(row interface{ Scan(dest ...any) error }) (*domain.Repo, error) {
	var r domain.Repo
	var identityID sql.NullString
	if err := row.Scan(
		&r.ID,
		&r.Name,
		&r.ProjectName,
		&r.Description,
		&r.Private,
		&r.Mirror,
		&r.Hidden,
		&identityID,
		&r.CreatedAt,
		&r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if identityID.Valid {
		v := identityID.String
		r.IdentityID = &v
	}
	return &r, nil
}

func scanRepos(rows *sql.Rows) ([]*domain.Repo, error) {
	var repos []*domain.Repo
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

const repoColumns = `id, name, project_name, description, private, mirror, hidden, identity_id, created_at, updated_at`

func getRepoByName(ctx context.Context, q querier, name string) (*domain.Repo, error) {
	row := q.QueryRowContext(ctx, `SELECT `+repoColumns+` FROM repos WHERE name = ?`, name)
	r, err := scanRepo(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %q", domain.ErrRepoNotFound, name)
	}
	return r, err
}

func listRepos(ctx context.Context, q querier) ([]*domain.Repo, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+repoColumns+` FROM repos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRepos(rows)
}

func listReposByIdentityID(ctx context.Context, q querier, identityID string) ([]*domain.Repo, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+repoColumns+` FROM repos WHERE identity_id = ?`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRepos(rows)
}

func createRepo(ctx context.Context, q querier, repo *domain.Repo) error {
	var res sql.Result
	var err error
	if repo.IdentityID != nil {
		res, err = q.ExecContext(ctx,
			`INSERT INTO repos (name, project_name, description, private, mirror, hidden, identity_id, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			repo.Name, repo.ProjectName, repo.Description, repo.Private, repo.Mirror, repo.Hidden, *repo.IdentityID,
		)
	} else {
		res, err = q.ExecContext(ctx,
			`INSERT INTO repos (name, project_name, description, private, mirror, hidden, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			repo.Name, repo.ProjectName, repo.Description, repo.Private, repo.Mirror, repo.Hidden,
		)
	}
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %q", domain.ErrRepoExist, repo.Name)
		}
		return err
	}
	id, _ := res.LastInsertId()
	repo.ID = id
	return nil
}

func updateRepo(ctx context.Context, q querier, repo *domain.Repo) error {
	_, err := q.ExecContext(ctx,
		`UPDATE repos SET name = ?, project_name = ?, description = ?, private = ?, mirror = ?, hidden = ?, identity_id = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		repo.Name, repo.ProjectName, repo.Description, repo.Private, repo.Mirror, repo.Hidden, repo.IdentityID, repo.ID,
	)
	return err
}

func deleteRepoByName(ctx context.Context, q querier, name string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM repos WHERE name = ?`, name)
	return err
}

// Store methods.

func (s *Store) GetRepoByName(ctx context.Context, name string) (*domain.Repo, error) {
	return getRepoByName(ctx, s.q(), name)
}

func (s *Store) ListRepos(ctx context.Context) ([]*domain.Repo, error) {
	return listRepos(ctx, s.q())
}

func (s *Store) ListReposByIdentityID(ctx context.Context, identityID string) ([]*domain.Repo, error) {
	return listReposByIdentityID(ctx, s.q(), identityID)
}

func (s *Store) CreateRepo(ctx context.Context, repo *domain.Repo) error {
	return createRepo(ctx, s.q(), repo)
}

func (s *Store) UpdateRepo(ctx context.Context, repo *domain.Repo) error {
	return updateRepo(ctx, s.q(), repo)
}

func (s *Store) DeleteRepoByName(ctx context.Context, name string) error {
	return deleteRepoByName(ctx, s.q(), name)
}

// txStore methods.

func (ts *txStore) GetRepoByName(ctx context.Context, name string) (*domain.Repo, error) {
	return getRepoByName(ctx, ts.q(), name)
}

func (ts *txStore) ListRepos(ctx context.Context) ([]*domain.Repo, error) {
	return listRepos(ctx, ts.q())
}

func (ts *txStore) ListReposByIdentityID(ctx context.Context, identityID string) ([]*domain.Repo, error) {
	return listReposByIdentityID(ctx, ts.q(), identityID)
}

func (ts *txStore) CreateRepo(ctx context.Context, repo *domain.Repo) error {
	return createRepo(ctx, ts.q(), repo)
}

func (ts *txStore) UpdateRepo(ctx context.Context, repo *domain.Repo) error {
	return updateRepo(ctx, ts.q(), repo)
}

func (ts *txStore) DeleteRepoByName(ctx context.Context, name string) error {
	return deleteRepoByName(ctx, ts.q(), name)
}
