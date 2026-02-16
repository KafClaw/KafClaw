# Prerequisites

## Quick Setup

Run the bootstrapper for your platform:

```bash
# macOS
bash scripts/bootstrap-macos.sh

# Ubuntu / Debian / Jetson Nano
bash scripts/bootstrap-ubuntu.sh
```

The scripts check what's already installed, only install what's missing, and verify everything works.

## Required

| Tool | Minimum Version | Check | Install |
|------|----------------|-------|---------|
| **Go** | 1.24.0 | `go version` | [go.dev/dl](https://go.dev/dl/) — download the tarball for your platform |
| **Git** | 2.0+ | `git --version` | `apt install git` / `brew install git` |
| **Make** | 3.81+ | `make --version` | Pre-installed on macOS and most Linux |

## Required for Electron Desktop App

| Tool | Minimum Version | Check | Install |
|------|----------------|-------|---------|
| **Node.js** | 18.0+ | `node --version` | [nodejs.org](https://nodejs.org/) or `brew install node` / `apt install nodejs` |
| **npm** | 9.0+ | `npm --version` | Bundled with Node.js |

Electron 28 and Vite 5 both require Node.js >= 18. npm is bundled with Node.js (Node 18 ships npm 9.x, Node 20 ships npm 10.x).

## Optional

| Tool | Purpose | Install |
|------|---------|---------|
| **Docker** + **Docker Compose** | Container builds (`make docker-*`) | [docker.com](https://docs.docker.com/get-docker/) |
| **gh** (GitHub CLI) | PR management, issue tracking, repo operations | `brew install gh` / [cli.github.com](https://cli.github.com/) |
| **lsof** | `make kill-gateway` (find process by port) | Pre-installed on macOS; `apt install lsof` on Linux |

## Platform Notes

### macOS

Xcode Command Line Tools provides git and make. Go is installed from the official tarball to `/usr/local/go`. The bootstrap script adds it to `~/.zshrc`.

### Ubuntu / Debian / Jetson Nano (ARM64)

Ubuntu 18.04 (Jetson Nano L4T) ships Go 1.10 via `apt`, which is too old. The bootstrap script removes the system Go and installs from the official ARM64 tarball. It also installs git and make via `apt` if missing.

## Verifying

```bash
cd gomikrobot
make check    # validates all prerequisites
```
