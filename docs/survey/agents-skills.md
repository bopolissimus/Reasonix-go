# Agents & Skills System

> **Date:** 2026-07-25
> **Source files:** [`internal/skill/`](../../internal/skill/), [`internal/frontmatter/`](../../internal/frontmatter/),
> [`internal/config/paths.go`](../../internal/config/paths.go)

This document covers the skills/agents discovery system, skill frontmatter format,
precedence order, and the agent import story.

---

## 1. Skill Discovery

Skills are invokable markdown playbooks discovered across multiple directory
roots. A `skill.Store` resolves them via a 9-level precedence ladder.

### 1.1 Precedence Order (highest first)

| Priority | Scope | Directory | Description |
|----------|-------|-----------|-------------|
| 1 | project | `./.reasonix/skills/` | Canonical Reasonix project skills |
| 2 | project | `./.agents/skills/` | Cline/Cursor compat |
| 3 | project | `./.agent/skills/` | Aider compat |
| 4 | project | `./.claude/skills/` | Claude Code compat |
| 5 | custom | `[skills] paths` in config | User-configured extras |
| 6 | global | `~/.reasonix/skills/` | Canonical Reasonix global |
| 7 | global | `~/.agents/skills/` | Cline/Cursor compat |
| 8 | global | `~/.agent/skills/` | Aider compat |
| 9 | global | `~/.claude/skills/` | Claude Code compat |
| — | builtin | `(builtin)` | Shipped skills (lowest; overridden by any file) |

**Source:** [`internal/skill/skill.go`](../../internal/skill/skill.go), function `roots()` (line ~210).

### 1.2 Convention Directories

The `config.ConventionDirs` slice (`internal/config/paths.go`, line ~220) defines:

```go
var ConventionDirs = []string{".reasonix", ".agents", ".agent", ".claude"}
```

Each convention directory is scanned with a `skills/` subdirectory appended.
The `.claude` root requires an explicit skill frontmatter marker for flat
`<name>.md` files (since Claude roots also contain ordinary documentation).

### 1.3 Excluded Paths

Paths matching `config.ExcludedPaths` are hidden from discovery. Inside each
root, directories named `assets`, `node_modules`, `references`, `scripts` and
any dotfiles are skipped during recursive scanning.

**Source:** [`internal/skill/skill.go`](../../internal/skill/skill.go), `shouldSkipScanDir()` (line ~340).

---

## 2. Skill File Formats

### 2.1 Directory Layout Skills

```
<name>/
  SKILL.md        ← canonical skill file (required)
  references/     ← optional; appended to body (Anthropic compat)
    foo.md
  scripts/        ← optional; listed in body for bash execution
    install.sh
    analyze.py
  assets/         ← reserved; skipped during discovery
```

A directory is treated as a skill when:
- The directory name is a valid skill identifier (alphanumeric start, 1-64 chars)
- It contains a `SKILL.md` file

### 2.2 Flat Skills (Claude-compat)

```
<name>.md         ← flat skill (requires frontmatter in .claude roots)
```

A flat `.md` file in a `.claude/` root is loaded only when it carries at least
one of these frontmatter keys: `name`, `description`, `runAs`, `context`,
`agent`, `allowed-tools`, `model`, `effort`.

**Source:** [`internal/skill/skill.go`](../../internal/skill/skill.go), `parseFlat()` / `hasSkillMarker()` (line ~430).

### 2.3 Frontmatter Fields

All fields are parsed from YAML-style frontmatter delimited by `---` lines.
Parsed in [`internal/frontmatter/frontmatter.go`](../../internal/frontmatter/frontmatter.go).

| Field | Required | Description |
|-------|----------|-------------|
| `name` | No | Overrides the directory/filename stem. Must be valid identifier. |
| `description` | No | One-liner shown in the Skills index. Missing → skill loads but is invisible in index. |
| `runAs` | No | `inline` (default) or `subagent`. Also triggered by `context: fork` or non-empty `agent:` field. |
| `context` | No | Legacy compat; `fork` value triggers subagent mode. |
| `agent` | No | Legacy compat; non-empty value triggers subagent mode. |
| `allowed-tools` | No | Comma-separated tool names (no wildcards, allowlist only). Scopes subagent's tool registry. |
| `model` | No | Model override for `runAs=subagent` (e.g. `deepseek-chat`). |
| `effort` | No | Effort override for `runAs=subagent` (e.g. `high`, `max`). |

### 2.4 Body Processing

On load, the skill body undergoes two automatic expansions:

1. **Reference injection** — for directory-layout skills, sibling `references/*.md`
   files are appended as `## Reference: <slug>` sections.
2. **Scripts listing** — for directory-layout skills, sibling `scripts/` entries
   with recognized extensions (`.sh`, `.py`, `.js`, `.ts`, `.rb`, `.pl`, `.php`,
   `.ps1`) are listed as a `## Scripts` section with absolute paths.

**Source:** [`internal/skill/skill.go`](../../internal/skill/skill.go), `loadBodyWithReferences()` and `loadBodyWithScripts()` (lines ~530–580).

---

## 3. Skills Index

Only names + descriptions enter the cache-stable system-prompt prefix; bodies
load on demand. The index block is capped at 4000 characters
(`IndexMaxChars`). Format:

```
- explore [🧬 subagent] — Explore the codebase in an isolated subagent...
- test — Run the project's test suite...
```

**Source:** [`internal/skill/index.go`](../../internal/skill/index.go).

---

## 4. Built-in Skills

Shipped as Go constants in [`internal/skill/builtins.go`](../../internal/skill/builtins.go).
Seven built-ins:

| Skill | runAs | Allowed Tools | Description |
|-------|-------|---------------|-------------|
| `init` | inline | (parent loop) | Bootstrap/refresh AGENTS.md |
| `explore` | subagent | read_file, ls, glob, grep, code_index | Read-only codebase investigation |
| `research` | subagent | read_file, ls, glob, grep, code_index, web_fetch | Web + code research |
| `review` | subagent | read_file, ls, glob, grep, code_index, bash | Code review on branch diff |
| `security-review` | subagent | read_file, ls, glob, grep, code_index, bash | Security-focused review |
| `test` | inline | (parent loop) | Run tests, diagnose, fix, re-run |
| `install-capability` | inline | (parent loop) | Install MCP servers and skills |

User/project files with the same name override built-ins.

---

## 5. Skill Tools

### 5.1 `run_skill` ([`internal/skill/tools.go`](../../internal/skill/tools.go))

General skill invocation. Supports:
- `name` — skill identifier (bare name; `[🧬 subagent]` tags are stripped)
- `arguments` — free-form task; required for subagent skills
- `continue_from` — resume a prior subagent transcript (`sa_...` value)
- `fork_from` — fork from a prior transcript (mutually exclusive with continue)

### 5.2 `read_skill` ([`internal/skill/tools.go`](../../internal/skill/tools.go))

Read-only inline-skill loader. Works in plan mode. Rejects subagent-tagged
skills (they must be executed, not read).

### 5.3 `read_only_skill` ([`internal/skill/tools.go`](../../internal/skill/tools.go))

Plan-mode-safe skill entry point. Inline skills are loaded into context;
subagent skills run in a restricted read-only subagent with no writer tools,
no installers, no memory mutation, no continuation/fork, no background jobs,
and no delegation.

### 5.4 `install_skill` ([`internal/skill/tools.go`](../../internal/skill/tools.go))

Author and save a new skill. Supports `scope: project|global`, `runAs`,
`model`, `effort`, `allowedTools`. Writes canonical `<name>/SKILL.md`.

### 5.5 Dedicated Subagent Wrappers ([`internal/skill/tools.go`](../../internal/skill/tools.go))

Top-level tools wrapping built-in subagent skills, named for natural model
selection:

- `explore` → skill "explore"
- `research` → skill "research"
- `review` → skill "review"
- `security_review` → skill "security-review"

Each is skipped when the underlying skill is disabled or overridden as inline.

---

## 6. Agent Import Story

The Go Reasonix project does not import skills from DR1 TypeScript; instead,
skills are authored directly in markdown (as SKILL.md / <name>.md files) and
discovered from the convention directories listed above.

### 6.1 Agent Directories at Project Root

The project at `/home/dev/reasonix/go1` has `.agents/` at the project root
containing:

| File | Purpose |
|------|---------|
| `.agents/AGENTS.md` | Project memory doc (loaded into every session) |
| `.agents/dependency-security.md` | Dependency audit policy (human-read) |
| `.agents/git.md` | Git workflow guidance (human-read) |
| `.agents/jira-search.md` | Jira search patterns (human-read) |
| `.agents/writing-agent-docs.md` | Agent documentation style (human-read) |

These are pure markdown files in the `.agents/` convention directory. The
`AGENTS.md` file is discovered by `internal/memory/` as a hierarchical doc;
the other `.md` files are not skill-formatted and are loaded only via
`read_file` by the model.

### 6.2 Skill Directory

The project also has `.reasonix/` at the root, which contains:
- `.reasonix/commands/review.md` — a slash command
- `.reasonix/truncated-results/` — cached truncated tool output
- `.reasonix/config.json` — additional tool configuration

No custom skills are currently registered under `.reasonix/skills/`.

### 6.3 Sub-Agent Tool Boundaries

When a subagent is spawned via `task`, `run_skill` (subagent), `read_only_task`,
`read_only_skill`, `explore`, `research`, `review`, or `security_review`, the
parent's tool registry is filtered:

**Excluded entirely:**
- Metatools: `task`, `read_only_task`, `parallel_tasks`, `run_skill`, `read_only_skill`, `read_skill`, `install_skill`, `install_source`, `explore`, `research`, `review`, `security_review`
- Job tools: `wait`, `bash_output`, `kill_shell`
- Workflow tools (read-only subagents only): `connect_tool_source`

**Bash restricted to foreground only** — subagents cannot spawn background
jobs, preserving the session's process isolation.

**Source:** [`internal/agent/task.go`](../../internal/agent/task.go), `SubagentToolRegistry()` (line ~80).

### 6.4 Allowed-Tools Scoping

When a skill declares `allowed-tools` in its frontmatter, the subagent's
registry is further restricted to only those tool names (intersected with the
parent's available tools). This allows skill authors to constrain a subagent
to, say, only `read_file, grep, glob` without any `bash` access.

---

## 7. Skill File Lifecycle

```
                          ┌──────────────────┐
                          │  Skill Directory  │
                          │  (.reasonix/     │
                          │   skills/<name>/ │
                          │   SKILL.md)       │
                          └────────┬─────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    ▼                             ▼
            ┌──────────────┐            ┌──────────────────┐
            │  Store.List() │            │   Store.Read()   │
            │ (discovery)   │            │ (by name lookup) │
            │ scan roots →  │            │ scan + parse     │
            │ all skills    │            │ single file      │
            └──────┬───────┘            └────────┬─────────┘
                   │                             │
                   ▼                             ▼
            ┌──────────────┐            ┌──────────────────┐
            │  skill.Index │            │  Body:           │
            │  Block()     │            │  - references/*  │
            │  (names +    │            │  - scripts/*     │
            │   descs only)│            │  appended        │
            └──────┬───────┘            └────────┬─────────┘
                   │                             │
                   ▼                             ▼
            ┌────────────────────────┐   ┌──────────────────┐
            │ System prompt prefix   │   │ run_skill /      │
            │ (cache-stable)         │   │ read_skill tool  │
            └────────────────────────┘   └──────────────────┘
                    ↑ cache-stable               │
                    │ (names only)               ▼
                    │                     ┌──────────────────┐
                    │                     │  Subagent spawn  │
                    │                     │  (if runAs=      │
                    │                     │   subagent) or   │
                    │                     │  inline result   │
                    │                     └──────────────────┘
```

---

## 8. Key Source Files

| File | Purpose |
|------|---------|
| [`internal/skill/skill.go`](../../internal/skill/skill.go) | Store, discovery, parsing, Create, path resolution |
| [`internal/skill/tools.go`](../../internal/skill/tools.go) | run_skill, read_skill, read_only_skill, install_skill, subagent wrappers |
| [`internal/skill/index.go`](../../internal/skill/index.go) | IndexBlock rendering, ApplyIndex to system prompt |
| [`internal/skill/builtins.go`](../../internal/skill/builtins.go) | 7 built-in skill bodies (explore, research, review, security-review, test, init, install-capability) |
| [`internal/frontmatter/frontmatter.go`](../../internal/frontmatter/frontmatter.go) | YAML frontmatter parser |
| [`internal/config/paths.go`](../../internal/config/paths.go) | ConventionDirs, CommandDirs, SkillCustomPaths |
| [`internal/agent/task.go`](../../internal/agent/task.go) | Subagent registry filtering, tool boundaries |
| [`internal/config/config.go`](../../internal/config/config.go) | SkillsConfig, IsValidSkillName, SkillNameKey |
