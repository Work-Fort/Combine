-- +goose Up

-- Migrate repos: add identity_id, drop old user_id FK column
ALTER TABLE repos ADD COLUMN identity_id TEXT REFERENCES identities(id) ON DELETE SET NULL;
ALTER TABLE repos DROP COLUMN IF EXISTS user_id;

-- Migrate lfs_locks: add identity_id, drop old user_id FK column
ALTER TABLE lfs_locks ADD COLUMN identity_id TEXT NOT NULL DEFAULT '' REFERENCES identities(id) ON DELETE CASCADE;
ALTER TABLE lfs_locks DROP COLUMN IF EXISTS user_id;

-- Migrate collabs: replace user_id INTEGER with identity_id TEXT
ALTER TABLE collabs ADD COLUMN identity_id TEXT NOT NULL DEFAULT '' REFERENCES identities(id) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE collabs DROP CONSTRAINT IF EXISTS collabs_user_id_repo_id_key;
ALTER TABLE collabs DROP COLUMN user_id;
ALTER TABLE collabs ADD CONSTRAINT collabs_identity_id_repo_id_key UNIQUE (identity_id, repo_id);

-- Drop legacy tables (no more FK references at this point)
DROP TABLE IF EXISTS access_tokens;
DROP TABLE IF EXISTS public_keys;
DROP TABLE IF EXISTS users;

-- +goose Down

CREATE TABLE IF NOT EXISTS users (
  id SERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  admin BOOLEAN NOT NULL DEFAULT FALSE,
  password TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public_keys (
  id SERIAL PRIMARY KEY,
  user_id INTEGER NOT NULL,
  public_key TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT user_id_fk
  FOREIGN KEY(user_id) REFERENCES users(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);

CREATE TABLE IF NOT EXISTS access_tokens (
  id SERIAL PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  user_id INTEGER NOT NULL,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT user_id_fk
  FOREIGN KEY(user_id) REFERENCES users(id)
  ON DELETE CASCADE
  ON UPDATE CASCADE
);

-- Reverse FK column changes on repos, lfs_locks, collabs
ALTER TABLE repos DROP COLUMN IF EXISTS identity_id;
ALTER TABLE repos ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE lfs_locks DROP COLUMN IF EXISTS identity_id;
ALTER TABLE lfs_locks ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;

ALTER TABLE collabs DROP CONSTRAINT IF EXISTS collabs_identity_id_repo_id_key;
ALTER TABLE collabs DROP COLUMN IF EXISTS identity_id;
ALTER TABLE collabs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE collabs ADD CONSTRAINT collabs_user_id_repo_id_key UNIQUE (user_id, repo_id);
