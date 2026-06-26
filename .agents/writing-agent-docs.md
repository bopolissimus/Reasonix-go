# Writing topic-specific agent docs

`.agents/` holds markdown files that encode project-specific knowledge for LLMs.
Each file covers one topic — a subsystem, a pattern, a convention.  The agent reads
them on demand, not eagerly.

## When to split out a topic file

Write a separate `.agents/<topic>.md` when:

- The guidance is **more than a paragraph** — inline in AGENTS.md would bloat the system prompt
- The topic is **conditional** — only relevant when working on a specific subsystem
- The pattern is **reusable** — future agents will hit the same problem again
- The guidance includes **code examples, file paths, or step-by-step instructions** that need room to breathe

Keep it inline in `.agents/AGENTS.md` when:

- It's a **standing rule** that applies to every session (tone, git policy, autopilot gating)
- It's **one sentence** — a pointer is more overhead than the content
- It's the **routing table itself** — the instructions for when to read which file

## File structure

```markdown
# Short title (will appear in search / routing)

One-sentence summary of what this file covers.

## When to read this

Concrete triggers: "when adding a new MCP transport", "when modifying the session
store", "when debugging TUI rendering issues".  The agent uses this to decide
whether to `read_file` the topic file.

## Relevant files

A bullet list of key source files with brief notes:

- `src/mcp/transports/` — transport implementations (stdio, SSE, streamable-http)
- `src/memory/session.ts` — session storage and metadata

## Guidance

The actual instructions, patterns, examples.  Be specific with file paths and
code snippets.  Assume the reader is an LLM that needs concrete direction, not
abstract principles.
```

## How AGENTS.md routes to topic files

In `.agents/AGENTS.md`, add a **routing section** near the top that lists topic
files with one-liner descriptions and when to read them:

```markdown
## Topic files (read on demand)

Before working on a subsystem, check whether a topic file exists:

| File | Covers | Read when… |
|---|---|---|
| `tables.md` | Ink TUI table/column alignment | Adding or modifying multi-column lists in the terminal UI |
| `jira-search.md` | Jira query patterns to avoid truncation | Searching Jira for issues |

Read `read_file(".agents/<topic>.md")` before touching the relevant code.
```

## Routing principles

- **One sentence per entry** — the agent scans the table, not the full file
- **Concrete triggers** — "when adding X" or "when modifying Y", not "for X-related work"
- **Read before touch** — the rule is to read the topic file BEFORE modifying the code, not after hitting a problem
- **Keep the table short** — if it grows past ~10 entries, split into subdirectories or group related topics

## Discovery

Agents discover these files by:
1. Reading `.agents/AGENTS.md` at session start (pinned in system prompt)
2. Scanning the routing table for relevant topics
3. Calling `read_file(".agents/<topic>.md")` when a trigger matches

The files are NOT auto-loaded — the agent must explicitly read them.  This keeps
the system prompt small while making deep knowledge available on demand.
