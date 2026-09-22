# Work Log

Current focus, hand-off notes, and open decisions. This file lets any contributor — or an AI agent starting a fresh session — pick up the work exactly where it stopped.

Per-requirement implementation status is **not** tracked here; it lives only in the `Status` column of [requirements.md](requirements.md) (spec §3.3).

---

## Current focus

| Item | Value |
|---|---|
| Phase | Phase 1 — Durable core |
| Active branch | `docs/engineering-guidelines` |
| Active work | Working agreement (`AGENTS.md`) and engineering handbook |
| Requirement IDs | — (governance; no spec requirement) |

## Next up

In order. Each item is one branch and one pull request.

1. **Wire contract** — Protocol Buffer definitions from Appendix A, `buf` lint and breaking-change checks in the Makefile and CI, generated code committed. `KV-API-000`, `KV-API-002`, `KV-API-003`, `KV-API-080`–`082`.
2. **Error model** — `internal/kverr` with the reason registry, and the public reason catalogue in `docs/errors.md`. `KV-API-090`–`092`, layering rule A-5.
3. **Slot function** — `internal/shard` slot computation with golden tests. `KV-DAT-020`–`022`, `QA-006`.

## Open decisions

| Decision | Owner | Notes |
|---|---|---|
| GoAuthx link in README | Maintainer | Link currently returns 404; publish GoAuthx or remove the link before sharing the repository. |

## Hand-off notes

- 2026-09-22 — Repository published. CI, pre-commit hooks, branch ruleset on `main`, CodeQL, Dependabot, and private vulnerability reporting are active. Working agreement and handbook drafted on `docs/engineering-guidelines`.
