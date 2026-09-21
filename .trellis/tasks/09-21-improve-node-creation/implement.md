# Implementation Plan

## 1. API Contract And Generation

- Document `POST /api/nodes/reality-keypair` and its authenticated, non-persistent response in `docs/api-contract.md`.
- Add the response type and typed API function under `web/src/api`.
- Register the admin route and implement X25519 keypair plus eight-byte Short ID generation using Go standard library primitives.
- Add API tests for authentication, encoding, keypair correspondence, Short ID shape, and absence of persistence/revision changes.

## 2. Settings Validation And Merge Semantics

- Refactor node settings handling so create validates complete protocol requirements while update merges supplied public and secret fields with existing values.
- Enforce supported Shadowsocks method, required VLESS private key and Server Name, valid Reality Short ID, and non-negative integer Hysteria2 bandwidth.
- Preserve allowlist rejection and encrypted-at-rest handling.
- Add regression tests for invalid-but-previously-storable VLESS nodes and partial updates retaining the existing private key/settings.

## 3. Node Form UX

- Reorganize `NodeFormDialog.vue` into basic and protocol sections using existing visual tokens.
- Remove Shadowsocks and Hysteria2 password inputs from the current UI; explain derived/user credentials where relevant.
- Add explicit Reality generation, public-key copy display, complete client-side validation, and protocol-state reset behavior.
- Add responsive styling for narrow screens.

## 4. Integration Review

- Verify create and edit payloads contain only active-protocol fields.
- Verify a generated Reality private key reaches rendered sing-box configuration while node API responses expose neither private key nor stored secrets.
- Verify successful node mutations still bump only the owning Server revision and generation does not bump it.
- Review contract, Go DTOs, and TypeScript types together for drift.

## Validation

```bash
gofmt -w internal/web/nodes.go internal/web/servers_nodes_test.go
go build ./...
go vet ./...
gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=1 -tags integration ./...
go build -tags embed_ui ./...
cd web && npm run typecheck
cd web && npm run build
cd web && npm run lint
make build
```

## Risk And Rollback Points

- X25519 encoding compatibility is a hard gate: verify generated values by reconstructing the public key from the returned private key in tests.
- Settings merge behavior touches all three protocols: run focused node CRUD and Agent render tests before the full suite.
- Do not add a database column for the public key; stop and revisit design if implementation discovers a current consumer that requires persistent public-key retrieval.
