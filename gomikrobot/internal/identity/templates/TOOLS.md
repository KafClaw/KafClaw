# Tool Reference

## File Operations

### read_file
Read the contents of a file.
- **path** (string, required): File path relative to work repo, or absolute with `!` prefix.

### write_file
Write content to a file (creates or overwrites).
- **path** (string, required): File path relative to work repo.
- **content** (string, required): File content to write.

### edit_file
Apply a targeted edit to an existing file.
- **path** (string, required): File path relative to work repo.
- **old** (string, required): Exact text to find.
- **new** (string, required): Replacement text.

## Shell Execution

### exec
Execute a shell command.
- **command** (string, required): The command to run.
- **timeout** (int, optional): Timeout in seconds (default: 60, max: 300).

**Safety notes:**
- Deny-pattern filtering blocks destructive commands (`rm -rf /`, `chmod 777`, `mkfs`, `shutdown`, fork bombs, etc.)
- In strict mode, only allow-listed command patterns are permitted.
- By default, execution is restricted to the work repo directory.

## Web Operations

### web_search
Search the web using Brave Search.
- **query** (string, required): Search query.
- **max_results** (int, optional): Maximum results (default: 10).

### web_fetch
Fetch and extract content from a URL.
- **url** (string, required): URL to fetch.

## Memory Operations

### remember
Store a fact or observation in long-term semantic memory.
- **content** (string, required): The information to remember.
- **source** (string, optional): Category or source label.

### recall
Search semantic memory for relevant context.
- **query** (string, required): What to search for.
- **limit** (int, optional): Max results (default: 5).

## Messaging

### message
Send a message to a channel.
- **channel** (string, required): Target channel (e.g., "whatsapp", "cli").
- **chat_id** (string, required): Chat identifier.
- **content** (string, required): Message text.
