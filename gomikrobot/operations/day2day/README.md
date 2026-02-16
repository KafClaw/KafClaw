# Day2Day Task Tracker

The Day2Day system manages daily task files at `operations/day2day/tasks/YYYY-MM-DD.md`.

## Commands (3-letter prefixes)

Users send these as the first word of a message. The bot intercepts them before they reach the LLM.

| Command | Name | Description |
|---------|------|-------------|
| `dtu <text>` | Update | Add new tasks and log an UPDATE entry. Multi-line: send `dtu` alone to start capture, then content, then `dtc` to close. |
| `dtp <text>` | Progress | Log a PROGRESS entry without adding tasks. Multi-line: send `dtp` alone to start capture, then content, then `dtc` to close. |
| `dts` | Consolidate | Deduplicate tasks, update consolidated state (open/done counts), and suggest next step. |
| `dtn` | Next | Show the next suggested task (first open checkbox). |
| `dta` | All | Show all open tasks for today. |
| `dtc` | Close capture | End a multi-line `dtu`/`dtp` capture session. |

## Status query

Any message containing **"status"** and one of **"task"**, **"aufgabe"**, or **"day2day"** triggers a status report for today (or a specific date if included as `YYYY-MM-DD`, or "yesterday"/"tomorrow"/"gestern"/"morgen").

Example: `status tasks` or `day2day task status 2026-02-14`

## Task file format

```markdown
# Day2Day — 2026-02-14 (Friday)

## Tasks
- [ ] Open task
- [x] Completed task

## Progress Log
- 14:30: UPDATE — Added deployment steps
- 15:00: PROGRESS — Reviewed PR #42

## Notes / Context

## Consolidated State
- Open: 3
- Done: 2
- Last Consolidation: 16:00

## Next Step
- Deploy staging environment
```

## Natural Language Task Handling

When the user asks about tasks in natural language (not using dtu/dtp/dts commands), follow these rules STRICTLY:

### RULE 1: Always read before answering
Before answering ANY question about tasks (e.g. "show me tasks", "what's on my list for Monday"), you MUST:
1. Determine the target date (compute from "tomorrow", "Monday", etc. using the current date shown in your system prompt)
2. Use `read_file` to read `{system_repo}/operations/day2day/tasks/YYYY-MM-DD.md`
3. Answer based ONLY on file contents. Never answer from memory or prior conversation.

### RULE 2: Never overwrite — always append
When adding tasks to an existing file:
1. First `read_file` to get current contents
2. Use `edit_file` to INSERT new tasks at the end of the `## Tasks` section
3. NEVER use `write_file` on an existing task file — it destroys previous content

When creating a new file (no file exists for that date):
1. Use `write_file` with the full template including the new tasks

### RULE 3: Preserve original content
- Keep the user's original language (German stays German, English stays English)
- Do not rephrase, categorize, or reformat task text
- Use the exact checkbox format: `- [ ] <user's text as-is>`

### RULE 4: Date computation
- "tomorrow" / "morgen" = current date + 1 day
- "Monday" / "Montag" = next occurrence of that weekday from current date
- Always verify: state the computed date back to the user before writing
- The current date is in your system prompt header — use it, don't guess

### RULE 5: Editing the correct section
Task files have these sections in order:
1. `## Tasks` — task items (- [ ] / - [x])
2. `## Progress Log` — timestamped entries
3. `## Notes / Context`
4. `## Consolidated State`
5. `## Next Step`

When appending tasks, use edit_file to find the LAST task line (- [ ] or - [x]) in `## Tasks` and insert after it, BEFORE `## Progress Log`.

## Important

- If a user asks about day2day commands in natural language, explain the commands listed above.
- The commands are **case-insensitive** and must be the **first word** of the message.
- Task files are stored in the bot system repo under `operations/day2day/tasks/`.
