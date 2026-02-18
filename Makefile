SHELL := /bin/bash
# Source the user's shell profile so env vars (API keys, PATH, etc.) are available.
# Tries .bashrc first (Linux default), then .zshrc (macOS default), then .profile (POSIX fallback).
SOURCE_ENV := if [ -f "$$HOME/.bashrc" ]; then . "$$HOME/.bashrc"; elif [ -f "$$HOME/.zshrc" ]; then . "$$HOME/.zshrc"; elif [ -f "$$HOME/.profile" ]; then . "$$HOME/.profile"; fi &&

# Minimum versions required
GO_MIN_VERSION := 1.24
NODE_MIN_VERSION := 24
NPM_MIN_VERSION := 11
HOST_GOMODCACHE := $(shell go env GOMODCACHE)
HOST_GOCACHE := $(shell go env GOCACHE)

.PHONY: help check bootstrap build run rerun test vet race commit-check test-smoke test-critical test-fuzz code-ql test-classification test-subagents-e2e install \
	release release-major release-minor release-patch dist-go \
	docker-build docker-up docker-down docker-logs \
	run-standalone run-full run-headless \
	electron-setup electron-dev electron-build electron-start electron-restart electron-dist \
	kill-gateway \
	workshop-setup workshop-up workshop-down workshop-logs workshop-ps

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

# ---------------------------------------------------------------------------
# Bootstrap & prerequisite check
# ---------------------------------------------------------------------------

bootstrap: ## Install build prerequisites for this platform
	@case "$$(uname -s)" in \
		Darwin) bash scripts/bootstrap-macos.sh ;; \
		Linux)  bash scripts/bootstrap-ubuntu.sh ;; \
		*)      echo "No bootstrap script for $$(uname -s). See PREREQUISITES.md"; exit 1 ;; \
	esac

check: ## Validate build prerequisites (Go, git, Node.js/npm)
	@echo "Checking prerequisites..."
	@command -v go >/dev/null 2>&1 || { \
		echo ""; \
		echo "  ERROR: Go is not installed."; \
		echo "  Required: Go >= $(GO_MIN_VERSION)"; \
		echo "  Install:  https://go.dev/dl/"; \
		echo "  See:      PREREQUISITES.md"; \
		echo ""; \
		exit 1; \
	}
	@GO_VER=$$(go version | sed -E 's/.*go([0-9]+\.[0-9]+).*/\1/'); \
	GO_MAJ=$$(echo "$$GO_VER" | cut -d. -f1); \
	GO_MIN=$$(echo "$$GO_VER" | cut -d. -f2); \
	REQ_MAJ=$$(echo "$(GO_MIN_VERSION)" | cut -d. -f1); \
	REQ_MIN=$$(echo "$(GO_MIN_VERSION)" | cut -d. -f2); \
	if [ "$$GO_MAJ" -lt "$$REQ_MAJ" ] || { [ "$$GO_MAJ" -eq "$$REQ_MAJ" ] && [ "$$GO_MIN" -lt "$$REQ_MIN" ]; }; then \
		echo ""; \
		echo "  ERROR: Go $$GO_VER is too old."; \
		echo "  Required: Go >= $(GO_MIN_VERSION)"; \
		echo "  Installed: Go $$GO_VER ($$(which go))"; \
		echo "  Install:  https://go.dev/dl/"; \
		echo "  See:      PREREQUISITES.md"; \
		echo ""; \
		exit 1; \
	fi
	@echo "  Go $$(go version | sed -E 's/.*go([0-9]+\.[0-9]+\.[0-9]+).*/\1/') — OK"
	@command -v git >/dev/null 2>&1 && \
		echo "  git $$(git --version | sed 's/git version //') — OK" || \
		echo "  WARNING: git not found (needed for work repo init)"
	@if command -v node >/dev/null 2>&1; then \
		NODE_VER=$$(node --version | sed 's/v//'); \
		NODE_MAJ=$$(echo "$$NODE_VER" | cut -d. -f1); \
		if [ "$$NODE_MAJ" -lt "$(NODE_MIN_VERSION)" ]; then \
			echo "  WARNING: Node.js $$NODE_VER is too old (need >= $(NODE_MIN_VERSION) for Electron)"; \
		else \
			echo "  Node.js $$NODE_VER — OK"; \
		fi; \
	else \
		echo "  WARNING: Node.js not found (needed for Electron desktop app)"; \
	fi
	@if command -v npm >/dev/null 2>&1; then \
		NPM_VER=$$(npm --version); \
		NPM_MAJ=$$(echo "$$NPM_VER" | cut -d. -f1); \
		if [ "$$NPM_MAJ" -lt "$(NPM_MIN_VERSION)" ]; then \
			echo "  WARNING: npm $$NPM_VER is too old (need >= $(NPM_MIN_VERSION))"; \
		else \
			echo "  npm $$NPM_VER — OK"; \
		fi; \
	else \
		echo "  WARNING: npm not found (bundled with Node.js)"; \
	fi
	@command -v lsof >/dev/null 2>&1 && \
		echo "  lsof — OK" || \
		echo "  WARNING: lsof not found (needed for make kill-gateway)"
	@command -v docker >/dev/null 2>&1 && \
		echo "  docker $$(docker --version | sed -E 's/Docker version ([0-9]+\.[0-9]+).*/\1/') — OK" || \
		echo "  docker — not installed (optional, for make docker-*)"
	@command -v gh >/dev/null 2>&1 && \
		echo "  gh $$(gh --version | head -1 | sed -E 's/.*version ([0-9]+\.[0-9]+).*/\1/') — OK" || \
		echo "  gh — not installed (optional, GitHub CLI)"
	@echo "  All prerequisites checked."

# ---------------------------------------------------------------------------
# Go binary
# ---------------------------------------------------------------------------

test: ## Run all tests
	go test ./...

vet: ## Run go vet across all packages
	go vet ./...

race: ## Run race-enabled tests across all packages
	go test -race ./...

test-smoke: ## Run fast critical-path smoke tests (bug-finding first)
	bash scripts/test_smoke.sh

test-critical: ## Enforce 100% coverage on critical logic
	bash scripts/check_critical_coverage.sh

test-fuzz: ## Run fuzz tests for critical guard logic
	bash scripts/test_fuzz.sh

code-ql: ## Run local CodeQL (Go + JS/TS + Actions) and emit SARIF under .tmp/codeql/
	bash scripts/codeql_local.sh

commit-check: check vet race test-fuzz code-ql ## Run pre-commit quality gates (vet, race, fuzz, CodeQL)
	@echo "commit-check completed."

test-classification: ## Run internal/external message classification E2E test (verbose)
	go test -v -run "TestInternalExternalClassificationE2E|TestMessageTypeAccessorDefaults|TestPolicyTierGatingByMessageType" ./internal/agent/

test-subagents-e2e: ## Run subagent nested announce routing + deferred retry parity tests
	mkdir -p .tmp/test-home
	HOME=$$(pwd)/.tmp/test-home GOMODCACHE=$(HOST_GOMODCACHE) GOCACHE=$(HOST_GOCACHE) go test -v -run "TestLoopNestedAnnounceDeferredRetry_RoutesToRootRequester|TestLoopStartSubagentRetryWorker_Continuous|TestLoopStartSubagentRetryWorker_DeferredCleanupDelete" ./internal/agent/

test-orchestrator: ## Run orchestrator tests (verbose)
	go test -v ./internal/orchestrator/

build: check ## Build the kafclaw binary
	go build -o kafclaw ./cmd/kafclaw

install: build ## Install kafclaw to /usr/local/bin
	cp kafclaw /usr/local/bin/kafclaw
	@echo "Installed to /usr/local/bin/kafclaw"

# ---------------------------------------------------------------------------
# Three operation modes (Go gateway)
# ---------------------------------------------------------------------------

run: build ## Build and run gateway (default, standalone mode)
	$(SOURCE_ENV) ./kafclaw gateway

run-standalone: build ## Mode 2: Standalone desktop — no Kafka, no orchestrator
	$(SOURCE_ENV) MIKROBOT_GROUP_ENABLED=false \
	./kafclaw gateway

run-full: build ## Mode 1: Full desktop — group + orchestrator enabled
	$(SOURCE_ENV) MIKROBOT_GROUP_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ROLE=orchestrator \
	./kafclaw gateway

run-headless: build ## Mode 3: Headless — binds 0.0.0.0, auth token required
	@if [ -z "$$KAFCLAW_GATEWAY_AUTH_TOKEN" ] && [ -z "$$MIKROBOT_GATEWAY_AUTH_TOKEN" ]; then \
		echo ""; \
		echo "  Set KAFCLAW_GATEWAY_AUTH_TOKEN to secure the API:"; \
		echo ""; \
		echo "    export KAFCLAW_GATEWAY_AUTH_TOKEN=mysecrettoken"; \
		echo "    make run-headless"; \
		echo ""; \
		exit 1; \
	fi
	$(SOURCE_ENV) MIKROBOT_GATEWAY_HOST=0.0.0.0 \
	MIKROBOT_GROUP_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ROLE=orchestrator \
	./kafclaw gateway

kill-gateway: ## Kill any running gateway on ports 18790/18791
	@set -euo pipefail; \
	pids=""; \
	for port in 18790 18791; do \
	  pid=$$(lsof -ti tcp:$$port -sTCP:LISTEN || true); \
	  if [[ -n "$$pid" ]]; then \
	    pids="$$pids $$pid"; \
	  fi; \
	done; \
	if [[ -n "$$pids" ]]; then \
	  echo "Killing gateway PIDs:$$pids"; \
	  kill $$pids; \
	else \
	  echo "No gateway processes found"; \
	fi

rerun: kill-gateway run ## Restart the gateway (kill + build + run)

rerun-full: kill-gateway run-full ## Restart in full mode (kill + build + run)

rerun-headless: kill-gateway run-headless ## Restart in headless mode (kill + build + run)

# ---------------------------------------------------------------------------
# Electron desktop app
# ---------------------------------------------------------------------------

electron-setup: ## Install Electron app dependencies
	cd electron && npm install

electron-dev: build electron-setup ## Dev mode: Vite + Electron (hot reload renderer)
	cd electron && npm run dev

electron-build: build electron-setup ## Build Electron app (main + renderer)
	cd electron && npm run build

electron-start: electron-build ## Build and launch Electron app
	cd electron && npm start

electron-restart: kill-gateway electron-build ## Rebuild Go + Electron, reset mode selection, and launch
	cd electron && npx electron . --reset-mode

electron-start-standalone: build electron-setup ## Launch Electron in standalone mode
	cd electron && npm run build && npx electron . --mode=standalone

electron-start-full: build electron-setup ## Launch Electron in full mode
	cd electron && npm run build && npx electron . --mode=full

electron-start-remote: electron-setup ## Launch Electron in remote client mode (no local binary)
	cd electron && npm run build && npx electron . --mode=remote

electron-dist: electron-build ## Package Electron app for current platform
	cd electron && npm run dist

electron-dist-mac: electron-build ## Package Electron .dmg for macOS
	cd electron && npm run dist:mac

electron-dist-linux: electron-build ## Package Electron .AppImage for Linux
	cd electron && npm run dist:linux

# ---------------------------------------------------------------------------
# Releases — `make release-patch` bumps, commits, tags, pushes → CI builds
# ---------------------------------------------------------------------------

release: release-patch ## Default: bump patch and release

release-major: ## Bump major version, commit, tag, push → triggers CI release
	@bash ./scripts/release.sh major

release-minor: ## Bump minor version, commit, tag, push → triggers CI release
	@bash ./scripts/release.sh minor

release-patch: ## Bump patch version, commit, tag, push → triggers CI release
	@bash ./scripts/release.sh patch

dist-go: ## Cross-compile Go binaries locally (all platforms)
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -o dist/kafclaw-darwin-arm64  ./cmd/kafclaw
	GOOS=darwin  GOARCH=amd64 go build -o dist/kafclaw-darwin-amd64  ./cmd/kafclaw
	GOOS=linux   GOARCH=amd64 go build -o dist/kafclaw-linux-amd64   ./cmd/kafclaw
	GOOS=linux   GOARCH=arm64 go build -o dist/kafclaw-linux-arm64   ./cmd/kafclaw
	@echo "Built 4 binaries in dist/"
	@ls -lh dist/kafclaw-*

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

docker-build: ## Build local Docker image (kafclaw:local) — multi-stage, no host binary needed
	docker build -t kafclaw:local -f Dockerfile .

docker-up: docker-build ## Start docker-compose using local image only
	SYSTEM_REPO_PATH=$${SYSTEM_REPO_PATH:-$$(pwd)} WORK_REPO_PATH=$${WORK_REPO_PATH:-$${HOME}/KafClaw-Workspace} docker compose -f docker-compose.yml up -d --no-build

docker-down: ## Stop docker-compose
	docker compose -f docker-compose.yml down

docker-logs: ## Tail docker-compose logs
	docker compose -f docker-compose.yml logs -f

# ---------------------------------------------------------------------------
# Workshop (group deployment: Kafka + 3 headless agents)
# ---------------------------------------------------------------------------

workshop-setup: ## Interactive setup for the 4-agent workshop
	@bash scripts/setup-workshop.sh

workshop-up: ## Start workshop stack (Kafka + 3 headless agents)
	docker compose -f docker-compose.group.yml up -d

workshop-down: ## Stop workshop stack
	docker compose -f docker-compose.group.yml down

workshop-logs: ## Tail workshop logs
	docker compose -f docker-compose.group.yml logs -f

workshop-ps: ## Show workshop container status
	docker compose -f docker-compose.group.yml ps
