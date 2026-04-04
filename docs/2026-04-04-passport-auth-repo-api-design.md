# Passport Auth + Repo Management REST API Design

## Overview

Integrate Passport (WorkFort's identity provider) for REST API authentication
and add a repo management REST API. This is the foundation for the issue
tracker and MCP bridge.

## Auth Model

**Passport** is the identity source of truth. Combine stores a local `Identity`
record auto-provisioned on first authenticated request.

Two auth domains:

- **REST API (HTTP):** Passport JWT + API key via `service-auth` middleware.
  Identity extracted from token claims.
- **SSH Git transport:** Combine's own SSH public key system. Users add keys
  via the REST API. SSH keys are linked to a Passport identity.

### Local Identity Table

Replaces the current `users` table.

| Field | Type | Description |
|-------|------|-------------|
| ID | string (UUID) | Passport-provided UUID, primary key |
| Username | string | From Passport claims |
| DisplayName | string | From Passport claims |
| Type | string | `user`, `agent`, `service` |
| IsAdmin | bool | Local admin flag |
| CreatedAt | timestamp | First seen |
| UpdatedAt | timestamp | Last claims update |

SSH public keys stay in a `public_keys` table linked to Identity ID. Collabs
table FKs update to reference Identity ID.

The existing users/passwords/access-tokens tables are dropped. Passport
handles web/API authentication; Combine handles SSH key auth.

## REST API

Base path: `/api/v1/`. All endpoints require Passport auth except health.

### Repo Management

- `GET /api/v1/repos` — list repos (filtered by visibility + access)
- `POST /api/v1/repos` — create repo
- `GET /api/v1/repos/{repo}` — get repo details
- `PATCH /api/v1/repos/{repo}` — update repo
- `DELETE /api/v1/repos/{repo}` — delete repo

### SSH Key Management

Users need SSH keys to push over SSH. Managed via the REST API.

- `GET /api/v1/user/keys` — list caller's SSH keys
- `POST /api/v1/user/keys` — add SSH key
- `DELETE /api/v1/user/keys/{id}` — remove SSH key

### Health (no auth)

- `GET /v1/health` — health check (DB ping, service info)
- `GET /ui/health` — Pylon service discovery

### Pylon Health Response

```json
{
  "service": "combine",
  "version": "...",
  "routes": [
    {"route": "/api/v1", "label": "API"}
  ]
}
```

## Public Path Skipping

Following the Hive/Sharkfin pattern, these paths skip Passport middleware:

- `/v1/health`, `/ui/health`
- Git smart HTTP transport (`/*/info/refs`, `/*/git-upload-pack`,
  `/*/git-receive-pack`) — keep existing auth (anonymous read for public
  repos, Basic auth for write)
- LFS endpoints — keep existing auth

## Configuration

New viper key: `passport-url` (env: `COMBINE_PASSPORT_URL`)

- When set: REST API requires Passport auth, JWT + API key validators initialized
- When empty: REST API is disabled (SSH-only mode, current behavior)

Combine stays functional without Passport for SSH-only use, matching the
"standalone viable" design principle.

## Dependency

```
github.com/Work-Fort/Passport/go/service-auth v0.0.2
```

Provides: `auth.NewFromValidators()`, `auth.IdentityFromContext()`,
`jwt.New()`, `apikey.New()`.

## Migration Impact

- `users` table → `identities` table (UUID PK, Passport claims)
- `access_tokens` table → dropped (Passport handles this)
- `public_keys` table → FK changes from user_id to identity_id
- `collabs` table → FK changes from user_id to identity_id
- `repos.user_id` → `repos.owner_id` referencing identity_id

Domain types updated: `User` struct replaced with `Identity` struct matching
Passport's identity model.
