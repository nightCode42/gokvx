# Contributing

Issues and suggestions are welcome.

## Setup

```bash
make setup   # install pinned tools (golangci-lint, govulncheck, gitleaks) and git hooks
make check   # run all quality gates locally
```

## Workflow

`main` is protected: all changes land through pull requests with passing CI. Commits directly to `main` are also blocked locally by a pre-commit hook.

```bash
git switch -c feat/command-log
# ...work, commit...
git push -u origin feat/command-log
```

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org/). The `commit-msg` hook and CI enforce this.

```text
feat(storage): add segmented command log
fix(auth): reject tokens without a kid header
docs: describe fsync policy trade-offs
```
