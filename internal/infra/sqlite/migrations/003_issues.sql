-- +goose Up

CREATE TABLE issues (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    number      INTEGER NOT NULL,
    repo_id     INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    author_id   TEXT NOT NULL REFERENCES identities(id),
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open',
    resolution  TEXT NOT NULL DEFAULT '',
    assignee_id TEXT REFERENCES identities(id),
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    closed_at   DATETIME,
    UNIQUE(repo_id, number)
);

CREATE INDEX idx_issues_repo_status ON issues(repo_id, status);

CREATE TABLE issue_labels (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    UNIQUE(issue_id, label)
);

CREATE TABLE issue_comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL REFERENCES identities(id),
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS issue_comments;
DROP TABLE IF EXISTS issue_labels;
DROP TABLE IF EXISTS issues;
