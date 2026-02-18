---
parent: Operations and Admin
---

# Docker Compose Deployment

> See also: [FR-024 Standalone Mode](../requirements/FR-024-standalone-mode/) for deployment mode details

## Build Image

From the KafClaw source directory:

```bash
make docker-build
```

## Run

Set host repo paths and start:

```bash
make docker-up
```

- `SYSTEM_REPO_PATH` defaults to current directory
- `WORK_REPO_PATH` defaults to `~/.kafclaw/work-repo`

## Stop

```bash
make docker-down
```

## Logs

```bash
make docker-logs
```

## Configuration

The Docker Compose setup mounts the following volumes:

| Host Path | Container Path | Purpose |
|-----------|----------------|---------|
| `$SYSTEM_REPO_PATH` | `/opt/system-repo` | System/identity repository |
| `$WORK_REPO_PATH` | `/opt/work-repo` | Work repository |
| `~/.kafclaw` | `/root/.kafclaw` | Config, timeline DB, WhatsApp session |

Ports exposed:
- `18790` — API server (POST /chat)
- `18791` — Dashboard / Web UI

## Notes

- Uses `kafclaw:local` image only (no remote pulls).
- Base image: `alpine:3.20` with `ca-certificates`.
- Entrypoint: `/usr/local/bin/kafclaw gateway`
