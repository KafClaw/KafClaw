# KafClaw Documentation

<p align="center">
  <img src="./assets/kafclaw.png" alt="KafClaw Logo" width="320" />
</p>

KafClaw is a Go-based agent runtime with three practical deployment modes:

- `local`: personal assistant on one machine
- `local-kafka`: local runtime connected to Kafka/group orchestration
- `remote`: headless gateway reachable over network (token required)

## Ecosystem

- **KafScale** ([github.com/kafscale](https://github.com/kafscale), [kafscale.io](https://kafscale.io)): Kafka-compatible and S3-compatible data plane used for durable event transport and large artifact flows in agent systems.
- **GitClaw** (in this KafClaw repository): agentic, self-hosted GitHub replacement focused on autonomous repository workflows and automation.
- **KafClaw**: runtime and coordination layer for local, Kafka-connected, and remote/headless agents.

## Why KafClaw

KafClaw gives teams an enterprise-ready way to run and coordinate autonomous agents over Apache Kafka.
Instead of locking agents to one model, one language, or one SDK, it uses a typed Kafka message protocol so any runtime that can read and write JSON envelopes can participate.

What this enables in practice:

- Reliable multi-agent collaboration with correlation IDs, timestamps, and trace-friendly envelopes
- Group and hierarchy orchestration with role boundaries and scoped visibility
- Shared memory and skill routing so agents can reuse knowledge across sessions
- Flexible deployment from a single local assistant to a Kafka-connected production gateway

In short: Kafka is the backbone, and KafClaw is the coordination layer that keeps heterogeneous agents working together safely and observably.

## Start Here

- [Getting Started](./getting-started.md) - install, onboard, first run
- [Manage KafClaw](./manage-kafclaw.md) - operator runbook for CLI, health checks, group and bridge operations
- [Operations and Maintenance](./maintenance.md) - runbook for updates, checks, backups, troubleshooting
- [Architecture Visual (HTML)](./architecture.html) - system overview diagram and component map

## Agent Concepts

- [How Agents Work](./concepts/how-agents-work.md) - end-to-end runtime flow from inbound message to outbound response
- [Soul and Identity Files](./concepts/soul-identity-tools.md) - what `AGENTS.md`, `SOUL.md`, `USER.md`, `TOOLS.md`, `IDENTITY.md` do
- [Runtime Tools and Capabilities](./concepts/runtime-tools.md) - actual tools registered by the Go runtime

## Integrations

- [Slack and Teams Bridge](./v2/slack-teams-bridge.md)
- [WhatsApp Setup](./v2/whatsapp-setup.md)
- [WhatsApp Onboarding](./v2/whatsapp-onboarding.md)

## Operations and Admin

- [Admin Guide](./v2/admin-guide.md)
- [Operations Guide](./v2/operations-guide.md)
- [Docker Deployment](./v2/docker-deployment.md)
- [Release Guide](./v2/release.md)

## Architecture and Security

- [Architecture Overview](./v2/architecture.md)
- [Detailed Architecture](./v2/architecture-detailed.md)
- [Timeline Architecture](./v2/architecture-timeline.md)
- [Security Risks](./v2/security-risks.md)
- [Subagents Threat Model](./v2/subagents-threat-model.md)

## Full v2 Reference Set

- [v2 Index](./v2/index.md)
- [User Manual](./v2/user-manual.md)

## Security Baseline

- Default bind is loopback: `127.0.0.1`
- Remote/headless mode should use auth token for dashboard API
- When `gateway.authToken` is set, both dashboard API and `POST /chat` require bearer auth (dashboard keeps `/api/v1/status` open for health checks)
- Use `kafclaw doctor` for diagnostics
- Use `kafclaw doctor --fix` for env merge and permissions hygiene

## Repository Docs Areas

- `docs/concepts/` - runtime behavior and core mental models
- `docs/v2/` - full product and operations reference set
- `docs/security/` - security audits and roadmap
