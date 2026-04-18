# Flow Integration

Combine's component-level work for the agent pool design. The
cross-cutting design lives in
`flow/lead/docs/2026-04-18-agent-pool.md`.

Combine's role in the agent pool design is small and mostly
operational — almost no new code.

## Operational setup

### Service token for Flow

Flow needs a Combine service token with permissions to:

- Approve pull requests
- Merge pull requests
- Create branches (for new work items)

Issued via Passport as a service-type token. Flow stores it as
configuration; agents do not have access to it.

### Webhooks to Flow

Combine's existing webhook infrastructure
(`internal/infra/httpapi/api_webhooks.go`) is used to notify Flow
on:

- `push` events to any branch — Flow treats most of these as no-ops
  but uses pushes to feature branches to know when an agent has
  delivered work.
- `pull_request.merged` events — Flow uses these to refresh the
  affected project's source master.

Webhook configuration is operator-manual today (Virgil-managed
later). One webhook registration per project, pointing at Flow's
endpoint.

## Code changes

None expected. Existing webhook + API surface should be sufficient.

If gaps surface during Flow integration (e.g. no `merged` event
type, or insufficient payload detail), file follow-ups against this
doc.

## Out of scope

- No agent ever receives a Combine write token. All
  approve/merge actions go through Flow.
- Per-agent access controls within Combine: agents pull and push
  branches but only Flow can merge to main. Whether this is enforced
  by token scoping or by branch protection is a Combine
  configuration choice, not part of this design.
