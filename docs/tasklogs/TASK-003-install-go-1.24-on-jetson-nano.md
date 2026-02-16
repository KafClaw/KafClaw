# TASK-003 — Install Go 1.24 on Jetson Nano and validate build

## Status: Done

## Completed: 2026-02-16

## Summary

Replaced Go 1.10 (from Ubuntu 18.04 apt) with Go 1.24.13 on the Jetson Nano so the project compiles natively on ARM64.

## What was done

1. Removed system Go 1.10 (`apt remove golang-go`)
2. Installed Go 1.24.13 from official ARM64 tarball to `/usr/local/go`
3. Updated PATH in `~/.bashrc`
4. Verified `go version` reports 1.24.13

## Insights

- Ubuntu 18.04 (Jetson Nano L4T base) ships Go 1.10 via apt — far too old for Go modules (1.11+), `go:embed` (1.16+), or any modern Go feature.
- The official Go ARM64 tarball works perfectly on Jetson Nano. No compilation from source needed.
- This was also the motivation for adding `make check` (TASK-004) and `scripts/bootstrap-ubuntu.sh` — so future setups can be automated with `make bootstrap`.

## References

- BUG-002: Jetson Nano has Go 1.10, project requires Go 1.24+
- TASK-004: Added prerequisite checks and bootstrap scripts to Makefile
