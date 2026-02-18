---
title: Home
nav_order: 1
---

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

## Start Here

- [Start Here](./start-here/index.md)
- [Getting Started](./start-here/getting-started.md)
- [User Manual](./start-here/user-manual.md)

## Agent Concepts

- [Agent Concepts](./agent-concepts/index.md)
- [How Agents Work](./agent-concepts/how-agents-work.md)
- [Soul and Identity Files](./agent-concepts/soul-identity-tools.md)
- [Runtime Tools and Capabilities](./agent-concepts/runtime-tools.md)

## Integrations

- [Integrations](./integrations/index.md)
- [Slack and Teams Bridge](./integrations/slack-teams-bridge.md)
- [WhatsApp Setup](./integrations/whatsapp-setup.md)
- [WhatsApp Onboarding](./integrations/whatsapp-onboarding.md)

## Operations and Admin

- [Operations and Admin](./operations-admin/index.md)
- [Manage KafClaw](./operations-admin/manage-kafclaw.md)
- [Operations and Maintenance](./operations-admin/maintenance.md)
- [Admin Guide](./operations-admin/admin-guide.md)
- [Operations Guide](./operations-admin/operations-guide.md)
- [Docker Deployment](./operations-admin/docker-deployment.md)
- [Release Guide](./operations-admin/release.md)

## Architecture and Security

- [Architecture and Security](./architecture-security/index.md)
- [Architecture Overview](./architecture-security/architecture.md)
- [Detailed Architecture](./architecture-security/architecture-detailed.md)
- [Timeline Architecture](./architecture-security/architecture-timeline.md)
- [Security Risks](./architecture-security/security-risks.md)
- [Subagents Threat Model](./architecture-security/subagents-threat-model.md)
