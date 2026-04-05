package sqlite

import (
	"context"
)

// nextNumber atomically allocates the next per-repo number (shared by issues and PRs).
// Must be called within a transaction for safety.
func nextNumber(ctx context.Context, q querier, repoID int64) (int64, error) {
	// Ensure a counter row exists for this repo.
	_, err := q.ExecContext(ctx,
		`INSERT OR IGNORE INTO repo_counters (repo_id, next_number) VALUES (?, 1)`, repoID)
	if err != nil {
		return 0, err
	}

	// Atomically increment and return.
	var num int64
	err = q.QueryRowContext(ctx,
		`UPDATE repo_counters SET next_number = next_number + 1 WHERE repo_id = ? RETURNING next_number - 1`,
		repoID).Scan(&num)
	if err != nil {
		return 0, err
	}
	return num, nil
}
