# Combine

A self-hostable Git forge for the WorkFort platform.

Combine provides Git hosting over SSH and HTTP, with
Git LFS support, access control, and webhooks. It is forked from
[Soft Serve](https://github.com/charmbracelet/soft-serve) by Charm.

## Quick Start

```bash
# Build
go build -o combine ./cmd/combine/

# Run (creates a data/ directory for repos, keys, and database)
COMBINE_INITIAL_ADMIN_KEYS="$(cat ~/.ssh/id_ed25519.pub)" ./combine serve
```

## Configuration

Configuration is loaded from `data/config.yaml` and can be overridden with
environment variables prefixed with `COMBINE_`.

| Variable | Description | Default |
|----------|-------------|---------|
| `COMBINE_DATA_PATH` | Data directory | `data` |
| `COMBINE_NAME` | Server name | `Combine` |
| `COMBINE_SSH_LISTEN_ADDR` | SSH listen address | `:23231` |
| `COMBINE_HTTP_LISTEN_ADDR` | HTTP listen address | `:23232` |
| `COMBINE_DB_DRIVER` | Database driver (`sqlite` or `postgres`) | `sqlite` |
| `COMBINE_INITIAL_ADMIN_KEYS` | Admin SSH public keys | |

## Development

Combine uses [mise](https://mise.jdx.dev/) to manage the Go and
golangci-lint toolchain. With mise installed:

```bash
mise install              # install pinned toolchain
mise run lint             # gofmt + go vet + golangci-lint
mise run test             # unit tests with -race and coverage
mise run build:dev        # build ./build/combine
mise run e2e              # build then run e2e tests against SQLite
mise run ci               # lint + test + e2e (full default-backend run)
```

### E2E against Postgres

The e2e harness selects its backend via env vars. Default (unset)
is SQLite; setting both runs against Postgres:

```bash
COMBINE_DB_DRIVER=postgres \
  COMBINE_DB_DATA_SOURCE="postgres://postgres@127.0.0.1/combine_e2e?sslmode=disable" \
  mise run e2e
```

The harness drops and recreates the `public` schema before each
test so runs are isolated.

### git-lfs

`TestLFSPushPull` requires `git-lfs` on the developer's PATH.
On Arch: `pacman -S git-lfs`.

## License

[MIT](LICENSE) (inherited from Soft Serve)
