# BUG-002 — Jetson Nano has Go 1.10, project requires Go 1.24+

## Status: Open

## Reported: 2026-02-16

## Platform: Jetson Nano (ARM64 Ubuntu 18.04 / L4T)

## Evolved from: BUG-001 (shell fix resolved, new blocker exposed)

## Symptoms

```
kamir@jetson-nano-1:~/KafClaw/gomikrobot$ sudo make dist-go
GOOS=darwin  GOARCH=arm64 go build -o dist/gomikrobot-darwin-arm64  ./cmd/gomikrobot
cmd/gomikrobot/main.go:7:2: cannot find package "github.com/KafClaw/KafClaw/gomikrobot/cmd/gomikrobot/cmd" in any of:
    /usr/lib/go-1.10/src/github.com/KafClaw/KafClaw/gomikrobot/cmd/gomikrobot/cmd (from $GOROOT)
    /home/kamir/go/src/github.com/KafClaw/KafClaw/gomikrobot/cmd/gomikrobot/cmd (from $GOPATH)
Makefile:144: recipe for target 'dist-go' failed
make: *** [dist-go] Error 1
```

## Root Cause

The Jetson Nano has **Go 1.10** installed (from Ubuntu 18.04's `apt` repository). This version:

- **Predates Go modules** (introduced in Go 1.11, default since Go 1.16)
- Falls back to `$GOPATH` package resolution, which doesn't work with the project's module layout
- Cannot compile code using `go:embed` (introduced in Go 1.16)
- Cannot use generics or other modern Go features

The project's `go.mod` specifies `go 1.24.0` with toolchain `go1.24.13`.

## Impact

- `make build`, `make dist-go`, `make test` — all fail on the Jetson Nano
- The Makefile shell fix (TASK-001) is confirmed working — the error is now a Go version problem, not a shell problem
- macOS dev machine is unaffected (has Go 1.24+)

## Analysis

### Why Go 1.10 is installed

Ubuntu 18.04 (Bionic) — the base OS for Jetson Nano L4T — ships Go 1.10 via `apt`. There is no newer Go in the default repos for this Ubuntu version.

### Options to install Go 1.24+ on ARM64

1. **Official Go tarball** (recommended):
   ```bash
   wget https://go.dev/dl/go1.24.13.linux-arm64.tar.gz
   sudo rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.24.13.linux-arm64.tar.gz
   export PATH=/usr/local/go/bin:$PATH
   ```
   - Works on any ARM64 Linux regardless of distro version
   - Official, well-tested ARM64 builds available since Go 1.16

2. **Snap** (if available):
   ```bash
   sudo snap install go --classic
   ```
   - May not be available on older L4T images

3. **Cross-compile from macOS** (workaround):
   ```bash
   # On macOS dev machine:
   GOOS=linux GOARCH=arm64 go build -o dist/gomikrobot-linux-arm64 ./cmd/gomikrobot
   # Then scp to Jetson Nano
   ```
   - No Go needed on the Nano at all for running the binary
   - Only needed if you want to build ON the Nano

4. **Docker build on Nano** (if Docker is available):
   ```bash
   docker run --rm -v $(pwd):/src -w /src golang:1.24 go build ./cmd/gomikrobot
   ```

## Fix Plan

See TASK-003.
