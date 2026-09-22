# Quality Guidelines

> Code standards, forbidden patterns, and verification commands.

---

## Code Standards

- Go 1.25, stdlib-first (`net/http`, `database/sql`, `slog`, `encoding/json`). No web framework, no ORM.
- **No comments in code** unless truly necessary — code must be self-explanatory.
- Agent payloads are decoded with `decodeJSON` (unknown fields ignored, per the binding `docs/api-contract.md`); undeclared fields must never be trusted for ownership or telemetry totals.
- Allowlists for anything admin-configurable that reaches runtime: node protocol settings are validated field-by-field per protocol; sing-box values are structured; no paths, executables, or shell from Admin API. The agent embeds sing-box as a Go library — it never execs an external binary, renders a config file, or runs a reload command.
- Context-aware repos; `Tx` helper for multi-statement invariants.

## Forbidden Patterns

- Storing or logging plaintext agent/user tokens, protocol secrets, or rendered sing-box configs.
- Trusting `server_id`/`node_id` ownership from request bodies on agent routes.
- Writing telemetry totals outside the batch-marker transaction (double counting on retry).
- Marking a server online from anything except heartbeat (`last_seen_at` is the only liveness source; traffic/device reports must not flip status).
- `any` in TypeScript, raw `fetch` in components, hand-cast JSON outside `web/src/api`.

## Review Gates

- Never expose protocol secrets in generic models, logs, errors, or list DTOs (there is an automated JSON-sweep test).
- Traffic measurement is verified in-process: `internal/kernel/singbox` counts real TCP/UDP payload bytes per user through the embedded sing-box `ConnectionTracker`, and integration tests must parse + start the Panel-rendered config against the pinned embedded sing-box (`make test-integration`) — no external binary or host prerequisite.
- Batch semantics: duplicate `batch_seq` returns the prior result without double counting — verified by e2e for both traffic (`traffic_batches`) and devices (`device_batches`).

## Validation Commands (must pass before reporting done)

```bash
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=1 -tags integration,with_quic,with_utls ./...
go build -tags with_quic,with_utls ./...
go build -tags embed_ui ./...
cd web && npm run typecheck && npm run build && npm run lint
make build
```

---

## Design Decisions

### Decision: contract-first cross-layer development
**Context**: Go panel, Vue frontend, and Go agent are built by separate agents/sessions.
**Decision**: `docs/api-contract.md` is the frozen binding contract; implementers read it and report defects rather than editing it; frontend types live only in `web/src/api/types.ts`; agent payloads only in `internal/agentclient`.
**Consequence**: cross-layer drift is caught by sampling flows against the contract; contract edits are explicit, minimal diffs listed in the task report.

### Decision: eligibility computed, not stored
**Context**: users must sync to agents only when `active` + unexpired + under quota.
**Decision**: `ListEligibleUsersByServer` computes this in SQL at read time; `expired` is never a stored status.
**Consequence**: no background job to flip states; tests cover each predicate (`transfer_enable=0` = unlimited).
