# Workflow

How a change moves from an idea to `main`. The rules here apply to every contributor, human or AI agent.

---

## 1. Roles

| Role | Responsibilities |
|---|---|
| **Maintainer** | Decides scope and priorities, resolves spec questions, reviews, commits, pushes, merges, and tags releases. |
| **Contributor or AI agent** | Implements a scoped change, runs the checks, updates documentation and the work log, and drafts the commit message and PR description. Commits or pushes only when the maintainer explicitly asks. |

## 2. Planning a change

- One branch implements **one slice of requirements** — small enough to review in one sitting, complete enough to be tested and useful on its own.
- Before writing code, identify the requirement IDs the slice covers and read their spec sections and the relevant handbook documents (see the routing table in `AGENTS.md` §4).
- If the slice needs a decision the spec does not make, record it as an ADR in the same PR — or stop and ask when it touches the triggers in `AGENTS.md` §7.
- Order of work within a slice: types and interfaces → tests → implementation → documentation. Tests for a behaviour exist before the behaviour is called done.

## 3. Branches

- Created from an up-to-date `main`: `git switch main && git pull && git switch -c <branch>`.
- Named `<type>/<short-kebab-description>`, using the Conventional Commits types: `feat/command-log`, `fix/jwks-refresh-backoff`, `docs/error-catalogue`, `test/mvcc-properties`, `ci/buf-breaking`, `chore/deps-grouping`.
- Short-lived: merged or closed within days, not weeks.
- `main` is protected: changes arrive only through pull requests with passing checks and linear history.

## 4. Commits

- Format: [Conventional Commits](https://www.conventionalcommits.org/), enforced by the `commit-msg` hook and in CI.

```text
<type>(<scope>): <description>

<optional body: what and why, wrapped at 72 characters>

<optional footer: Refs: KV-STO-002, KV-STO-004>
```

- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`. Scopes are package or area names: `storage`, `mvcc`, `auth`, `raft`, `shard`, `server`, `ms1`, `proto`, `deps`.
- The header is at most 72 characters, in the imperative mood ("add", not "added"), and does not end with a period.
- A breaking change to a public interface uses `!` (`feat(api)!: …`) and explains the migration in the body. After the Phase 1 tag, a breaking wire-contract change is not permitted at all.
- The body explains *why*; the diff already shows *what*.
- Commits are signed and use the author's GitHub no-reply address.

## 5. Pull requests

- The PR title is a valid Conventional Commits header — it becomes the single commit on `main` when squash-merged.
- The description follows the [pull request template](../../.github/pull_request_template.md): what, why, how it was verified, and the checklist.
- Target size: under ~400 changed lines excluding generated code and test data. A larger change is split into a sequence of PRs that each leave `main` working.
- All required checks pass: Lint, Test, Build, Vulnerability scan, Secret scan, and Commit messages.
- Merged with **squash merge** only; the branch is deleted automatically.

## 6. Definition of done

A change is done when **all** of the following hold:

- [ ] `make check` passes locally; CI passes on the pull request.
- [ ] New behaviour is covered by tests that name their requirement IDs (`QA-071`).
- [ ] The `Status` of every implemented requirement is updated in `docs/requirements.md` in the same PR (`QA-072`).
- [ ] Doc comments, package documentation, and the handbook are updated where behaviour or rules changed.
- [ ] No document contradicts the code: any divergence was presented to the maintainer (`AGENTS.md` §3) and every document restating the affected rule is updated in this PR.
- [ ] A new design decision is recorded as an ADR.
- [ ] `docs/WORKLOG.md` reflects the new state and the next step.
- [ ] No `TODO` without an issue, no commented-out code, no debug output, no unrelated changes.

At the end of a phase, the phase-level Definition of Done in spec §4.2 applies as well.

## 7. Local tooling

The `Makefile` is the only entry point for development tasks; `make help` lists them.

| Command | When |
|---|---|
| `make setup` | Once after cloning: installs pinned tools and git hooks |
| `make check` | Before every push: format, lint, tidiness, tests, vulnerability scan |
| `make test` | While developing |
| `make coverage` | To inspect coverage locally |
| `make secrets` | To scan the full history for secrets |

Git hooks (installed by `make setup`):

| Stage | Checks |
|---|---|
| `pre-commit` | File hygiene, private keys, secrets (gitleaks), formatting, lint, `go mod tidy`, no commits to `main` |
| `commit-msg` | Conventional Commits |
| `pre-push` | Unit tests, vulnerability scan |

Hooks are never bypassed with `--no-verify`. A hook that is wrong is fixed in its own pull request.

**Tool versions** are pinned in the `Makefile` and in `.github/workflows/ci.yml`. They are always changed together, in one PR.

## 8. Releases

- Versions follow [Semantic Versioning](https://semver.org/) and are driven by Conventional Commits (`QA-045`).
- Each delivery phase ends with a tagged release once its Definition of Done (spec §4.2) is met. The Phase 1 tag freezes the wire contract (`KV-API-000`).
- Tags are created by the maintainer only.

## 9. Working on Windows

The maintainer's environment is Windows with Git Bash; CI runs on Linux.

- The repository stores LF line endings only (`.gitattributes`); editors follow `.editorconfig`.
- Git Bash's `sed -i` rewrites files with CRLF and can leave temporary `sed*` files. Prefer an editor; when `sed -i` is used, check the result with `file <path>` and clean up afterwards.
- The race detector needs cgo, which is not set up locally. `make test` runs without `-race`; CI runs the race-enabled suite.
- Shell scripts must be committed as executable: `git update-index --chmod=+x <script>`.
