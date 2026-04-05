package sqlite

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
	rows, err := q.QueryContext(ctx, `SELECT label FROM issue_labels WHERE issue_id = ? ORDER BY label`, issueID)
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

	res, err := q.ExecContext(ctx,
		`INSERT INTO issues (number, repo_id, author_id, title, body, status, resolution, assignee_id, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		num, issue.RepoID, issue.AuthorID, issue.Title, issue.Body,
		issue.Status, issue.Resolution, issue.AssigneeID,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	issue.ID = id
	issue.Number = num
	row := q.QueryRowContext(ctx, `SELECT created_at, updated_at FROM issues WHERE id = ?`, id)
	return row.Scan(&issue.CreatedAt, &issue.UpdatedAt)
}

func getIssueByNumber(ctx context.Context, q querier, repoID, number int64) (*domain.Issue, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+issueColumns+` FROM issues WHERE repo_id = ? AND number = ?`,
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
	where := []string{"repo_id = ?"}
	args := []any{repoID}

	if opts.Status != nil {
		where = append(where, "status = ?")
		args = append(args, string(*opts.Status))
	}
	if opts.AssigneeID != nil {
		where = append(where, "assignee_id = ?")
		args = append(args, *opts.AssigneeID)
	}
	if opts.Label != nil {
		where = append(where, "EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = issues.id AND label = ?)")
		args = append(args, *opts.Label)
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
	_, err := q.ExecContext(ctx,
		`UPDATE issues SET title = ?, body = ?, status = ?, resolution = ?, assignee_id = ?, closed_at = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		issue.Title, issue.Body, issue.Status, issue.Resolution, issue.AssigneeID, issue.ClosedAt, issue.ID,
	)
	if err != nil {
		return err
	}
	row := q.QueryRowContext(ctx, `SELECT updated_at FROM issues WHERE id = ?`, issue.ID)
	return row.Scan(&issue.UpdatedAt)
}

func setIssueLabels(ctx context.Context, q querier, issueID int64, labels []string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM issue_labels WHERE issue_id = ?`, issueID); err != nil {
		return err
	}
	for _, label := range labels {
		if _, err := q.ExecContext(ctx, `INSERT INTO issue_labels (issue_id, label) VALUES (?, ?)`, issueID, label); err != nil {
			return err
		}
	}
	return nil
}

func createIssueComment(ctx context.Context, q querier, comment *domain.IssueComment) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO issue_comments (issue_id, author_id, body, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		comment.IssueID, comment.AuthorID, comment.Body,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	comment.ID = id
	row := q.QueryRowContext(ctx, `SELECT created_at, updated_at FROM issue_comments WHERE id = ?`, id)
	return row.Scan(&comment.CreatedAt, &comment.UpdatedAt)
}

func listIssueComments(ctx context.Context, q querier, issueID int64) ([]*domain.IssueComment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, issue_id, author_id, body, created_at, updated_at FROM issue_comments WHERE issue_id = ? ORDER BY created_at ASC`,
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
