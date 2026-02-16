SHELL := /bin/bash
# Source the user's shell profile so env vars (API keys, PATH, etc.) are available.
# Tries .bashrc first (Linux default), then .zshrc (macOS default), then .profile (POSIX fallback).
SOURCE_ENV := if [ -f "$$HOME/.bashrc" ]; then . "$$HOME/.bashrc"; elif [ -f "$$HOME/.zshrc" ]; then . "$$HOME/.zshrc"; elif [ -f "$$HOME/.profile" ]; then . "$$HOME/.profile"; fi &&

# Minimum versions required
GO_MIN_VERSION := 1.24
NODE_MIN_VERSION := 18
NPM_MIN_VERSION := 9

.PHONY: help check bootstrap build run rerun test test-classification install \
	release release-major release-minor release-patch dist-go \
	docker-build docker-up docker-down docker-logs \
	run-standalone run-full run-headless \
	electron-setup electron-dev electron-build electron-start electron-restart electron-dist \
	kill-gateway

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

test-classification: ## Run internal/external message classification E2E test (verbose)
	go test -v -run "TestInternalExternalClassificationE2E|TestMessageTypeAccessorDefaults|TestPolicyTierGatingByMessageType" ./internal/agent/

test-orchestrator: ## Run orchestrator tests (verbose)
	go test -v ./internal/orchestrator/

build: check ## Build the gomikrobot binary
	go build ./cmd/gomikrobot

install: build ## Install gomikrobot to /usr/local/bin
	cp gomikrobot /usr/local/bin/gomikrobot
	@echo "Installed to /usr/local/bin/gomikrobot"

# ---------------------------------------------------------------------------
# Three operation modes (Go gateway)
# ---------------------------------------------------------------------------

run: build ## Build and run gateway (default, standalone mode)
	$(SOURCE_ENV) ./gomikrobot gateway

run-standalone: build ## Mode 2: Standalone desktop — no Kafka, no orchestrator
	$(SOURCE_ENV) MIKROBOT_GROUP_ENABLED=false \
	./gomikrobot gateway

run-full: build ## Mode 1: Full desktop — group + orchestrator enabled
	$(SOURCE_ENV) MIKROBOT_GROUP_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ROLE=orchestrator \
	./gomikrobot gateway

run-headless: build ## Mode 3: Headless — binds 0.0.0.0, auth token required
	@if [ -z "$$MIKROBOT_GATEWAY_AUTH_TOKEN" ]; then \
		echo ""; \
		echo "  Set MIKROBOT_GATEWAY_AUTH_TOKEN to secure the API:"; \
		echo ""; \
		echo "    export MIKROBOT_GATEWAY_AUTH_TOKEN=mysecrettoken"; \
		echo "    make run-headless"; \
		echo ""; \
		exit 1; \
	fi
	$(SOURCE_ENV) MIKROBOT_GATEWAY_HOST=0.0.0.0 \
	MIKROBOT_GROUP_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ENABLED=true \
	MIKROBOT_ORCHESTRATOR_ROLE=orchestrator \
	./gomikrobot gateway

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
	GOOS=darwin  GOARCH=arm64 go build -o dist/gomikrobot-darwin-arm64  ./cmd/gomikrobot
	GOOS=darwin  GOARCH=amd64 go build -o dist/gomikrobot-darwin-amd64  ./cmd/gomikrobot
	GOOS=linux   GOARCH=amd64 go build -o dist/gomikrobot-linux-amd64   ./cmd/gomikrobot
	GOOS=linux   GOARCH=arm64 go build -o dist/gomikrobot-linux-arm64   ./cmd/gomikrobot
	@echo "Built 4 binaries in dist/"
	@ls -lh dist/gomikrobot-*

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

docker-build: build ## Build local Docker image (gomikrobot:local)
	docker build -t gomikrobot:local -f Dockerfile .

docker-up: docker-build ## Start docker-compose using local image only
	SYSTEM_REPO_PATH=$${SYSTEM_REPO_PATH:-$$(pwd)} WORK_REPO_PATH=$${WORK_REPO_PATH:-$${HOME}/KafClaw-Workspace} docker compose -f docker-compose.yml up -d --no-build

docker-down: ## Stop docker-compose
	docker compose -f docker-compose.yml down

docker-logs: ## Tail docker-compose logs
	docker compose -f docker-compose.yml logs -f
