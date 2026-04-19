---
type: plan
step: "1"
title: "Passport scheme split — Combine consumer migration"
status: approved
assessment_status: complete
provenance:
  source: cross-repo-coordination
  issue_id: "Cluster 3b (Passport, 2026-04-19)"
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: "2026-04-19"
  completed: null
related_plans:
  - passport/lead/docs/plans/2026-04-19-auth-scheme-dispatch.md
  - sharkfin/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - hive/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - flow/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
  - pylon/lead/docs/plans/2026-04-19-passport-scheme-split-consumer.md
---

# Combine — Passport Scheme Split Consumer Migration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Switch Combine's inbound middleware (`internal/infra/httpapi/passport.go`) to `NewSchemeDispatch`, switch the `mcp-bridge` outbound API-key call to `ApiKey-v1`, and rename the bridge's internal `token` field to `apiKey` for type honesty. The e2e harness JWT-bearing call sites are left untouched — those exercise the inbound JWT validator (still needed for browser-routed traffic). The Git LFS JWT signing path (`internal/infra/gitutil/lfs_auth.go`) is **out of scope** — it signs short-lived LFS-only JWTs that do not flow through Passport's middleware.

**Background / Why:** Per TPM clarification 2026-04-19: only web browser clients use JWT; agents and services use API keys. Combine's `mcp-bridge` is a service caller and unambiguously sends API keys outbound — the `--token` flag's documented purpose was already "Passport agent API key". Inbound middleware in Combine still needs both schemes because some inbound traffic is browser-routed JWT (via Scope) and some is `ApiKey-v1` from agents — the scheme-dispatch middleware handles both safely.

**Architecture:** Three regions:

1. **Inbound middleware** (`internal/infra/httpapi/passport.go:32`) — `auth.NewFromValidators(jwtV, akV)` → `auth.NewSchemeDispatch(jwtV, akV)`. Both schemes still accepted; dispatch is by `Authorization` scheme.
2. **Outbound mcp-bridge** (`cmd/mcpbridge/client.go:43`) — `Bearer + token` → `ApiKey-v1 + apiKey` (rename the internal field for type honesty; flag rename `--token` → `--api-key` for clarity).
3. **e2e harness** (`tests/e2e/harness/api_client.go:53`) — sends JWTs (issued by `d.SignJWT`), keep `Bearer`. The harness `signJWT` helper stays — it's how we exercise the inbound JWT-acceptance path.

The LFS JWT path (`internal/infra/gitutil/lfs_auth.go:78`) is a separate Git LFS-protocol contract: Combine signs a JWT, the Git LFS client puts it in `Authorization: Bearer <jwt>` on subsequent LFS protocol requests, and Combine's LFS handler validates it directly (not via Passport's middleware). This is correctly `Bearer` per the LFS spec and stays untouched.

**Tech Stack:** Go 1.x, cobra, viper. No new dependencies.

---

## Conventions

- Conventional Commits multi-line + `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` per commit.
- Combine has no `mise` tasks per `~/Work/WorkFort/INTEGRATION-ENVIRONMENT.md` § Known gaps. Use raw Go: `cd combine/lead && go test ./...` and `go build -o build/combine ./cmd/combine`.
- Pin `service-auth` to local passport branch via `replace`; drop in Task 4.

---

## Pre-flight: pin to local passport branch

Add to `go.mod`:

```
replace github.com/Work-Fort/Passport/go/service-auth => /home/kazw/Work/WorkFort/passport/lead/go/service-auth
```

`go mod tidy`, commit.

---

### Task 1: Switch inbound middleware to `NewSchemeDispatch`

**Files:**
- Modify: `internal/infra/httpapi/passport.go` — line 19 (`store domain.Store` → `domain.IdentityStore`), line 25 (same in `NewPassportAuth` signature), line 32 (`NewFromValidators` → `NewSchemeDispatch`)
- Add: `internal/infra/httpapi/passport_test.go` — verified to NOT exist at planning time; this task creates it

**Step 1: Write a failing test**

`internal/infra/httpapi/passport_test.go` does not exist today — create the file with the full test scaffolding below (no pseudocode):

`NewPassportAuth` only calls `store.UpsertIdentity` (verified at planning time — the only store call is at `passport.go:55`). The `PassportAuth` struct therefore needs only `domain.IdentityStore`, not the full composite `domain.Store`. This task narrows `PassportAuth.store` and `NewPassportAuth`'s parameter to `domain.IdentityStore` so the test's inline stub can satisfy it with the 9 methods of that interface without implementing the entire composite store.

```go
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/Work-Fort/Combine/internal/domain"
)

// passportStub stands in for the real Passport service. It serves
// /.well-known/jwks.json with a fixed minimal JWKS and
// /v1/verify-api-key returning 200 with a service identity for one
// canned key, 401 otherwise. It also records every verify-api-key call
// so the test can assert no fallthrough.
type passportStub struct {
	*httptest.Server
	verifyCount int64
	validKey    string
}

func newPassportStub(t *testing.T) *passportStub {
	t.Helper()
	s := &passportStub{validKey: "wf-svc_test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		// Empty JWKS is enough — these tests don't exercise JWT acceptance.
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
	})
	mux.HandleFunc("/v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.verifyCount, 1)
		var body struct{ Key string `json:"key"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Key != s.validKey {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "svc-1",
			"username": "test-service",
			"type":     "service",
		})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *passportStub) VerifyCount() int64 { return atomic.LoadInt64(&s.verifyCount) }

// inMemoryStore satisfies domain.IdentityStore (the narrow interface that
// NewPassportAuth now accepts — see Step 2). Only UpsertIdentity is
// exercised by the middleware; the remaining methods panic to surface any
// unexpected calls during testing.
type inMemoryStore struct{}

func (inMemoryStore) UpsertIdentity(_ context.Context, id, username, displayName, typ string) (*domain.Identity, error) {
	return &domain.Identity{ID: id, Username: username, DisplayName: displayName, Type: typ}, nil
}
func (inMemoryStore) GetIdentityByID(_ context.Context, _ string) (*domain.Identity, error) {
	panic("not implemented in test stub")
}
func (inMemoryStore) GetIdentityByUsername(_ context.Context, _ string) (*domain.Identity, error) {
	panic("not implemented in test stub")
}
func (inMemoryStore) GetIdentityByPublicKey(_ context.Context, _ ssh.PublicKey) (*domain.Identity, error) {
	panic("not implemented in test stub")
}
func (inMemoryStore) ListIdentities(_ context.Context) ([]*domain.Identity, error) {
	panic("not implemented in test stub")
}
func (inMemoryStore) SetIdentityAdmin(_ context.Context, _ string, _ bool) error {
	panic("not implemented in test stub")
}
func (inMemoryStore) AddIdentityPublicKey(_ context.Context, _ string, _ ssh.PublicKey) error {
	panic("not implemented in test stub")
}
func (inMemoryStore) RemoveIdentityPublicKey(_ context.Context, _ string, _ int64) error {
	panic("not implemented in test stub")
}
func (inMemoryStore) ListIdentityPublicKeys(_ context.Context, _ string) ([]*domain.PublicKey, error) {
	panic("not implemented in test stub")
}

func TestPassportAuth_BearerForAPIKeyReturns401(t *testing.T) {
	stub := newPassportStub(t)
	pa, err := NewPassportAuth(context.Background(), stub.URL, inMemoryStore{})
	if err != nil {
		t.Fatalf("NewPassportAuth: %v", err)
	}
	defer pa.Close()

	handler := pa.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called when API key is sent under Bearer")
	}))

	req := httptest.NewRequest("GET", "/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+stub.validKey) // wrong scheme
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := stub.VerifyCount(); got != 0 {
		t.Errorf("verify-api-key called %d times; want 0 (no fallthrough)", got)
	}
}

func TestPassportAuth_ApiKeyV1Routes(t *testing.T) {
	stub := newPassportStub(t)
	pa, err := NewPassportAuth(context.Background(), stub.URL, inMemoryStore{})
	if err != nil {
		t.Fatalf("NewPassportAuth: %v", err)
	}
	defer pa.Close()

	called := false
	handler := pa.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/x", nil)
	req.Header.Set("Authorization", "ApiKey-v1 "+stub.validKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !called {
		t.Error("downstream handler was not called")
	}
	if got := stub.VerifyCount(); got != 1 {
		t.Errorf("verify-api-key called %d times; want 1", got)
	}
	if !strings.HasPrefix(stub.URL, "http") {
		t.Errorf("stub URL malformed: %s", stub.URL) // sanity
	}
	_ = fmt.Sprintf // keep fmt import live if test scaffolding evolves
}
```

(The load-bearing assertions are the 401-with-zero-verify-count and the 200-with-one-verify-count.)

**Step 2: Narrow `NewPassportAuth` to `domain.IdentityStore` and apply the dispatch change**

In `internal/infra/httpapi/passport.go`, make two edits:

First, narrow the store field and constructor parameter (production code now depends on only the interface it actually uses):

```go
// PassportAuth wraps Passport's auth middleware and auto-provisions
// identities in Combine's store on each authenticated request.
type PassportAuth struct {
	mw    auth.Middleware
	store domain.IdentityStore
	jwtV  *jwt.Validator
}

// NewPassportAuth creates a PassportAuth that validates tokens against the
// Passport service at passportURL and upserts identities into store.
func NewPassportAuth(ctx context.Context, passportURL string, store domain.IdentityStore) (*PassportAuth, error) {
```

Second, replace the dispatch call (same file, same function body):

```go
mw := auth.NewFromValidators(jwtV, akV)
```

→

```go
mw := auth.NewSchemeDispatch(jwtV, akV)
```

Verified at planning time: `NewPassportAuth` (lines 23-35 of `passport.go`) already returns an error if `jwt.New` fails (line 28-30), so `jwtV` is always non-nil at the `NewSchemeDispatch` call site. **No `auth.AlwaysFail` substitution needed here.** (If a future refactor makes JWKS init non-fatal, use `auth.AlwaysFail(err)` from passport's `service-auth` — do NOT define a local stub. The upstream helper exists specifically to keep this consistent across consumers.)

**Step 3: Verify**

```
cd /home/kazw/Work/WorkFort/combine/lead
go test ./internal/infra/httpapi/...
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/infra/httpapi/passport.go internal/infra/httpapi/passport_test.go
git commit -m "$(cat <<'EOF'
feat(httpapi)!: dispatch inbound auth by Authorization scheme

BREAKING CHANGE: Combine's HTTP API now requires JWTs under "Bearer"
and API keys under "ApiKey-v1". Switches from passport's legacy
NewFromValidators chain (which retried on any error and amplified
malformed JWTs into verify-api-key calls — Cluster 3b) to the new
NewSchemeDispatch. Also narrows PassportAuth.store and
NewPassportAuth's store parameter from domain.Store to
domain.IdentityStore (the only method used is UpsertIdentity).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Inventory operator-facing flag references

**Files:** read-only inventory pass.

**Step 1: Run the operator-surface greps**

```bash
cd /home/kazw/Work/WorkFort/combine/lead

# Every place the old --token flag appears (must all migrate to --api-key):
grep -rn '\-\-token\|"token"\|COMBINE_TOKEN' --include='*.go' --include='*.json' --include='*.md' --include='*.sh' --include='*.service' --include='*.openrc' .

# Every operator-facing bridge invocation:
grep -rn 'mcp-bridge.*--token' .
```

**Step 2: Snapshot at planning time (re-verify before editing)**

Combine has **no `dist/` directory** and **no `.mcp.json`** at the repo root, so there is no shipped operator config bound to `--token` in this repo. The operator-facing surface is:

| File | Line | Kind | Action |
| --- | --- | --- | --- |
| `cmd/mcpbridge/mcp_bridge.go` | 32 | error message "--token is required" | flip to "--api-key is required" |
| `cmd/mcpbridge/client.go` | 43 | `Bearer + token` setter | edit (Task 3) |
| `cmd/mcpbridge/client.go` | 15-19 | `apiClient` struct + constructor | rename `token` → `apiKey` |
| `docs/2026-04-05-mcp-bridge-design.md` | 37, 43, 178 | operator-facing example invocations + env table | update (`--token` → `--api-key`, `COMBINE_TOKEN` → `COMBINE_API_KEY`) |
| `docs/plans/2026-04-05-mcp-bridge-plan.md` | 156, 334, 365 | historical plan — leave (documents pre-rename state) |

If the live grep finds a new file (e.g., a CI workflow, a private wrapper), include it in the rename pass.

**Step 3: No commit — this task produces only the inventory.**

---

### Task 3: Update `cmd/mcpbridge/client.go` outbound API-key path

**Files:**
- Modify: `cmd/mcpbridge/client.go:43` (header setter — `Bearer + token` → `ApiKey-v1 + apiKey`)
- Modify: `cmd/mcpbridge/client.go:15-19` (rename struct field + constructor parameter)
- Modify: `cmd/mcpbridge/mcp_bridge.go` (rename local variable, cobra flag, error message)
- Modify: `docs/2026-04-05-mcp-bridge-design.md` (operator-facing examples + env-var table)

**Step 1: Update the header and rename for type honesty**

The `--token` flag is documented as "Passport agent API key", so the bridge unambiguously sends API keys (per TPM clarification: agents and services never send JWT outbound). Rename the field on `apiClient` from `token` to `apiKey`, update `newAPIClient` accordingly, and replace:

```go
req.Header.Set("Authorization", "Bearer "+c.token)
```

with:

```go
req.Header.Set("Authorization", "ApiKey-v1 "+c.apiKey)
```

In `mcp_bridge.go`:
- Rename the local variable `token` → `apiKey` in `runBridge`.
- Rename the cobra flag `--token` → `--api-key` (env var bumps from `COMBINE_TOKEN` → `COMBINE_API_KEY` via cobra/viper convention).
- Update the cobra flag's help string to "Passport API key (sent as ApiKey-v1)".
- Update the error message at line 32: `"--api-key is required"` (was `"--token is required"`).

This is an operator-visible change — anyone running the bridge with `--token` or `COMBINE_TOKEN` must update their invocation. Documented in `~/Work/WorkFort/passport-scheme-split-deploy.md` § Operator-visible breaking changes.

**Step 2: Update `docs/2026-04-05-mcp-bridge-design.md`**

Per the Task 2 inventory, lines 37, 43, 178 reference the old flag/env name. Update each:

- Line 37: `combine mcp-bridge --server-url http://localhost:23235 --token <…>` → `combine mcp-bridge --server-url http://localhost:23235 --api-key <…>`
- Line 43 (env-var table): `--token` / `COMBINE_TOKEN` → `--api-key` / `COMBINE_API_KEY`
- Line 178 (`.mcp.json` operator example): `["mcp-bridge", "--server-url", "...", "--token", "<key>"]` → `["mcp-bridge", "--server-url", "...", "--api-key", "<key>"]`

(The historical plan at `docs/plans/2026-04-05-mcp-bridge-plan.md` is left as-is — it documents pre-rename state.)

**Step 3: Verify the bridge builds and basic tests pass**

```
cd /home/kazw/Work/WorkFort/combine/lead
go build ./cmd/mcpbridge/...
go test ./cmd/mcpbridge/...
```

Expected: build OK, tests PASS.

**Step 4: Verify the rename is exhaustive**

```
grep -rn '\-\-token\|COMBINE_TOKEN' --include='*.go' --include='*.md' --include='*.json' --include='*.sh' .
```

Expected: matches only inside `docs/plans/2026-04-05-mcp-bridge-plan.md` (historical) and `docs/plans/2026-04-19-passport-scheme-split-consumer.md` (this plan, in inventory tables).

**Step 5: Commit**

```bash
git add cmd/mcpbridge/client.go cmd/mcpbridge/mcp_bridge.go docs/2026-04-05-mcp-bridge-design.md
git commit -m "$(cat <<'EOF'
feat(mcpbridge)!: rename --token → --api-key; send ApiKey-v1

BREAKING CHANGE: combine mcp-bridge renames the --token flag to
--api-key and the env var COMBINE_TOKEN to COMBINE_API_KEY. Sends
its value under the ApiKey-v1 Authorization scheme (was Bearer).
Required by passport's scheme-dispatch middleware. Internal
apiClient field renamed token → apiKey to make the contract
explicit (per TPM clarification 2026-04-19: agents and services
only ever send API keys outbound).

Updates docs/2026-04-05-mcp-bridge-design.md operator-facing
examples and env-var table to match. Operators consuming combine's
mcp-bridge must update their flags/env (see
~/Work/WorkFort/passport-scheme-split-deploy.md § Operator-visible
breaking changes).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Run the full test suite

**Files:** none modified.

**Step 1: Comprehensive verification**

```
cd /home/kazw/Work/WorkFort/combine/lead
go test ./...
go vet ./...
golangci-lint run
go build -o build/combine ./cmd/combine
```

Expected: all PASS, build OK, no lint regressions.

The e2e harness at `tests/e2e/harness/api_client.go:53` sends JWTs (constructed by `d.SignJWT(...)` in `APIClient`), so it stays on `Bearer`. No change. If any e2e test fails, that's a sign the test was unintentionally relying on the API-key fallthrough — investigate, do NOT silently switch the test to `ApiKey-v1` without confirming the test's intent.

**Step 2: If any test fails, stop and ask before changing anything.** A passing test that flips to failing on this migration is information, not noise.

---

### Task 5: Drop replace, bump dep, push

Stage individually (per CLAUDE.md guidance against `git commit -am` / `git add -A`):

```bash
# Remove replace from go.mod
go get github.com/Work-Fort/Passport/go/service-auth@<tag>
go mod tidy
go test ./...
git add go.mod go.sum
git commit -m "$(cat <<'EOF'
chore(deps): bump passport service-auth to <tag> (scheme dispatch)

Drops the local replace directive. Combine's middleware now uses
NewSchemeDispatch and the mcp-bridge sends API keys as ApiKey-v1.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Out-of-scope (recorded for future reference)

`internal/infra/gitutil/lfs_auth.go:78` signs an LFS-protocol JWT and includes it in a Git LFS `Authorization: Bearer <jwt>` header. This JWT does NOT travel through Passport's middleware — it's validated by Combine's own LFS handler. The `Bearer` is mandated by the Git LFS protocol spec and stays unchanged.

`internal/infra/httpapi/git.go:191` sets `WWW-Authenticate: Basic realm="Git" charset="UTF-8", Token, Bearer` on the Git smart-HTTP 401 path. This is the Git-protocol negotiation header (Basic for HTTP-Basic git, Token for personal-access-token git, Bearer for browser-routed JWT). It is intentionally **not** updated to advertise `ApiKey-v1`: Git smart-HTTP clients don't speak the API-key scheme, and adding it would mislead clients into trying a scheme that's not actually accepted on this code path. Pylon, by contrast, advertises `ApiKey-v1` on its 401s (per TPM directive, see pylon plan Task 3 Step 3) because Pylon's clients are programmatic and benefit from scheme discovery. The two endpoints have different audiences — leaving Combine's git path on `Basic, Token, Bearer` is deliberate.

## Verification checklist

- [ ] `go test ./...` PASS (all packages)
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean
- [ ] `cmd/mcpbridge/client.go` sends `ApiKey-v1`
- [ ] `internal/infra/httpapi/passport.go` uses `NewSchemeDispatch`
- [ ] `internal/infra/gitutil/lfs_auth.go:78` is **unchanged** (LFS protocol contract)
- [ ] `tests/e2e/harness/api_client.go:53` is **unchanged** (sends JWTs)
- [ ] No `replace` in `go.mod`
