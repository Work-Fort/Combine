# Bug: HTTP Server Missing ReadTimeout and WriteTimeout

## Problem

`internal/infra/httpapi/http.go` lines 31-32 — the `http.Server` sets
`ReadHeaderTimeout` and `IdleTimeout` but is missing `ReadTimeout` and
`WriteTimeout`.

Without `ReadTimeout`, a client can hold a connection open by sending a
request body slowly. Without `WriteTimeout`, a slow client reading a
response can hold server resources indefinitely.

## Fix

Add the missing timeouts:

```go
ReadTimeout:  15 * time.Second
WriteTimeout: 15 * time.Second
```

See `codex/src/architecture/go-service-patterns.md` for the standard pattern.

## Severity

Medium — partial mitigation from existing ReadHeaderTimeout and IdleTimeout.
