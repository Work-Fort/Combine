package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Work-Fort/Combine/internal/domain"
)

const issueColumns = `id, number, repo_id, author_id, title, body, status, resolution, assignee_id, created_at, updated_at, closed_at`

func scanIssue(row interface{ Scan(dest ...any) error }) (*domain.Issue, error) {
	var i domain.Issue
	var assigneeID sql.NullString
	var closedAt sql.NullTime
	if err := row.Scan(
		&i.ID, &i.Number, &i.RepoID, &i.AuthorID,
		&i.Title, &i.Body, &i.Status, &i.Resolution,
		&assigneeID, &i.CreatedAt, &i.UpdatedAt, &closedAt,
	); err != nil {
		return nil, err
	}
	if assigneeID.Valid {
		i.AssigneeID = &assigneeID.String
	}
	if closedAt.Valid {
		i.ClosedAt = &closedAt.Time
	}
	return &i, nil
}

func scanIssues(rows *sql.Rows) ([]*domain.Issue, error) {
	var issues []*domain.Issue
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

func getIssueLabels(ctx context.Context, q querier, issueID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT label FROM issue_labels WHERE issue_id = $1 ORDER BY label`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

func createIssue(ctx context.Context, q querier, issue *domain.Issue) error {
	num, err := nextNumber(ctx, q, issue.RepoID)
	if err != nil {
		return err
	}

	row := q.QueryRowContext(ctx,
		`INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		 RETURNING id, created_at, updated_at`,
		num, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
		issue.Status, issue.Resolution, issue.AssigneeID,
	)
	issue.Number = num
	return row.Scan(&issue.ID, &issue.CreatedAt, &issue.UpdatedAt)
}

func getIssueByNumber(ctx context.Context, q querier, repoID, number int64) (*domain.Issue, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+issueColumns+` FROM issues WHERE repo_id = $1 AND number = $2`,
		repoID, number)
	issue, err := scanIssue(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: #%d", domain.ErrIssueNotFound, number)
	}
	if err != nil {
		return nil, err
	}
	issue.Labels, err = getIssueLabels(ctx, q, issue.ID)
	return issue, err
}

func listIssues(ctx context.Context, q querier, repoID int64, opts domain.IssueListOptions) ([]*domain.Issue, error) {
	where := []string{"repo_id = $1"}
	args := []any{repoID}
	argIdx := 2

	if opts.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*opts.Status))
		argIdx++
	}
	if opts.AssigneeID != nil {
		where = append(where, fmt.Sprintf("assignee_id = $%d", argIdx))
		args = append(args, *opts.AssigneeID)
		argIdx++
	}
	if opts.Label != nil {
		where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = issues.id AND label = $%d)", argIdx))
		args = append(args, *opts.Label)
		argIdx++
	}

	query := `SELECT ` + issueColumns + ` FROM issues WHERE ` + strings.Join(where, " AND ") + ` ORDER BY number DESC`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	issues, err := scanIssues(rows)
	if err != nil {
		return nil, err
	}

	for _, issue := range issues {
		issue.Labels, err = getIssueLabels(ctx, q, issue.ID)
		if err != nil {
			return nil, err
		}
	}
	return issues, nil
}

func updateIssue(ctx context.Context, q querier, issue *domain.Issue) error {
	row := q.QueryRowContext(ctx,
		`UPDATE issues SET title = $1, body = $2, status = $3, resolution = $4, assignee_id = $5, closed_at = $6, updated_at = NOW()
		 WHERE id = $7 RETURNING updated_at`,
		issue.Title, issue.Body, issue.Status, issue.Resolution, issue.AssigneeID, issue.ClosedAt, issue.ID,
	)
	return row.Scan(&issue.UpdatedAt)
}

func setIssueLabels(ctx context.Context, q querier, issueID int64, labels []string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM issue_labels WHERE issue_id = $1`, issueID); err != nil {
		return err
	}
	for _, label := range labels {
		if _, err := q.ExecContext(ctx, `INSERT INTO issue_labels (issue_id, label) VALUES ($1, $2)`, issueID, label); err != nil {
			return err
		}
	}
	return nil
}

func createIssueComment(ctx context.Context, q querier, comment *domain.IssueComment) error {
	row := q.QueryRowContext(ctx,
		`INSERT INTO issue_comments (issue_id, author_id, body, updated_at) VALUES ($1, $2, $3, NOW()) RETURNING id, created_at, updated_at`,
		comment.IssueID, comment.AuthorID, comment.Body,
	)
	return row.Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)
}

func listIssueComments(ctx context.Context, q querier, issueID int64) ([]*domain.IssueComment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, issue_id, author_id, body, created_at, updated_at FROM issue_comments WHERE issue_id = $1 ORDER BY created_at ASC`,
		issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*domain.IssueComment
	for rows.Next() {
		var c domain.IssueComment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}
	return comments, rows.Err()
}

// Store methods.

func (s *Store) CreateIssue(ctx context.Context, issue *domain.Issue) error {
	return createIssue(ctx, s.q(), issue)
}

func (s *Store) GetIssueByNumber(ctx context.Context, repoID int64, number int64) (*domain.Issue, error) {
	return getIssueByNumber(ctx, s.q(), repoID, number)
}

func (s *Store) ListIssues(ctx context.Context, repoID int64, opts domain.IssueListOptions) ([]*domain.Issue, error) {
	return listIssues(ctx, s.q(), repoID, opts)
}

func (s *Store) UpdateIssue(ctx context.Context, issue *domain.Issue) error {
	return updateIssue(ctx, s.q(), issue)
}

func (s *Store) SetIssueLabels(ctx context.Context, issueID int64, labels []string) error {
	return setIssueLabels(ctx, s.q(), issueID, labels)
}

func (s *Store) CreateIssueComment(ctx context.Context, comment *domain.IssueComment) error {
	return createIssueComment(ctx, s.q(), comment)
}

func (s *Store) ListIssueComments(ctx context.Context, issueID int64) ([]*domain.IssueComment, error) {
	return listIssueComments(ctx, s.q(), issueID)
}

// txStore methods.

func (ts *txStore) CreateIssue(ctx context.Context, issue *domain.Issue) error {
	return createIssue(ctx, ts.q(), issue)
}

func (ts *txStore) GetIssueByNumber(ctx context.Context, repoID int64, number int64) (*domain.Issue, error) {
	return getIssueByNumber(ctx, ts.q(), repoID, number)
}

func (ts *txStore) ListIssues(ctx context.Context, repoID int64, opts domain.IssueListOptions) ([]*domain.Issue, error) {
	return listIssues(ctx, ts.q(), repoID, opts)
}

func (ts *txStore) UpdateIssue(ctx context.Context, issue *domain.Issue) error {
	return updateIssue(ctx, ts.q(), issue)
}

func (ts *txStore) SetIssueLabels(ctx context.Context, issueID int64, labels []string) error {
	return setIssueLabels(ctx, ts.q(), issueID, labels)
}

func (ts *txStore) CreateIssueComment(ctx context.Context, comment *domain.IssueComment) error {
	return createIssueComment(ctx, ts.q(), comment)
}

func (ts *txStore) ListIssueComments(ctx context.Context, issueID int64) ([]*domain.IssueComment, error) {
	return listIssueComments(ctx, ts.q(), issueID)
}
