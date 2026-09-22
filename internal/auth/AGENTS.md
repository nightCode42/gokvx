# internal/auth — Agent Notes

The JWKS cache, the JWT verifier, and the scope authorizer. This package is the security boundary of gokvx. Read spec §11 and [system-invariants §11](../../docs/engineering/system-invariants.md#11-security) before editing.

## Invariants

- Tokens are verified locally; no network call to the issuer happens on the request path (`KV-SEC-011`, C-3).
- The JWKS refreshes in the background. A failed refresh keeps the last good key set and raises a metric and `WARN` (`KV-SEC-012`).
- An unknown `kid` triggers at most one refresh, rate-limited to the configured minimum interval (`KV-SEC-013`).
- The accepted algorithms come from configuration — never from the token header. `alg: none` and anything outside the set are rejected before any other processing (`KV-SEC-014`).
- `iss`, `aud`, `exp`, `nbf`, `iat` are validated with the configured skew; tokens without `exp` or `kid` are rejected (`KV-SEC-015`–`016`).
- Authorization is deny-by-default through the method-to-scope table; `kv:write` never implies `kv:read` (`KV-SEC-020`–`023`).
- Failures return generic messages and fixed reasons; which check failed is logged, never returned (`KV-SEC-030`).
- Tokens, signatures, and key material never reach logs, traces, labels, or errors (`KV-SEC-031`).
- This package never references GoAuthx (C-1).

## Stop and ask before

- Relaxing any validation, default, limit, or rate limit.
- Adding an accepted algorithm, credential type, or scope.

## Required tests

- The hostile-token corpus in `QA-008`: `alg: none`, algorithm confusion, missing and unknown `kid`, expired, not-yet-valid, wrong issuer, wrong audience, tampered payload, oversized token.
- The full authorization matrix: every method against every scope combination (`QA-007`).
- JWKS refresh success, failure, and rate limiting with a fake clock and a stub issuer (`KV-SEC-050`).
- Fuzzing of token parsing (`QA-005`).
