# Implementation Plan

## Ordered Work

1. Bootstrap Go Panel and Vue 3 frontend with SQLite, migrations, configuration loading, health endpoint and local commands.
2. Implement repositories and migrations for Admin, User, Server, Agent, Node, UserNode, Session, ConnectionLog and TrafficRecord with ownership/unique constraints.
3. Implement admin authentication, session middleware, DTOs, pagination, filtering and validation.
4. Implement user and Server/Node CRUD, authorization, status/traffic/expiry actions and detail APIs.
5. Implement login, dashboard, user list/detail, Server list/detail and settings pages, prioritizing User Detail.
6. Implement Agent registration, token hashing/rotation, Bearer authentication and Server-scoped authorization.
7. Implement heartbeat and configurable online/offline calculation.
8. Implement versioned sing-box configuration rendering for the three protocols and Agent application rollback.
9. Verify pinned sing-box stats/log sources and implement Agent runtime adapters and integration tests.
10. Implement idempotent traffic ingestion, current-session snapshots, connection-log ingestion, retention cleanup and query APIs.
11. Add Linux systemd packaging, sample configuration and one-agent canary instructions.
12. Review every PRD acceptance criterion and correct missing contracts/documentation.

## Validation Commands

The repository has no application toolchain yet; establish these commands during bootstrap:

```text
go test ./...
go test -race ./...
go vet ./...
go build ./...
go test ./... -tags integration
npm ci
npm run typecheck
npm run build
```

Focused tests must cover status/expiry/quota filtering, ownership/token rotation, revision rollback, duplicate batch idempotency, session expiry, retention cleanup, auth separation and protocol rendering.

## Review Gates

- Test Server/Node/Agent ownership before Agent integration.
- Do not claim traffic or connection-log support until pinned sing-box sources are verified.
- Never expose protocol secrets in generic models, logs, errors or list DTOs.
- Heartbeat is the only liveness source; traffic/session reports cannot mark a Server online.
- Agent identifiers must be checked against its bound Server.
- Verify User Detail end to end before dashboard polish.

## Rollback Points

- Keep the Server/Node schema migration reversible in development.
- Keep dry-run config rendering before enabling runtime writes.
- Gate telemetry total mutation until idempotency tests pass.
- Canary one Agent and verify old configuration survives Panel interruption.

## Deferred Follow-Ups

- WebSocket/server-push commands, Snell, subscriptions, user portal, plans, payments, orders, destination host/port logs, multi-admin RBAC, multi-tenancy and notifications.
