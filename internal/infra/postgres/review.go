package postgres

import (
	"context"

	"github.com/Work-Fort/Combine/internal/domain"
)

func createReview(ctx context.Context, q querier, review *domain.PullRequestReview) error {
	row := q.QueryRowContext(ctx,
		`INSERT INTO pull_request_reviews (pr_id, author_id, state, body) VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		review.PRID, review.AuthorID, review.State, review.Body,
	)
	if err := row.Scan(&review.ID, &review.CreatedAt); err != nil {
		return err
	}

	for _, c := range review.Comments {
		c.ReviewID = review.ID
		if err := createReviewComment(ctx, q, c); err != nil {
			return err
		}
	}
	return nil
}

func listReviewsByPRID(ctx context.Context, q querier, prID int64) ([]*domain.PullRequestReview, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, pr_id, author_id, state, body, created_at
		 FROM pull_request_reviews WHERE pr_id = $1 ORDER BY created_at ASC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []*domain.PullRequestReview
	for rows.Next() {
		var r domain.PullRequestReview
		if err := rows.Scan(&r.ID, &r.PRID, &r.AuthorID, &r.State, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Comments, err = listReviewComments(ctx, q, r.ID)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, &r)
	}
	return reviews, rows.Err()
}

func createReviewComment(ctx context.Context, q querier, comment *domain.ReviewComment) error {
	row := q.QueryRowContext(ctx,
		`INSERT INTO review_comments (review_id, path, line, side, body, updated_at)
		 VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP) RETURNING id, created_at, updated_at`,
		comment.ReviewID, comment.Path, comment.Line, comment.Side, comment.Body,
	)
	return row.Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)
}

func listReviewComments(ctx context.Context, q querier, reviewID int64) ([]*domain.ReviewComment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, review_id, path, line, side, body, created_at, updated_at
		 FROM review_comments WHERE review_id = $1 ORDER BY path, line`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*domain.ReviewComment
	for rows.Next() {
		var c domain.ReviewComment
		if err := rows.Scan(&c.ID, &c.ReviewID, &c.Path, &c.Line, &c.Side, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}
	return comments, rows.Err()
}

// Store methods.

func (s *Store) CreateReview(ctx context.Context, review *domain.PullRequestReview) error {
	return createReview(ctx, s.q(), review)
}

func (s *Store) ListReviewsByPRID(ctx context.Context, prID int64) ([]*domain.PullRequestReview, error) {
	return listReviewsByPRID(ctx, s.q(), prID)
}

func (s *Store) CreateReviewComment(ctx context.Context, comment *domain.ReviewComment) error {
	return createReviewComment(ctx, s.q(), comment)
}

func (s *Store) ListReviewComments(ctx context.Context, reviewID int64) ([]*domain.ReviewComment, error) {
	return listReviewComments(ctx, s.q(), reviewID)
}

// txStore methods.

func (ts *txStore) CreateReview(ctx context.Context, review *domain.PullRequestReview) error {
	return createReview(ctx, ts.q(), review)
}

func (ts *txStore) ListReviewsByPRID(ctx context.Context, prID int64) ([]*domain.PullRequestReview, error) {
	return listReviewsByPRID(ctx, ts.q(), prID)
}

func (ts *txStore) CreateReviewComment(ctx context.Context, comment *domain.ReviewComment) error {
	return createReviewComment(ctx, ts.q(), comment)
}

func (ts *txStore) ListReviewComments(ctx context.Context, reviewID int64) ([]*domain.ReviewComment, error) {
	return listReviewComments(ctx, ts.q(), reviewID)
}
