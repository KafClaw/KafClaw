---
title: Google CLI
parent: Skills
nav_order: 7
---

# Google CLI

Install and validate the Google Cloud CLI (`gcloud`) for cloud/operator workflows.

## Default State

- Bundled with KafClaw
- Disabled by default

## Check

```bash
kafclaw skills prereq check google-cli
```

## Install Plan

```bash
kafclaw skills prereq install google-cli --dry-run
```

## Install

```bash
kafclaw skills prereq install google-cli --yes
```

The installer uses Go-native OS routines (APT on Linux, Homebrew on macOS).

## Enable Skill

```bash
kafclaw skills enable-skill google-cli
```

## Usage

- Verify auth status:

```bash
gcloud auth list
```

- Verify active project:

```bash
gcloud config get-value project
```

## Troubleshooting

- If install fails on Linux, run package metadata update and retry.
- If `gcloud` is not found after install, ensure your shell `PATH` includes the install location.
- If auth fails in headless environments, use device/browser-based auth flow from Google CLI.
