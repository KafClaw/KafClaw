---
title: Group and Kafka Operations
parent: Collaboration
nav_order: 1
---

# Group and Kafka Operations

Core runbook for distributed KafClaw collaboration.

## Group Lifecycle

```bash
./kafclaw group join mygroup
./kafclaw group status
./kafclaw group members
./kafclaw group leave
```

Operational notes:

- Group state is persisted in settings (`group_name`, `group_active`).
- Heartbeat continuity metadata persists across restarts (`group_heartbeat_*`, `group_heartbeat_seq`).
- Startup reconciliation persists `runtime_reconcile_*` counters.

## Kafka Configuration

Onboarding profile:

```bash
./kafclaw onboard --non-interactive --profile local-kafka \
  --kafka-brokers "broker1:9092,broker2:9092" \
  --group-name kafclaw \
  --agent-id agent-ops \
  --role worker \
  --llm skip
```

Direct config:

```bash
./kafclaw config set group.enabled true
./kafclaw config set group.groupName "kafclaw"
./kafclaw config set group.kafkaBrokers "broker1:9092,broker2:9092"
./kafclaw config set group.consumerGroup "kafclaw-workers"
./kafclaw config set group.agentId "agent-ops"
```

Security examples:

```bash
./kafclaw config set group.kafkaSecurityProtocol "SASL_SSL"
./kafclaw config set group.kafkaSaslMechanism "PLAIN"
./kafclaw config set group.kafkaSaslUsername "<username>"
./kafclaw config set group.kafkaSaslPassword "<password>"
./kafclaw config set group.kafkaTlsCAFile "/path/to/ca.pem"
```

## Diagnostics (KShark)

Auto-detect from group config:

```bash
./kafclaw kshark --auto --yes
```

Explicit props:

```bash
./kafclaw kshark --props ./client.properties --topic group.mygroup.requests --group mygroup-workers --yes
```

Useful options:

- `--json <file>` export report
- `--diag` include traceroute/MTU diagnostics
- `--preset` use connection preset templates
