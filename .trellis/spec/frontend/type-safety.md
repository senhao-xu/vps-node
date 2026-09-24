# Type Safety

> Type safety patterns in this project.

---

## Overview

TypeScript strict with `noUnusedLocals` / `noUnusedParameters`. `any` is forbidden; `unknown` + type guards at boundaries.

---

## Type Organization

- All backend payload types live in **one** file: `web/src/api/types.ts` — a hand mirror of `docs/api-contract.md`. When the contract changes, update types.ts + Go DTOs together in the same task.
- Components accept typed props; `DataTable<T extends Record<string, unknown>>` uses typed slots so pages write zero casts.

## Validation

- Runtime validation happens ONLY at the boundary (`http.ts` decodes error envelope via type guards; success payloads are trusted per contract — backend is the validator).
- Error envelope: `{"error":{code,message}}` → `ApiError` with `errorMessage()` for UI display; validation codes surface field messages from the backend.

---

## Common Patterns

```ts
const err = asApiError(payload); // type guard over unknown
if (err) throw new ApiError(err.error.code, err.error.message);
```

- Secrets: user tokens use `OneTimeSecret.vue` — plaintext shown exactly once after create/reset/rotate. The per-server **agent key** is different: it is auto-issued on `POST /api/servers` (201 carries `agent_key`, typed as `CreateServerResult = Server & { agent_key: string }`), retrievable at any time (`GET /api/servers/:id/agent-key`, decrypted from `key_enc`) and reset via `POST`, so the Server detail page shows/copies it directly. List/detail DTOs never contain secrets (`key_hash`/`key_enc` are excluded by the backend sweep test).
- Derived state: user `expired` display state is derived from `expires_at` client-side (`displayUserStatus`); stored `status` filter uses backend values.

---

## Forbidden Patterns

- `any`, non-null `!` assertions on API data, `as X` casts of `unknown` payloads without a guard.
- Local type re-declarations of API objects in pages/components (import from `api/types.ts`).
- `fetch` outside `web/src/api/http.ts`.
