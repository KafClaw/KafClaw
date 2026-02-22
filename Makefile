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

# ---------------------------------------------------------------------------
# Python Scripts
# ---------------------------------------------------------------------------

define PY_CODEQL_SUMMARY
import glob, json, re
from collections import Counter

sarifs = sorted(glob.glob(".tmp/codeql/*.sarif"))
if not sarifs:
    print("No SARIF files found under .tmp/codeql/. Did make code-ql run successfully?")
    raise SystemExit(2)

class C:
    RED = '\033[91m'; YEL = '\033[93m'; CYA = '\033[96m'; GRN = '\033[92m'
    RST = '\033[0m'; BLD = '\033[1m'; DIM = '\033[2m'

c_map = {"error": C.RED, "warning": C.YEL, "note": C.CYA, "recommendation": C.CYA}
icon_map = {"error": "❌", "warning": "⚠️ ", "note": "ℹ️ ", "recommendation": "💡"}

def first_location(result):
    locs = result.get("locations") or []
    if not locs:
        return ("unknown", 0)
    pl = (locs[0].get("physicalLocation") or {})
    file = ((pl.get("artifactLocation") or {}).get("uri") or "unknown")
    line = ((pl.get("region") or {}).get("startLine") or 0)
    return (file, line)

total = 0
levels = Counter()
rows = []

for path in sarifs:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    for run in data.get("runs", []):
        for r in (run.get("results") or []):
            lvl = (r.get("level") or "warning").lower()
            rule = r.get("ruleId") or "no-rule"
            msg = (r.get("message") or {}).get("text") or "no-message"
            # Strip markdown links like [text](1) -> text
            msg = re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', msg)
            file, line = first_location(r)
            levels[lvl] += 1
            total += 1
            rows.append((lvl, rule, file, line, msg))

print(f"\n{C.BLD}CodeQL SARIF Summary:{C.RST}")
for p in sarifs:
    print(f"  {C.DIM}- {p}{C.RST}")
print(f"\n  Total findings: {C.BLD}{total}{C.RST}")
if total:
    print("  By level: " + ", ".join(f"{c_map.get(k, '')}{k}={v}{C.RST}" for k, v in sorted(levels.items())))
print("")

priority = {"error": 0, "warning": 1, "note": 2, "recommendation": 3}
rows.sort(key=lambda x: (priority.get(x[0], 9), x[2], x[3], x[1]))

limit = 50
if not rows:
    print(f"  {C.GRN}✅ No findings.{C.RST}")
else:
    print(f"  {C.BLD}Top {min(limit, len(rows))} findings:{C.RST}")
    for i, (lvl, rule, file, line, msg) in enumerate(rows[:limit], 1):
        msg = msg.replace("\n", " ").strip()
        if len(msg) > 120:
            msg = msg[:117] + "..."
        col = c_map.get(lvl, "")
        icn = icon_map.get(lvl, "-")
        print(f"  {i:>2}. {icn} {col}[{lvl}]{C.RST} {C.BLD}{rule}{C.RST}")
        print(f"      {C.DIM}📄 {file}:{line}{C.RST}")
        print(f"      💬 {msg}")
        print()
endef
export PY_CODEQL_SUMMARY

define PY_CODEQL_GATE
import glob, json
sarifs = sorted(glob.glob(".tmp/codeql/*.sarif"))
errors = 0
warnings = 0
for path in sarifs:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    for run in data.get("runs", []):
        for r in (run.get("results") or []):
            lvl = (r.get("level") or "warning").lower()
            if lvl == "error":
                errors += 1
            elif lvl == "warning":
                warnings += 1

if errors > 0:
    print(f"\n\033[91m❌ CodeQL gate failed: {errors} blocking error(s) found.\033[0m")
    if warnings > 0:
        print(f"   \033[93m(Also found {warnings} non-blocking warnings)\033[0m")
    raise SystemExit(1)

print(f"\n\033[92m✅ CodeQL gate passed: no blocking errors found.\033[0m")
if warnings > 0:
    print(f"   \033[93m⚠️  Note: {warnings} non-blocking warning(s) were found.\033[0m")
endef
export PY_CODEQL_GATE

.PHONY: help check bootstrap build run rerun test vet race fmt fmt-check commit-check test-smoke test-critical test-fuzz code-ql code-ql-summary code-ql-gate test-classification test-subagents-e2e check-bundled-skills install \
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
		echo "  docker $$(docker --version | sed 's/Docker version //; s/,.*//') — OK" || \
		echo "  WARNING: docker not found (needed for docker targets)"
	@echo "Prerequisite check completed."

# ---------------------------------------------------------------------------
# Build & run
# ---------------------------------------------------------------------------

build: ## Build the gateway (Go binary)
	go build ./...

run: ## Run the gateway
	go run ./cmd/gateway

rerun: ## Run the gateway with env sourced
	@$(SOURCE_ENV) go run ./cmd/gateway

run-standalone: ## Run standalone mode
	go run ./cmd/gateway --mode standalone

run-full: ## Run full mode
	go run ./cmd/gateway --mode full

run-headless: ## Run headless mode
	go run ./cmd/gateway --mode headless

# ---------------------------------------------------------------------------
# Tests & quality
# ---------------------------------------------------------------------------

test: ## Run tests
	go test ./...

vet: ## Run go vet
	go vet ./...

race: ## Run race detector tests
	go test -race ./...

fmt: ## Auto-format Go code
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "⚠️  Found unformatted files. Auto-formatting now:"; \
		echo "$$unformatted"; \
		gofmt -w .; \
	else \
		echo "✅ All Go files are formatted correctly."; \
	fi

fmt-check: ## Check formatting (fails if unformatted - for CI)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "❌ The following files are not gofmt-formatted:"; \
		echo "$$unformatted"; \
		echo "👉 Run 'make fmt' to fix them."; \
		exit 1; \
	fi

test-smoke: ## Run smoke tests
	bash scripts/test_smoke.sh

check-bundled-skills: ## Validate that bundled skills/docs artifacts exist
	bash scripts/check_bundled_skills.sh

test-critical: ## Enforce 100% coverage on critical logic
	bash scripts/check_critical_coverage.sh

test-fuzz: ## Run fuzz tests for critical guard logic
	bash scripts/test_fuzz.sh

code-ql: ## Run local CodeQL (Go + JS/TS + Actions) and emit SARIF under .tmp/codeql/
	bash scripts/codeql_local.sh

code-ql-summary: code-ql ## Run CodeQL and print a readable summary from SARIF
	@printf "%s\n" "$$PY_CODEQL_SUMMARY" | python3 -

code-ql-gate: code-ql-summary ## Fail if CodeQL reports any error/warning findings
	@printf "%s\n" "$$PY_CODEQL_GATE" | python3 -

commit-check: check fmt vet race test-fuzz code-ql-gate ## Run pre-commit quality gates (fmt, vet, race, fuzz, CodeQL)
	@echo "commit-check completed."

test-classification: ## Run internal/external message classification E2E test (verbose)
	go test -v -run "TestInternalExternalClassificationE2E|TestMessageTypeAccessorDefaults|TestPolicyTierGatingByMessageType" ./internal/agent/

test-subagents-e2e: ## Run subagent nested announce routing + deferred retry parity tests
	mkdir -p .tmp/e2e
	go test -v -run "TestSubagentsE2E" ./internal/agent/ > .tmp/e2e/subagents.log
	@echo "E2E logs written to .tmp/e2e/subagents.log"

install: ## Install gateway binary to GOBIN
	go install ./cmd/gateway

# ---------------------------------------------------------------------------
# Release
# ---------------------------------------------------------------------------

release: ## Build release artifacts
	bash scripts/release.sh

release-major: ## Bump major version + build release
	bash scripts/release.sh major

release-minor: ## Bump minor version + build release
	bash scripts/release.sh minor

release-patch: ## Bump patch version + build release
	bash scripts/release.sh patch

dist-go: ## Build Go distribution artifacts
	bash scripts/dist_go.sh

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

docker-build: ## Build Docker image
	docker build -t gateway .

docker-up: ## Start docker compose stack
	docker compose up -d

docker-down: ## Stop docker compose stack
	docker compose down

docker-logs: ## Tail docker compose logs
	docker compose logs -f

# ---------------------------------------------------------------------------
# Electron desktop app
# ---------------------------------------------------------------------------

electron-setup: ## Install Electron app dependencies
	cd desktop && npm install

electron-dev: ## Run Electron app in dev mode
	cd desktop && npm run dev

electron-build: ## Build Electron app
	cd desktop && npm run build

electron-start: ## Start Electron app
	cd desktop && npm run start

electron-restart: ## Restart Electron app
	cd desktop && npm run restart

electron-dist: ## Package Electron app
	cd desktop && npm run dist

# ---------------------------------------------------------------------------
# Utility
# ---------------------------------------------------------------------------

kill-gateway: ## Kill any process listening on the gateway port (default 8080)
	@PORT=8080; \
	PIDS=$$(lsof -ti tcp:$$PORT || true); \
	if [ -z "$$PIDS" ]; then \
		echo "No process found listening on port $$PORT"; \
	else \
		echo "Killing processes on port $$PORT: $$PIDS"; \
		kill -9 $$PIDS; \
	fi

# ---------------------------------------------------------------------------
# Workshop (Docker-based dev env)
# ---------------------------------------------------------------------------

workshop-setup: ## Setup workshop env
	bash scripts/workshop_setup.sh

workshop-up: ## Start workshop stack
	docker compose -f workshop/docker-compose.yml up -d

workshop-down: ## Stop workshop stack
	docker compose -f workshop/docker-compose.yml down

workshop-logs: ## Tail workshop logs
	docker compose -f workshop/docker-compose.yml logs -f

workshop-ps: ## Show workshop containers
	docker compose -f workshop/docker-compose.yml ps