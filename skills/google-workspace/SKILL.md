---
name: google-workspace
description: Integrate with Google Workspace services under tenant policy and least privilege.
---

# google-workspace

Use for Gmail, Calendar, Drive, and Docs workflows.

## Workflow
- Validate requested action against tenant policy.
- Use minimal scopes required for the task.
- Return clear action log (resource, operation, result).

## Safety
- Disabled by default.
- Explicit approval for write/send/delete actions.
