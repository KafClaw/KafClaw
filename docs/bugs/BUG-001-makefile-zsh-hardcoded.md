# BUG-001 — Makefile hardcodes /bin/zsh, breaks on Linux (Jetson Nano)

## Status: Fixed (shell issue) — evolved into BUG-002

## Reported: 2026-02-16

## Platform: Jetson Nano (ARM64 Ubuntu)

## Symptoms

```
kamir@jetson-nano-1:~/KafClaw/gomikrobot$ make
make: /bin/zsh: Command not found
Makefile:13: recipe for target 'help' failed
make: *** [help] Error 127
```

## Root Cause

Three issues in `gomikrobot/Makefile`:

### 1. Hardcoded zsh (line 1)

```makefile
SHELL := /bin/zsh
```

`/bin/zsh` is not installed by default on Ubuntu/Debian ARM64 (Jetson Nano, Raspberry Pi, cloud VMs). Only macOS ships with zsh as default.

### 2. zsh-specific env sourcing (line 3)

```makefile
SOURCE_ENV := source $$HOME/.zshrc &&
```

Sources `.zshrc` which does not exist on bash-default systems. Also uses `source` which is a bashism (POSIX uses `.`).

### 3. bash/zsh-specific syntax in kill-gateway (lines 69-82)

```makefile
kill-gateway:
	@set -euo pipefail; \
	...
	  if [[ -n "$$pid" ]]; then \
	...
	if [[ -n "$$pids" ]]; then \
```

`[[ ]]` is not POSIX sh. `set -o pipefail` is not POSIX sh. Both work in bash and zsh but not in `/bin/sh`.

### 4. Missing ARM64 Linux in dist-go

The `dist-go` target builds `linux/arm64` already, so this is fine. But the binary name is still `gomikrobot` — should be `kafclaw` post-rebrand (separate issue).

## Impact

- **All make targets are broken** on any Linux system without zsh installed
- Affects: Jetson Nano, Raspberry Pi, most Docker containers, most CI runners, cloud VMs
- macOS is unaffected (zsh is default since Catalina)

## Fix Plan

See TASK-001 below.

## Affected Files

| File | Lines | Issue |
|------|-------|-------|
| `gomikrobot/Makefile` | 1 | `SHELL := /bin/zsh` |
| `gomikrobot/Makefile` | 3 | `SOURCE_ENV` sources `.zshrc` |
| `gomikrobot/Makefile` | 69-82 | `[[ ]]` and `pipefail` |
| `gomikrobot/Makefile` | 40,43,47,52,62 | `$(SOURCE_ENV)` usage in run targets |

## Workaround

Install zsh on the Jetson Nano:

```bash
sudo apt-get install zsh
```

This unblocks immediately but is not the right long-term fix.
