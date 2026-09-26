# AGENTS.md — Working Agreement for gokvx

This file is the binding working agreement for every contributor — human or AI agent — on this repository. It is loaded at the start of every session and takes precedence over habits, defaults, and memory of earlier conversations. Details live in the [engineering handbook](docs/engineering/README.md); this file holds the rules and tells you where to look.

---

## 1. What this project is

**gokvx is a correctness-first, strongly consistent (CP), sharded key-value store written in Go. It is designed to be read end to end: every design decision is documented, and every consistency claim is backed by tests.** It exposes a gRPC-only API, replicates state with Raft (`etcd-io/raft`), partitions the key space into fixed slots, and models data with MVCC revisions and streaming watches.

The project is judged on two things: whether its claims are **provably true**, and whether an experienced engineer can **read it end to end and understand why it is built the way it is**. Every change must preserve, in this order of priority:

1. **Correctness** — linearizability, durability, and the invariants in [system-invariants.md](docs/engineering/system-invariants.md). A correctness claim without a test that verifies it is a defect.
2. **Security** — mutual TLS plus scoped JWT authorization on every request; secure by default, unsafe only by explicit, loud configuration.
3. **Clarity** — code a reviewer can follow without asking the author. Clever but opaque code is a defect.
4. **Operability** — health, metrics, traces, and logs are features, not afterthoughts.
5. **Performance** — measured, never assumed, and never bought at the expense of 1–4.

The repository contains two services and one external contract:

| Component | Role |
|---|---|
| `gokvx` | The key-value store. gRPC only. |
| `microservice-1` | A deliberately thin REST consumer demonstrating secure service-to-service integration. No business logic, no datastore, one gokvx RPC per REST request. |
| Identity provider | External. gokvx trusts only the issuer and JWKS URL set in its configuration and has no dependency on any specific provider. |

Full product context, goals, and non-goals: [product-context.md](docs/engineering/product-context.md).

---

## 2. Session protocol — surviving context loss

Context is finite. Rules, specification details, and project state must always be **re-read from this repository**, never reconstructed from memory of an earlier conversation or a summary.

**At the start of every session, and after any context compaction or resumption:**

1. Re-read this file.
2. Read [docs/WORKLOG.md](docs/WORKLOG.md) for the current focus, active branch, and hand-off notes.
3. Run `git status` and `git branch --show-current`, and confirm they match the work log.
4. Before touching any area, read the documents listed for it in the routing table (§4).

**During work:** if you are not certain you have read a rule or a spec section *in this session*, read it again. Guessing is not permitted.

**At the end of every task:**

1. Update [docs/WORKLOG.md](docs/WORKLOG.md): what changed, what was verified, what is next, and any open question.
2. Report to the maintainer using the format in §9.

---

## 3. Sources of truth

When sources disagree, the higher one wins. If the conflict is real rather than a stale document, **stop and ask** — never resolve it silently.

1. [docs/requirements.md](docs/requirements.md) — the specification (`SRS-GOKVX-001`). Requirement IDs (e.g. `KV-STO-004`) are the vocabulary of this project.
2. [docs/adr/](docs/adr/README.md) — accepted Architecture Decision Records.
3. This file and the [engineering handbook](docs/engineering/README.md).
4. Existing code. Code that contradicts 1–3 is a bug, not a precedent.

Per-requirement implementation status lives **only** in the `Status` column of `docs/requirements.md` (§3.3 of the spec). No other file restates it.

### Keeping documents and code in agreement

Documents and code must never drift apart. When an implementation cannot, or should not, follow a document exactly — the spec, an ADR, the handbook, a package `AGENTS.md`, or a doc comment — **stop and present the divergence to the maintainer before continuing**:

1. **The conflict** — the document, section or requirement ID, what it says, and what the implementation needs instead.
2. **Option A: update the document** — the exact wording change and why the implementation is right.
3. **Option B: change the implementation** — what following the document costs.
4. **Recommendation** — which option, and the trade-off.

The maintainer decides. The chosen document change lands in the **same pull request** as the code, and every other document that restates the same rule is updated with it, so no stale copy remains. A spec change that alters a `MUST` requirement also gets an ADR (spec, Document Control).

---

## 4. Routing table — read before you start

| If the task touches… | Read first |
|---|---|
| Anything at all | This file, `docs/WORKLOG.md` |
| Go code in general | [go-standards.md](docs/engineering/go-standards.md) |
| Errors, status codes, HTTP mapping | [error-handling.md](docs/engineering/error-handling.md), spec Appendix B |
| Tests of any kind | [testing.md](docs/engineering/testing.md), spec §18 |
| Adding or upgrading a dependency | [dependencies.md](docs/engineering/dependencies.md) |
| Git, branches, commits, PRs | [workflow.md](docs/engineering/workflow.md) |
| `proto/` or the wire contract | spec §4.3, §8, Appendix A; system-invariants §Proto contract |
| `internal/storage` | `internal/storage/AGENTS.md`, spec §9 |
| `internal/mvcc` | `internal/mvcc/AGENTS.md`, spec §7.1 |
| `internal/consensus` | `internal/consensus/AGENTS.md`, spec §10.1–10.2 |
| `internal/shard` | `internal/shard/AGENTS.md`, spec §7.3, §10.3 |
| `internal/auth`, TLS, scopes | `internal/auth/AGENTS.md`, spec §11 |
| `internal/watch` | spec §8.5; system-invariants §Watch |
| Observability | spec §12, Appendix C |
| Configuration and lifecycle | spec §13, Appendix D, [docs/configuration.md](docs/configuration.md) |
| `microservice-1` | spec §14–15, Appendix B.2 |
| Deployment | spec §16–17 |
| A design choice not covered above | `docs/adr/`, then ask |

---

## 5. Hard rules

These are not guidelines. A change that breaks one is not mergeable.

### Architecture

- All state mutations are serialized commands applied through the `CommandLog` → apply loop → `StateMachine` path (`KV-STO-000`). Nothing bypasses it.
- Layering rules A-1 to A-5 (spec §6.2) are enforced: the service layer never touches the storage engine; the replication layer never imports the transport layer; only the transport layer converts errors to gRPC status.
- The state machine is deterministic. No wall-clock time, randomness, map iteration order, or hostnames influence applied state (A-2).
- `gokvx` never references GoAuthx in code, configuration defaults, or dependencies (C-1).
- Stay inside the current phase. Later-phase fields return `UNIMPLEMENTED` with reason `FEATURE_NOT_IN_CURRENT_PHASE`; they are never half-implemented.

### Errors

- One error type across the system: `*kverr.Error`, carrying a stable `Reason`. Functions return the `error` interface — never a concrete error pointer.
- Wrap with `fmt.Errorf("pkg.Func: %w", err)` when crossing a boundary. Never compare error strings. Log an error once, at the edge — never log and return.
- Full model: [error-handling.md](docs/engineering/error-handling.md).

### Go code

- `context.Context` is the first parameter of every function that does I/O, blocks, or crosses a package boundary, and deadlines are honoured.
- Every goroutine has an owner, a shutdown path, and a test proving it does not leak.
- Time and randomness are injected. Tests never use `time.Sleep` for correctness.
- Every function and method carries a doc comment stating what it does; every package has a `doc.go`. See [go-standards.md §Comments](docs/engineering/go-standards.md#4-comments-and-documentation).
- Size and complexity limits in go-standards.md §Size are linted and respected.

### Security

- Never log, trace, label, or return keys, values, tokens, signatures, or private key material.
- The JWT algorithm is validated against configuration, never taken from the token header.
- Authorization is deny-by-default through the method-to-scope table. `kv:write` never implies `kv:read`.
- Unsafe settings (`auth.mode: disabled`, `tls.enabled: false`, relaxed `fsync`) are never defaults and always produce a distinct startup warning.

### Wire contract

- After the Phase 1 tag, no field is added, removed, renumbered, or changed in meaning. Removed fields are `reserved`. Enum zero values are `*_UNSPECIFIED`. Mutually exclusive fields use `oneof`.
- Generated code is committed and never edited by hand.

### Tests

- Every `MUST` requirement is verified by at least one automated test that names its requirement ID (`QA-070`, `QA-071`).
- Tests are hermetic, deterministic, and pass with `-race`. A flaky test is quarantined with an issue link, never retried into passing.
- Never delete, skip, or weaken a test to make a change pass.

### Dependencies

- Only modules on the allowlist in [dependencies.md](docs/engineering/dependencies.md) may be imported. Anything else requires an accepted ADR first.

---

## 6. Workflow

- **The maintainer commits, pushes, merges, and tags.** Agents prepare changes, run the checks, and draft commit messages and PR descriptions. An agent commits or pushes only when the maintainer explicitly asks in the current session.
- One branch per requirement slice, named `<type>/<short-kebab-description>` (e.g. `feat/command-log`). Never work on `main`; it is protected.
- Commits and PR titles follow Conventional Commits. PRs are squash-merged, so the PR title becomes the commit on `main`.
- Stay in scope: no speculative features, no drive-by refactors, no edits to unrelated files. Note out-of-scope findings in the work log instead.
- Every task ends with `make check` passing. Details: [workflow.md](docs/engineering/workflow.md).

---

## 7. Stop and ask the maintainer before

- Changing anything in `proto/` once the Phase 1 tag exists.
- Changing the slot function, slot count semantics, or any on-disk format (log records, snapshots, `storage_version`).
- Adding, removing, or upgrading a dependency beyond a patch release.
- Weakening any security control, default, or test.
- Deviating from, or reinterpreting, a requirement or any other document — including when the document looks wrong. Present it as described in §3, "Keeping documents and code in agreement".
- Resolving an ambiguity in the spec or a conflict between sources of truth.
- Deleting tests, data, or history, or rewriting published commits.

When you stop, state the question, the options, and your recommendation with its trade-off.

---

## 8. Never

- Claim that something builds, passes, or works without having run it in this session.
- Invent a library API. Verify signatures against the module source or documentation — `etcd-io/raft`'s `Ready` handling in particular.
- Leave `TODO`, `FIXME`, placeholders, or commented-out code. A deferred item is tracked as an issue and referenced as `// TODO(#<issue>): …`.
- Introduce global mutable state, `init()` side effects beyond registration, or panics across package boundaries.
- Skip git hooks (`--no-verify`) or bypass CI.
- Commit secrets, credentials, certificates, or `.env` files.

---

## 9. Reporting format

End every task with:

1. **Changed** — files and a one-line summary of each.
2. **Requirements** — IDs implemented or affected, with their new `Status`.
3. **Verified** — the exact commands run and their result. Anything not run is listed as not verified.
4. **Open** — questions, risks, and follow-ups.
5. **Suggested commit message** — Conventional Commits format.

---

## 10. Environment notes

- The maintainer develops on Windows with Git Bash; CI runs on Linux. The repository uses LF line endings only (`.gitattributes`).
- Git Bash's `sed -i` rewrites files with CRLF and can leave temporary `sed*` files behind. Prefer editor tools; if `sed -i` is used, verify line endings and clean up.
- `-race` requires cgo, which is unavailable locally; race-enabled tests run in CI (`make test-race` on Linux).
- `make help` lists every task. The Makefile is the single entry point; tool versions are pinned there and in CI together.

---

## 11. Repository map

| Path | Contents |
|---|---|
| `cmd/` | Binaries: `gokvx`, `gokvxctl`, `microservice1`, `kvbench`, `kvcheck` |
| `internal/` | Implementation packages; critical ones carry their own `AGENTS.md` |
| `pkg/client/` | Public Go client SDK |
| `proto/gokvx/v1/` | The wire contract |
| `test/` | Integration, end-to-end, chaos tests, and the stub issuer |
| `deploy/` | Compose, Helm, Terraform, kind, observability configuration |
| `docs/requirements.md` | The specification |
| `docs/engineering/` | The engineering handbook |
| `docs/adr/` | Architecture Decision Records |
| `docs/WORKLOG.md` | Current focus and hand-off notes |
