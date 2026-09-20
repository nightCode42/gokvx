# ============================================================
# gokvx — Makefile
# ============================================================
# Usage: make <target>
# Run `make help` to list all available targets.

GO            := go
GOFLAGS       := -trimpath
BIN_DIR       := bin
CMD_PATH      := ./cmd/...
COVERAGE_OUT  := coverage.out
COVERAGE_HTML := coverage.html

# Pinned tool versions — bump deliberately, together with CI.
GOLANGCI_LINT_VERSION := v2.13.2
GOVULNCHECK_VERSION   := v1.8.0
GITLEAKS_VERSION      := v8.30.1

# Tools are invoked from GOPATH/bin so the pinned versions win over any on PATH.
TOOLS_BIN     := $$($(GO) env GOPATH)/bin
GOLANGCI_LINT := "$(TOOLS_BIN)/golangci-lint"
GOVULNCHECK   := "$(TOOLS_BIN)/govulncheck"
GITLEAKS      := "$(TOOLS_BIN)/gitleaks"

# Build info injected at compile time
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT        ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS       := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

.DEFAULT_GOAL := help
.PHONY: help setup hooks-install hooks-uninstall \
        build run clean \
        test test-race coverage \
        fmt fmt-check lint tidy tidy-check vuln secrets check

# ── Help ────────────────────────────────────────────────────────────────────
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── Setup ───────────────────────────────────────────────────────────────────
setup: ## Install pinned dev tools and git hooks (run once after cloning)
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@mkdir -p "$(TOOLS_BIN)"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
		| sh -s -- -b "$(TOOLS_BIN)" $(GOLANGCI_LINT_VERSION)
	@echo "Installing govulncheck $(GOVULNCHECK_VERSION)..."
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@echo "Installing gitleaks $(GITLEAKS_VERSION)..."
	$(GO) install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)
	@echo ""
	@echo "Installing pre-commit..."
	@if command -v pre-commit >/dev/null 2>&1; then \
		echo "- pre-commit already installed"; \
	elif command -v brew >/dev/null 2>&1; then \
		NONINTERACTIVE=1 brew install pre-commit; \
	elif command -v pipx >/dev/null 2>&1; then \
		pipx install pre-commit; \
	elif command -v pip3 >/dev/null 2>&1; then \
		pip3 install --user pre-commit; \
	elif command -v pip >/dev/null 2>&1; then \
		pip install --user pre-commit; \
	else \
		echo "ERROR: pre-commit not found. Install manually: https://pre-commit.com/#install"; \
		exit 1; \
	fi
	@echo ""
	@if git rev-parse --git-dir >/dev/null 2>&1; then \
		"$(MAKE)" hooks-install; \
		echo ""; \
		echo "- Setup complete: tools installed, git hooks registered"; \
	else \
		echo "! Not a git repository - skipping git hooks."; \
		echo "  Run 'git init -b main' and then 'make hooks-install'."; \
		echo ""; \
		echo "- Setup complete: tools installed"; \
	fi

hooks-install: ## Register git hooks (pre-commit, commit-msg, pre-push)
	pre-commit install --hook-type pre-commit --hook-type commit-msg --hook-type pre-push
	@echo "- Git hooks installed"

hooks-uninstall: ## Remove git hooks
	pre-commit uninstall --hook-type pre-commit --hook-type commit-msg --hook-type pre-push
	@echo "- Git hooks removed"

# ── Build ───────────────────────────────────────────────────────────────────
build: ## Build all binaries into bin/ with version info
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/ $(CMD_PATH)

run: ## Run the gokvx server locally
	$(GO) run ./cmd/gokvx

clean: ## Remove build and coverage output
	rm -rf $(BIN_DIR) $(COVERAGE_OUT) $(COVERAGE_HTML)

# ── Test ────────────────────────────────────────────────────────────────────
test: ## Run unit tests
	$(GO) test ./...

test-race: ## Run unit tests with the race detector (requires cgo)
	CGO_ENABLED=1 $(GO) test -race ./...

coverage: ## Run tests with coverage and write an HTML report
	$(GO) test -covermode=atomic -coverprofile=$(COVERAGE_OUT) ./...
	$(GO) tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@$(GO) tool cover -func=$(COVERAGE_OUT) | tail -1

# ── Quality ─────────────────────────────────────────────────────────────────
fmt: ## Format code (gofumpt + goimports via golangci-lint)
	$(GOLANGCI_LINT) fmt

fmt-check: ## Fail if any file is not formatted
	$(GOLANGCI_LINT) fmt --diff

lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run

tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

tidy-check: ## Fail if go.mod or go.sum is not tidy
	$(GO) mod tidy -diff

vuln: ## Scan dependencies for known vulnerabilities
	$(GOVULNCHECK) ./...

secrets: ## Scan the full git history for secrets
	$(GITLEAKS) git --config=.gitleaks.toml --redact --verbose

check: fmt-check lint tidy-check test vuln ## Run every local quality gate (what CI runs)
	@echo "- All checks passed"
