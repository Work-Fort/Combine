-- +goose Up

-- Shared per-repo number counter for issues and PRs.
CREATE TABLE repo_counters (
    repo_id     BIGINT PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    next_number BIGINT NOT NULL DEFAULT 1
);

-- Initialize counters from existing issue numbers.
INSERT INTO repo_counters (repo_id, next_number)
SELECT repo_id, MAX(number) + 1
FROM issues
GROUP BY repo_id;

-- Ensure repos without issues also get a counter row.
INSERT INTO repo_counters (repo_id, next_number)
SELECT id, 1 FROM repos
ON CONFLICT DO NOTHING;

-- Pull requests table.
CREATE TABLE pull_requests (
    id             BIGSERIAL PRIMARY KEY,
    number         BIGINT NOT NULL,
    repo_id        BIGINT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    author_id      TEXT NOT NULL REFERENCES identities(id),
    title          TEXT NOT NULL,
    body           TEXT NOT NULL DEFAULT '',
    source_branch  TEXT NOT NULL,
    target_branch  TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'open',
    merge_method   TEXT,
    merged_by      TEXT REFERENCES identities(id),
    assignee_id    TEXT REFERENCES identities(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    merged_at      TIMESTAMPTZ,
    closed_at      TIMESTAMPTZ,
    UNIQUE(repo_id, number)
);

CREATE INDEX idx_pull_requests_repo_status ON pull_requests(repo_id, status);

-- Review tables (schema only — populated in plan 7c).
CREATE TABLE pull_request_reviews (
    id         BIGSERIAL PRIMARY KEY,
    pr_id      BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES identities(id),
    state      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE review_comments (
    id         BIGSERIAL PRIMARY KEY,
    review_id  BIGINT NOT NULL REFERENCES pull_request_reviews(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    line       INTEGER NOT NULL,
    side       TEXT NOT NULL DEFAULT 'right',
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS review_comments;
DROP TABLE IF EXISTS pull_request_reviews;
DROP TABLE IF EXISTS pull_requests;
DROP TABLE IF EXISTS repo_counters;
