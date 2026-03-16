---
parent: Operations and Admin
title: Release Process
---

# Release Process

## Versioning

KafClaw uses semantic versioning (`MAJOR.MINOR.PATCH`). The version is defined in `internal/cli/root.go` and can be overridden at build time:

```bash
go build -ldflags "-X github.com/KafClaw/KafClaw/internal/cli.version=0.1.0" ./cmd/kafclaw
```

## Make Targets

From the KafClaw source directory:

```bash
make release-patch    # bumps PATCH, builds, tags, pushes
make release-minor    # bumps MINOR, builds, tags, pushes
make release-major    # bumps MAJOR, builds, tags, pushes
```

Each `make release*` target:
1. Bumps the version via `scripts/release.sh`
2. Creates a commit: `Release vX.Y.Z`
3. Tags: `vX.Y.Z`
4. Pushes commit and tag to remote

Release commits are created from the repository root (all staged changes included).

## GitHub Actions

- Workflow: `.github/workflows/release.yml`
- Trigger: tag push `v*`, scheduled edge releases, or manual `workflow_dispatch`
- Scheduled edge releases are skipped when `main` has not moved since the most recent `edge-*` tag
- Build matrix: `ubuntu-latest`, `macos-latest`, `windows-latest`
- Artifacts attached to GitHub Release via `softprops/action-gh-release@v2`

## Script

Release bump logic: `scripts/release.sh`
