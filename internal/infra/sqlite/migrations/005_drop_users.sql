-- +goose Up

-- Recreate repos without user_id FK, adding identity_id TEXT
CREATE TABLE repos_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  project_name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  private BOOLEAN NOT NULL DEFAULT 0,
  mirror BOOLEAN NOT NULL DEFAULT 0,
  hidden BOOLEAN NOT NULL DEFAULT 0,
  identity_id TEXT REFERENCES identities(id) ON DELETE SET NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO repos_new (id, name, project_name, description, private, mirror, hidden, identity_id, created_at, updated_at)
  SELECT id, name, project_name, description, private, mirror, hidden, NULL, created_at, updated_at FROM repos;
DROP TABLE repos;
ALTER TABLE repos_new RENAME TO repos;

-- Recreate lfs_locks without user_id FK, adding identity_id TEXT
CREATE TABLE lfs_locks_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id INTEGER NOT NULL,
  identity_id TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL,
  refname TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (repo_id, path),
  CONSTRAINT repo_id_fk
  FOREIGN KEY(repo_id) REFERENCES repos(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE,
  CONSTRAINT identity_id_fk
  FOREIGN KEY(identity_id) REFERENCES identities(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);
INSERT INTO lfs_locks_new (id, repo_id, identity_id, path, refname, created_at, updated_at)
  SELECT id, repo_id, '', path, refname, created_at, updated_at FROM lfs_locks;
DROP TABLE lfs_locks;
ALTER TABLE lfs_locks_new RENAME TO lfs_locks;

-- Recreate collabs without user_id FK, using identity_id TEXT
CREATE TABLE collabs_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_id TEXT NOT NULL,
  repo_id INTEGER NOT NULL,
  access_level INTEGER NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (identity_id, repo_id),
  CONSTRAINT identity_id_fk
  FOREIGN KEY(identity_id) REFERENCES identities(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE,
  CONSTRAINT repo_id_fk
  FOREIGN KEY(repo_id) REFERENCES repos(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);
DROP TABLE collabs;
ALTER TABLE collabs_new RENAME TO collabs;

-- Drop legacy tables
DROP TABLE IF EXISTS access_tokens;
DROP TABLE IF EXISTS public_keys;
DROP TABLE IF EXISTS users;

-- +goose Down

CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  admin BOOLEAN NOT NULL DEFAULT 0,
  password TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS public_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  public_key TEXT NOT NULL UNIQUE,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_id_fk
  FOREIGN KEY(user_id) REFERENCES users(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);

CREATE TABLE IF NOT EXISTS access_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  token TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  user_id INTEGER NOT NULL,
  expires_at DATETIME,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_id_fk
  FOREIGN KEY(user_id) REFERENCES users(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);
