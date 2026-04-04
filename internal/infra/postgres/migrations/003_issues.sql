-- +goose Up

CREATE TABLE issues (
    id          BIGSERIAL PRIMARY KEY,
    number      INTEGER NOT NULL,
    repo_id     BIGINT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    author_id   TEXT NOT NULL REFERENCES identities(id),
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open',
    resolution  TEXT NOT NULL DEFAULT '',
    assignee_id TEXT REFERENCES identities(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at   TIMESTAMPTZ,
    UNIQUE(repo_id, number)
);

CREATE INDEX idx_issues_repo_status ON issues(repo_id, status);

CREATE TABLE issue_labels (
    id       BIGSERIAL PRIMARY KEY,
    issue_id BIGINT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    UNIQUE(issue_id, label)
);

CREATE TABLE issue_comments (
    id         BIGSERIAL PRIMARY KEY,
    issue_id   BIGINT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES identities(id),
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS issue_comments;
DROP TABLE IF EXISTS issue_labels;
DROP TABLE IF EXISTS issues;
