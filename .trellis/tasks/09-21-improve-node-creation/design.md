# Technical Design

## Boundaries

The existing Server and Node model remains unchanged. The work touches the admin node API, protocol validation, the Vue node dialog, and the documented API contract. No database migration or Agent protocol change is required.

## Reality Generation Contract

Add an admin-only endpoint:

```text
POST /api/nodes/reality-keypair
```

The endpoint accepts no body and returns:

```json
{
  "private_key": "base64url-without-padding",
  "public_key": "base64url-without-padding",
  "short_id": "16-lowercase-hex-characters"
}
```

The Panel uses `crypto/ecdh.X25519().GenerateKey(rand.Reader)`. Both 32-byte keys are encoded with `base64.RawURLEncoding`; Short ID uses eight bytes from `crypto/rand` encoded with `hex.EncodeToString`. Generation does not write to the database or bump a server revision.

The route stays behind `requireAdmin`. Responses must not be logged. The generic node list/detail DTOs remain unchanged.

## Node Validation

Creation validation becomes aligned with renderer requirements:

- Shadowsocks requires a supported method. A server password remains optional because the renderer derives one after the node ID exists.
- VLESS requires a non-empty private key and at least one non-empty Server Name. Short ID, when supplied, must be an even-length hexadecimal string of at most 16 characters, matching Reality's 0-8 byte representation.
- Hysteria2 accepts optional non-negative integer bandwidth values. Its unused node password is no longer submitted by the UI.

Update settings are patches, not destructive replacements. Existing public settings are decoded and merged with supplied fields. Existing encrypted secrets are decrypted and merged only when a replacement secret is supplied. This preserves the current "leave blank to keep unchanged" UI contract and prevents changing a VLESS Short ID from erasing its private key.

Unknown fields remain rejected by each protocol allowlist. Existing stored settings remain readable; no migration is needed.

## Frontend Flow

`NodeFormDialog.vue` keeps one form but presents two visual sections: basic node information and protocol configuration. Switching protocols resets protocol-specific transient state and only the active protocol contributes to the request payload.

For VLESS creation/editing, an explicit generate action calls the new API and fills private key and Short ID. The returned public key is displayed with a copy action until the dialog closes or another keypair is generated. Existing stored private keys are never fetched.

Shadowsocks presents the supported cipher selector and explains that credentials are derived automatically. Hysteria2 presents only optional up/down bandwidth. Creation validation mirrors backend requirements and uses Chinese messages.

The layout uses existing CSS tokens and controls. Field rows collapse to one column at narrow widths so all controls remain usable on mobile.

## Compatibility

- Existing create/update endpoint paths and generic DTOs are preserved.
- Existing nodes do not require migration.
- The backend may continue accepting legacy optional Shadowsocks/Hysteria2 password fields, but the new UI does not request or submit them.
- Tightened VLESS creation validation intentionally rejects configurations that would currently save successfully but fail during Agent rendering.

## Security

- Randomness comes from `crypto/rand` only.
- Generated private keys are returned only to an authenticated administrator and are not persisted by the generation endpoint.
- Node persistence continues encrypting private keys with the configured application key.
- Tests assert that node responses never contain generated private keys.

## Rollback

The generation endpoint and frontend action can be removed without data rollback. Validation and merge changes are isolated to node settings handling; reverting them restores prior request behavior. No schema rollback is necessary.
