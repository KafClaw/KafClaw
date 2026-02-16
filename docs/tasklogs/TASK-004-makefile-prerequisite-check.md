# TASK-004 — Add prerequisite checks and bootstrap scripts to Makefile

## Status: Done

## Completed: 2026-02-16

## Summary

Added a `make check` target that validates Go version >= 1.24, a `make bootstrap` target that auto-detects the platform and runs the appropriate setup script, and per-platform bootstrapper scripts for macOS and Ubuntu/Debian.

## Files Created

| File | Purpose |
|------|---------|
| `PREREQUISITES.md` | Documents all build/run prerequisites with install instructions |
| `scripts/bootstrap-macos.sh` | Installs Xcode CLT, Go (tarball), verifies git/node |
| `scripts/bootstrap-ubuntu.sh` | Installs git/make (apt), removes old Go, installs Go (tarball), handles ARM64 |

## Files Modified

| File | Change |
|------|--------|
| `Makefile` | Added `GO_MIN_VERSION`, `check` target (Go version validation), `bootstrap` target (platform-detect + run script), wired `check` as dependency of `build` |

## How It Works

- **`make check`** — Validates Go is installed and version >= 1.24. Warns if git is missing. Runs automatically before every `make build`.
- **`make bootstrap`** — Detects macOS vs Linux, runs the appropriate `scripts/bootstrap-*.sh`. Idempotent — safe to re-run.
- **Bootstrap scripts** — Check what's already installed, only install what's missing. Handle the Go 1.10 → 1.24 upgrade on Jetson Nano (removes apt Go, installs tarball). Add Go to PATH in shell rc file.

## Verification (macOS)

| Check | Result |
|-------|--------|
| `make check` | "Go 1.24.13 — OK / All prerequisites met." |
| `make build` | Runs check, then compiles |
| `make help` | Shows `bootstrap` and `check` targets |

## Insights

- The `check` target uses pure POSIX shell (no bashisms beyond `[[ ]]` which bash handles) so it works with the `SHELL := /bin/bash` directive.
- Go version parsing uses `sed` + `cut` rather than bash-specific `${var//pattern}` for portability.
- The bootstrap scripts are idempotent — they check for existing installs before doing anything, and only append to shell rc if the PATH line isn't already present.
- On Jetson Nano, the Ubuntu script explicitly removes the apt-provided Go 1.10 before installing the tarball version, avoiding PATH conflicts.
