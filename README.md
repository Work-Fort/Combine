# Combine

A self-hostable Git forge for the WorkFort platform.

Combine provides Git hosting over SSH, HTTP, and the Git protocol, with
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

## License

[MIT](LICENSE) (inherited from Soft Serve)
