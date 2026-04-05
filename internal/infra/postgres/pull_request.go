package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
)

const prColumns = `id, number, repo_id, author_id, title, body, source_branch, target_branch, status, merge_method, merged_by, assignee_id, created_at, updated_at, merged_at, closed_at`

func scanPullRequest(row interface{ Scan(dest ...any) error }) (*domain.PullRequest, error) {
	var pr domain.PullRequest
	var mergeMethod sql.NullString
	var mergedBy sql.NullString
	var assigneeID sql.NullString
	var mergedAt sql.NullTime
	var closedAt sql.NullTime
	if err := row.Scan(
		&pr.ID, &pr.Number, &pr.RepoID, &pr.AuthorID,
		&pr.Title, &pr.Body, &pr.SourceBranch, &pr.TargetBranch,
		&pr.Status, &mergeMethod, &mergedBy, &assigneeID,
		&pr.CreatedAt, &pr.UpdatedAt, &mergedAt, &closedAt,
	); err != nil {
		return nil, err
	}
	if mergeMethod.Valid {
		mm := domain.MergeMethod(mergeMethod.String)
		pr.MergeMethod = &mm
	}
	if mergedBy.Valid {
		pr.MergedBy = &mergedBy.String
	}
	if assigneeID.Valid {
		pr.AssigneeID = &assigneeID.String
	}
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}
	if closedAt.Valid {
		pr.ClosedAt = &closedAt.Time
	}
	return &pr, nil
}

func scanPullRequests(rows *sql.Rows) ([]*domain.PullRequest, error) {
	var prs []*domain.PullRequest
	for rows.Next() {
		pr, err := scanPullRequest(rows)
		if err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	return prs, rows.Err()
}

func createPullRequest(ctx context.Context, q querier, pr *domain.PullRequest) error {
	num, err := nextNumber(ctx, q, pr.RepoID)
	if err != nil {
		return err
	}

	row := q.QueryRowContext(ctx,
		`INSERT INTO pull_requests (number, repo_id, author_id, title, body, source_branch, target_branch, status, assignee_id, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		 RETURNING id, created_at, updated_at`,
		num, pr.RepoID, pr.AuthorID, pr.Title, pr.Body,
		pr.SourceBranch, pr.TargetBranch, pr.Status, pr.AssigneeID,
	)
	pr.Number = num
	return row.Scan(&pr.ID, &pr.CreatedAt, &pr.UpdatedAt)
}

func getPullRequestByNumber(ctx context.Context, q querier, repoID, number int64) (*domain.PullRequest, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+prColumns+` FROM pull_requests WHERE repo_id = $1 AND number = $2`,
		repoID, number)
	pr, err := scanPullRequest(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: #%d", domain.ErrPullRequestNotFound, number)
	}
	return pr, err
}

func listPullRequests(ctx context.Context, q querier, repoID int64, opts domain.PullRequestListOptions) ([]*domain.PullRequest, error) {
	where := []string{"repo_id = $1"}
	args := []any{repoID}
	argIdx := 2

	if opts.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*opts.Status))
		argIdx++
	}
	if opts.AuthorID != nil {
		where = append(where, fmt.Sprintf("author_id = $%d", argIdx))
		args = append(args, *opts.AuthorID)
		argIdx++
	}

	query := `SELECT ` + prColumns + ` FROM pull_requests WHERE ` + strings.Join(where, " AND ") + ` ORDER BY number DESC`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPullRequests(rows)
}

func updatePullRequest(ctx context.Context, q querier, pr *domain.PullRequest) error {
	row := q.QueryRowContext(ctx,
		`UPDATE pull_requests SET title = $1, body = $2, status = $3, merge_method = $4, merged_by = $5,
		 assignee_id = $6, merged_at = $7, closed_at = $8, updated_at = NOW()
		 WHERE id = $9 RETURNING updated_at`,
		pr.Title, pr.Body, pr.Status, pr.MergeMethod, pr.MergedBy,
		pr.AssigneeID, pr.MergedAt, pr.ClosedAt, pr.ID,
	)
	return row.Scan(&pr.UpdatedAt)
}

// Store methods.

func (s *Store) CreatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
	return createPullRequest(ctx, s.q(), pr)
}

func (s *Store) GetPullRequestByNumber(ctx context.Context, repoID int64, number int64) (*domain.PullRequest, error) {
	return getPullRequestByNumber(ctx, s.q(), repoID, number)
}

func (s *Store) ListPullRequests(ctx context.Context, repoID int64, opts domain.PullRequestListOptions) ([]*domain.PullRequest, error) {
	return listPullRequests(ctx, s.q(), repoID, opts)
}

func (s *Store) UpdatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
	return updatePullRequest(ctx, s.q(), pr)
}

// txStore methods.

func (ts *txStore) CreatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
	return createPullRequest(ctx, ts.q(), pr)
}

func (ts *txStore) GetPullRequestByNumber(ctx context.Context, repoID int64, number int64) (*domain.PullRequest, error) {
	return getPullRequestByNumber(ctx, ts.q(), repoID, number)
}

func (ts *txStore) ListPullRequests(ctx context.Context, repoID int64, opts domain.PullRequestListOptions) ([]*domain.PullRequest, error) {
	return listPullRequests(ctx, ts.q(), repoID, opts)
}

func (ts *txStore) UpdatePullRequest(ctx context.Context, pr *domain.PullRequest) error {
	return updatePullRequest(ctx, ts.q(), pr)
}
