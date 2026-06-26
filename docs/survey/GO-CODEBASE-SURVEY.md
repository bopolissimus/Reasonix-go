# Go Reasonix Codebase Survey

> **Revision:** 2026-07-26  
> **Project:** Reasonix — Go rewrite of the original TypeScript DR1 Reasonix  
> **Binary:** ~20 MB static binary (vs ~200 MB Node.js/DR1)  
> **Build:** `go build -mod=vendor ./...`  
> **Config:** TOML (reasonix.toml)  
> **TUI:** [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) (Elm architecture) vs Ink (React) in DR1  
> **Frontends:** Terminal TUI, HTTP/SSE serve, Wails desktop, IM bots (Feishu, WeChat, QQ)

---

## Master Index

1. [Control — Transport-Agnostic Session Controller](#1-control--transport-agnostic-session-controller)
2. [Agent — Agent Loop, Sub-Agent Spawning, Compaction, Plan Mode](#2-agent--agent-loop-sub-agent-spawning-compaction-plan-mode)
3. [Tool & Registry — Tool Interface, Registration, MCP Prefixing](#3-tool--registry--tool-interface-registration-mcp-prefixing)
4. [Built-in Tools — 19 Compile-Time Built-In Tools](#4-built-in-tools--19-compile-time-built-in-tools)
5. [Config — TOML Config, Provider Entries, Security Config](#5-config--toml-config-provider-entries-security-config)
6. [Boot — Application Assembly (Controller Builder)](#6-boot--application-assembly-controller-builder)
7. [Provider — OpenAI-Compatible + Anthropic Providers, Retry Logic](#7-provider--openai-compatible--anthropic-providers-retry-logic)
8. [Skill — Skill Discovery, Frontmatter Parsing, Index](#8-skill--skill-discovery-frontmatter-parsing-index)
9. [Memory — Hierarchical Memory (REASONIX.md/AGENTS.md/CLAUDE.md)](#9-memory--hierarchical-memory-reasonixmdagentsmdclaudemd)
10. [Memory Compiler — Memory v5 Session-Based Compilation](#10-memory-compiler--memory-v5-session-based-compilation)
11. [Plugin — MCP Client (stdio/HTTP/SSE Transports)](#11-plugin--mcp-client-stdiohttpsse-transports)
12. [Security — ContentSanitizer, Honeytokens, AnomalyDetector, AuditLog, Output Validation](#12-security--contentsanitizer-honeytokens-anomalydetector-auditlog-output-validation)
13. [CLI / TUI — Bubble Tea Chat Terminal, Slash Commands, Sub-Commands](#13-cli--tui--bubble-tea-chat-terminal-slash-commands-sub-commands)
14. [Event — Typed Event Stream & Sink Interface](#14-event--typed-event-stream--sink-interface)
15. [Sandbox — macOS Seatbelt Confinement, Bash Sandbox](#15-sandbox--macos-seatbelt-confinement-bash-sandbox)
16. [Hook — 10 Hook Events (PreToolUse, PostToolUse, PostLLMCall, etc.)](#16-hook--10-hook-events-pretooluse-posttooluse-postllmcall-etc)
17. [Permission — Permission Policy Engine, Bash Read-Only Classification](#17-permission--permission-policy-engine-bash-read-only-classification)
18. [Plan Mode — Plan Mode Policy, Bash Safety Gating, Reconciliation Tests](#18-plan-mode--plan-mode-policy-bash-safety-gating-reconciliation-tests)
19. [Checkpoint — File Checkpoint/Rewind Store](#19-checkpoint--file-checkpointrewind-store)
20. [i18n — EN, zh-CN, zh-TW Messages](#20-i18n--en-zh-cn-zh-tw-messages)
21. [LSP — Language Server Protocol Tools](#21-lsp--language-server-protocol-tools)
22. [CLI Sub-Commands — `reasonix`, `serve`, `bot`, `acp`, etc.](#22-cli-sub-commands--reasonix-serve-bot-acp-etc)
23. [Desktop — Wails Desktop Application](#23-desktop--wails-desktop-application)
24. [mcp-web-search — Standalone MCP Web Search Server](#24-mcp-web-search--standalone-mcp-web-search-server)

---

## 1. Control — Transport-Agnostic Session Controller

**Package:** [`internal/control/`](internal/control/)  
**Key File:** [`controller.go`](internal/control/controller.go) (3262 lines)

The `Controller` is the single transport-agnostic session driver. It owns the agent run loop and session lifecycle, takes commands (`Send`/`Cancel`/`Approve`/`SetPlanMode`/`Compact`/`NewSession`/…), and emits everything — reasoning, tool calls, approvals, turn completion — as a typed event stream to an `event.Sink`. Every frontend (terminal TUI, desktop webview, HTTP/SSE serve, IM bot) drives the Controller identically.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Controller` struct | L66 | Main session driver; owns agent runner, executor, skill set, tool registry, session state |
| `Options` struct | L219 | Configures controller: runner, executor, sink, policy, label, model ref, session dir |
| `New(opts Options)` | L280 | Constructor — wires agent, tool registry, plugin host, skill store, memory |
| `Send(ctx, msg)` | L444 | Submit a user message → runs a turn |
| `RunTurn(ctx, msg)` | L482 | Synchronous turn execution (used by serve/bot frontends) |
| `Approve(ctx, id)` | — | Resolve a pending tool approval (interactive frontends call this) |
| `AnswerQuestion(ctx, id, answers)` | — | Resolve a pending `ask` tool question |
| `SetPlanMode(enabled)` | — | Toggle plan mode mid-session |
| `Compact()` | — | Force context compaction |
| `NewSession()` | — | Start a fresh session (pinned `/new`) |
| `Close()` | — | Clean up: close plugin host, cleanup pending reconciler |
| `approvalReply` / `pendingApproval` | L157–164 | Internal approval state tracking |
| `pendingAsk` | L175 | Internal `ask` tool state tracking |
| `RuntimeStatus` | L188 | Live session status (model, branch, plan mode, etc.) |

### Cross-References
- Calls into `agent.Runner` and `agent.Agent` for turn execution
- Receives commands from frontends via the `command.Command` queue
- Owns `skillSet` (wraps `skill.Store` + runtime overrides)
- Wires `permission.Policy` + `permission.Gate` for tool authorization
- Uses `checkpoint.Store` for edit safety net
- Uses `memory.Set` for persistent memory
- Uses `plugin.Host` for MCP connectivity
- Uses `hook.Runner` for hook execution

### Tests
19 test files in the package:
- [`controller_test.go`](internal/control/controller_test.go) — core controller tests
- [`approval_e2e_test.go`](internal/control/approval_e2e_test.go) — end-to-end approval flow
- [`auto_plan_test.go`](internal/control/auto_plan_test.go) — auto-plan classification
- [`goal_test.go`](internal/control/goal_test.go) — goal machine
- [`rewind_e2e_test.go`](internal/control/rewind_e2e_test.go) — checkpoint rewind
- [`turn_orchestrator_test.go`](internal/control/turn_orchestrator_test.go) — turn lifecycle

### Goal Machine

**File:** [`goal.go`](internal/control/goal.go)  
The controller maintains a "goal" — a one-line directive the user sets via `/goal` — that is persisted as a memory fact and re-injected when the session resumes. The `Goal` struct manages: set, clear, current state, and memory persistence.

### Auto-Plan Classifier

**File:** [`auto_plan_classifier.go`](internal/control/auto_plan_classifier.go)  
Heuristic classifier that auto-activates plan mode based on the user's prompt: contains phrases like "plan", "design", "approach", "architecture". Uses a `PromptClassification` enum: `ClassPlan`, `ClassExecute`, `ClassUncertain`.

### Approval Manager

**File:** [`approval.go`](internal/control/approval.go)  
Manages tool-call approval flow: creates `pendingApproval` entries, tracks `granted` session-level and turn-level approvals (`map[string]bool`), and resolves auto-approvals for tools the user already blessed.

### Slash Commands

**File:** [`slash.go`](internal/control/slash.go)  
Bridges user-facing slash commands (`/model`, `/compact`, `/clear`, `/help`, etc.) to controller actions. Maintains a `SlashHandler` registry and dispatches parsed commands.

### Branches

**File:** [`branches.go`](internal/control/branches.go)  
Multi-branch conversation support: `Branch`, `SwitchBranch`, `ListBranches` — each branch has its own conversation history and checkpoint store.

---

## 2. Agent — Agent Loop, Sub-Agent Spawning, Compaction, Plan Mode

**Package:** [`internal/agent/`](internal/agent/)  
**Key File:** [`agent.go`](internal/agent/agent.go) (2312 lines)

The core agent loop that drives model interactions. Contains the `Agent` struct (the executor) and the `Runner` interface (the planner/coordinator). Handles tool call dispatch, streaming, compaction, plan mode enforcement, and sub-agent spawning.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Agent` struct | L177 | The executor — owns provider connection, tool registry, event sink, message buffer |
| `Renderer` interface | L44 | Redraws final-answer text as styled output |
| `Asker` interface | L53 | Puts multiple-choice questions to the user (`ask` tool) |
| `Gate` interface | L149 | Tool-call authorization gate (allow/deny/ask) |
| `ToolHooks` struct | L158 | Pre/post tool-call hook runners |
| `Agent.Options` | — | Max steps, temperature, pricing, tool registry |
| `New()` | — | Agent constructor |
| `Stream(ctx, msg)` | — | Main turn execution — streams completion, dispatches tool calls |
| `Compact()` | — | Context compaction (summarizer) |
| `TaskTool` | [`task.go`](internal/agent/task.go) L81+ | Sub-agent spawning tool (`task` and `parallel_tasks`) |

### Agent Loop Architecture

The `Runner` interface abstracts the 2-model coordinator (planner + executor) from a single-model agent. The loop:

1. User message → system prompt + history → model stream
2. Model produces text/reasoning/tool_calls
3. Tool calls dispatched via `Agent.executeOne` (sequential for writers, parallel for read-only)
4. Results fed back to model
5. Repeat until turn limits met or model stops

### Task / Sub-Agent Tool

**File:** [`task.go`](internal/agent/task.go) (814 lines)  
The `TaskTool` spawns an isolated sub-agent: creates a fresh session with a subset of tools (no meta tools, no job tools), runs it to completion, and returns only the final answer. Supports:
- `task` — general sub-agent (writer-capable)
- `read_only_task` — read-only sub-agent (plan-mode-safe)
- `parallel_tasks` — multiple sub-agents concurrently (`parallel_tasks.go`)

### Compaction

**File:** [`compact.go`](internal/agent/compact.go)  
Context compaction via summarizer: folds old messages into a summary when the conversation exceeds the context window threshold. Supports `auto` (threshold-triggered) and `manual` (user-invoked) modes. Uses the same provider for summarization.

### Cache Shape

**File:** [`cache_shape.go`](internal/agent/cache_shape.go)  
Manages the cache-stable system prompt prefix: hashes the prefix, detects changes, and emits `CacheDiagnostics` so frontends can show cache-churn attribution. Critical for DeepSeek prefix caching.

### Plan Mode

The agent enforces plan mode at the gate call site: checks `planmode.Policy.Decide()` before every tool call, refusing writer tools during planning.

### Nuclear / YOLO

**File:** [`nuclear_test.go`](internal/agent/nuclear_test.go)  
Tests for the "NUCLEAR-YOLO" emergency bypass mode.

### Tests
28 test files (one of the most heavily tested packages):
- [`loop_e2e_test.go`](internal/agent/loop_e2e_test.go) — end-to-end agent loop
- [`compact_loop_e2e_test.go`](internal/agent/compact_loop_e2e_test.go) — compaction in the loop
- [`cachehit_e2e_test.go`](internal/agent/cachehit_e2e_test.go) — cache hit behavior
- [`parallel_tasks_test.go`](internal/agent/parallel_tasks_test.go) — parallel sub-agents
- [`planmode_test.go`](internal/agent/planmode_test.go) — plan mode enforcement
- [`nuclear_test.go`](internal/agent/nuclear_test.go) — nuclear bypass
- [`reasoning_language_test.go`](internal/agent/reasoning_language_test.go) — reasoning language
- [`coordinator_test.go`](internal/agent/coordinator_test.go) — 2-model coordinator

---

## 3. Tool & Registry — Tool Interface, Registration, MCP Prefixing

**Package:** [`internal/tool/`](internal/tool/)  
**Key File:** [`tool.go`](internal/tool/tool.go) (370 lines)

Defines the `Tool` interface and the per-run `Registry`. Built-in tools self-register via `init()` and blank imports; plugin-provided tools are added to a runtime Registry.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Tool` interface | L16 | `Name()`, `Description()`, `Schema()`, `Execute()`, `ReadOnly()` |
| `Previewer` interface | L30 | Optional: preview a writer tool's change without touching disk |
| `PlanModeClassifier` interface | L49 | Optional: declare plan mode stance |
| `PlanModeUntrustedReadOnly` interface | L62 | Marks externally-sourced readOnly hint as untrusted |
| `Registry` struct | L83 | Per-run tool set: built-ins + plugins; thread-safe |
| `RegisterBuiltin(t)` | L72 | Compile-time registration (panics on duplicate) |
| `Builtins()` | L78 | Returns all registered built-ins sorted by name |
| `MCPNamePrefix` | L106 | `"mcp__"` — namespace every MCP tool name carries |
| `SplitMCPName(name)` | L111 | Splits `"mcp__<server>__<tool>"` |
| `RemovePrefix(prefix)` | L120 | Unregisters all tools with a prefix (MCP disconnect) |

### Registry Semantics
- Every frontend gets its own `*Registry`
- Built-ins registered via `init()` in [`internal/tool/builtin/`](internal/tool/builtin/)
- MCP tools namespaced `mcp__<server>__<tool>` to avoid collisions
- Schema is canonicalized once at registration time
- Thread-safe (`sync.RWMutex`) for concurrent MCP hot-add

### Tests
Tool interface tests in [`tool_test.go`](internal/tool/tool_test.go) and diff-related tests.

---

## 4. Built-in Tools — 19 Compile-Time Built-In Tools

**Package:** [`internal/tool/builtin/`](internal/tool/builtin/)  
**Files:** 30+ `.go` files, each implementing one or more tools

The 19 built-in tools (Go has 43 total tool surfaces when counting subagent wrappers and LSP):

| Tool | File | Lines | ReadOnly | Description |
|---|---|---|---|---|
| `bash` | [`bash.go`](internal/tool/builtin/bash.go) | ~500 | false | Shell command execution. Deep safety: quoting-aware parsing, argument-level gating, sandbox confinement via `confine` |
| `read_file` | [`readfile.go`](internal/tool/builtin/readfile.go) | ~200 | true | File read with line offset/limit, UTF-16 detection, streaming |
| `write_file` | [`writefile.go`](internal/tool/builtin/writefile.go) | ~200 | false | Write/overwrite file content |
| `edit_file` | [`editfile.go`](internal/tool/builtin/editfile.go) | ~150 | false | SEARCH/REPLACE with exact text anchoring |
| `multi_edit` | [`multiedit.go`](internal/tool/builtin/multiedit.go) | ~100 | false | Atomic multi-edit across files |
| `move_file` | [`movefile.go`](internal/tool/builtin/movefile.go) | ~80 | false | Rename/move files |
| `glob` | [`glob.go`](internal/tool/builtin/glob.go) | ~80 | true | File pattern matching (e.g. `**/*.go`) |
| `grep` | [`grep.go`](internal/tool/builtin/grep.go) | ~100 | true | Content regex search (rg/zgrep/fallback) |
| `ls` | [`ls.go`](internal/tool/builtin/ls.go) | ~50 | true | Directory listing |
| `web_fetch` | [`webfetch.go`](internal/tool/builtin/webfetch.go) | ~150 | true | HTTP fetch with SSRF protection |
| `todo_write` | [`todo.go`](internal/tool/builtin/todo.go) | ~100 | false | Structured task list with levels |
| `complete_step` | [`completestep.go`](internal/tool/builtin/completestep.go) | ~80 | true (plan-only false) | Evidence-backed plan step completion |
| `bgjobs` / `bash_output` / `wait` / `kill_shell` | [`bgjobs.go`](internal/tool/builtin/bgjobs.go) | ~250 | mixed | Background job management |
| `confine` | [`confine.go`](internal/tool/builtin/confine.go) | ~80 | false | Workspace/sandbox confinement toggle |
| `notebook_edit` | [`notebookedit.go`](internal/tool/builtin/notebookedit.go) | ~100 | false | Edits a Jupyter notebook cell |
| `delete_range` | [`delete_range.go`](internal/tool/builtin/delete_range.go) | ~80 | false | Delete text range by start/end anchor |
| `delete_symbol` | [`delete_symbol.go`](internal/tool/builtin/delete_symbol.go) | ~80 | false | Delete Go symbol via AST parsing |
| `code_index` | [`codeindex.go`](internal/tool/builtin/codeindex.go) | ~150 | true | Go AST-based symbol outline + definition candidates |
| `workspace` | [`workspace.go`](internal/tool/builtin/workspace.go) | ~30 | true | Return workspace root path |

### Additional Tool Sources

| Source | Package | Tools |
|---|---|---|
| Agent meta-tools | `internal/agent/` | `task`, `read_only_task`, `parallel_tasks`, `ask` |
| Skill tools | `internal/skill/tools.go` | `run_skill`, `read_only_skill`, `read_skill`, `install_skill`, `explore`, `research`, `review`, `security_review` |
| Memory tools | `internal/memory/` | `remember`, `forget`, `recall_memory` |
| History tools | `internal/history/` | `session` (list + read past sessions) |
| LSP tools | `internal/lsp/` | `lsp_definition`, `lsp_references`, `lsp_hover`, `lsp_diagnostics` |
| Slash command tool | `internal/command/` | `slash_command` |
| Install source tool | `internal/installsource/` | `install_source` |
| Misc | Various | `connect_tool_source`, `read_session`, `list_sessions` |

### Tests
Each tool has its own test file (30+ test files). Notable:
- [`bash_test.go`](internal/tool/builtin/bash_test.go) — bash execution, timeout, cancellation
- [`bash_cancel_test.go`](internal/tool/builtin/bash_cancel_test.go) — graceful tool cancellation
- [`crlf_edit_test.go`](internal/tool/builtin/crlf_edit_test.go) — CRLF handling in edits
- [`gitignore_test.go`](internal/tool/builtin/gitignore_test.go) — gitignore filtering
- [`preview_test.go`](internal/tool/builtin/preview_test.go) — preview change computation
- [`webfetch_test.go`](internal/tool/builtin/webfetch_test.go) — SSRF testing
- [`write_tool_test.go`](internal/tool/builtin/write_tool_test.go) — file writer safety

---

## 5. Config — TOML Config, Provider Entries, Security Config

**Package:** [`internal/config/`](internal/config/)  
**Key Files:** [`config.go`](internal/config/config.go) (500+ lines), [`paths.go`](internal/config/paths.go) (200+ lines)

TOML-based configuration. Resolution: flag > `./reasonix.toml` > `~/.reasonix/config.toml` > legacy XDG > built-in defaults. API keys come from environment variables (never stored in config files).

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Config` struct | config.go L80 | Root config: model, providers, tools, permissions, sandbox, network, plugins, skills, security |
| `UIConfig` | L124 | Theme, style, shortcut layout, close behavior |
| `DesktopConfig` | L145 | Desktop-only: language, layout, theme, status bar |
| `ProviderEntry` | config.go | Per-provider: name, kind, model, base_url, api_key_env, pricing |
| `ToolsConfig` | — | `bash_timeout`, `enabled` slice, `search` engine, `shell` |
| `PermissionsConfig` | — | Mode (allow/ask/deny), allow/ask/deny rule lists |
| `SandboxConfig` | — | Workspace root, allow_write extras, bash mode (enforce/off) |
| `SecurityConfig` | — | Sanitization, honeytokens, anomaly, audit config |
| `SkillsConfig` | — | Custom paths, excluded paths, disabled skills, max depth |
| `LSPConfig` | — | Enabled flag + per-language server overrides |
| `BotConfig` | — | Multi-channel IM bot config (Feishu, WeChat, QQ) |
| `ServeConfig` | — | HTTP auth mode (none/token/password) |
| `NetworkConfig` | — | Proxy mode, proxy URL, no-proxy bypass |
| `Plugins` | — | MCP plugin specs |
| `Load()` / `LoadForRoot(root)` | load.go | Load and merge configs |
| `SourcePath()` / `SourcePathForRoot(root)` | paths.go | Resolve highest-priority config file |
| `ResolveSystemPrompt()` | config.go | Template resolution for system prompt |
| `ResolveModel(name)` | config.go | Look up a model entry by name |
| `SessionDir()` | paths.go | `~/.reasonix/sessions/` |
| `ArchiveDir()` | paths.go | `~/.reasonix/archive/` |
| `MemoryUserDir()` | paths.go | `~/.reasonix/` — root for memory, projects |
| `CacheDir()` | paths.go | `~/.cache/reasonix/` |
| `CommandDirs()` | paths.go | Convention dirs for slash commands |
| `ConventionDirs` | paths.go | `[".reasonix", ".agents", ".agent", ".claude"]` |
| `UserCredentialsPath()` | paths.go | `~/.reasonix/.env` |
| `ExpandVars()` | expand.go | `${VAR}` / `${VAR:-default}` expansion |

### Config Resolution Flow
1. `REASONIX_HOME` env var → overrides home
2. `./reasonix.toml` → project-level, highest priority
3. `~/.reasonix/config.toml` → user-global
4. Legacy XDG paths → automatic migration
5. Built-in defaults for every field

### Config Migration
**File:** [`migrate.go`](internal/config/migrate.go)  
Auto-migration from DR1/v0.5 legacy config files on first boot. Handles:
- Legacy `~/.reasonix/config.json` → TOML
- Legacy provider format → new provider entries
- Renamed fields with compatibility shims

### Tests
19 test files:
- [`config_test.go`](internal/config/config_test.go) — core config loading
- [`paths_test.go`](internal/config/paths_test.go) — path resolution
- [`backup_test.go`](internal/config/backup_test.go) — config backup rotation
- [`migrate_test.go`](internal/config/migrate_test.go) — migration
- [`model_fallback_test.go`](internal/config/model_fallback_test.go) — model fallback
- [`proxy_test.go`](internal/config/proxy_test.go) — proxy configuration

---

## 6. Boot — Application Assembly (Controller Builder)

**Package:** [`internal/boot/`](internal/boot/)  
**Key File:** [`boot.go`](internal/boot/boot.go) (800+ lines)

The single assembly point that turns user configuration into a ready-to-drive `control.Controller`. Every frontend — terminal TUI, HTTP/SSE server, desktop webview — calls `Build()` and gets a fully wired controller.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Build(ctx, opts)` | boot.go | Assembles everything: config, provider, tools, plugins, memory, skills, hooks, gate |
| `Options` struct | L93 | Per-run knobs: model, max steps, sink, workspace root, extra plugins, token mode |
| `ErrUnknownModel` | L28 | Sentinel error for unresolved model |
| `NewProviderWithProxy(entry, spec)` | — | Builds a provider with proxy configuration |
| `LSPSpecs(cfg)` | — | Resolves LSP config to server specs |
| `PluginSpecsForRoot(entries, root)` | — | Resolves MCP plugin specs with path expansion |

### Build Order
1. Load config (with optional legacy migration)
2. Resolve model → create provider
3. Resolve system prompt + language policy + output style
4. Load memory (REASONIX.md / AGENTS.md / auto-memory)
5. Discover skills → fold index into system prompt
6. Create tool registry → register built-ins
7. Start MCP plugins (eager + background tiers)
8. Register LSP tools
9. Wire permission gate + hook runner
10. Construct `task` tool (sub-agent support)
11. Register memory tools, history tools, skill tools
12. Construct Controller

### Token Economy Mode
When `TokenMode=economy`, optional tool sources (skills index, MCP, LSP, web_fetch, install_source, task) are hidden behind `connect_tool_source` — a connector tool that lazily enables them.

---

## 7. Provider — OpenAI-Compatible + Anthropic Providers, Retry Logic

**Package:** [`internal/provider/`](internal/provider/)  
**Key Files:** [`provider.go`](internal/provider/provider.go) (350+ lines), [`retry.go`](internal/provider/retry.go)

Abstracts model backends behind a `Provider` interface. Concrete implementations self-register via `init()`.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Provider` interface | L192 | `Name()`, `Stream(ctx, req)` |
| `Factory` func | L237 | `func(cfg Config) (Provider, error)` |
| `Register(kind, factory)` | L242 | Self-registration in `init()` |
| `Request` struct | L150 | Messages + tools + temperature + max_tokens |
| `Message` struct | L89 | Role + content + images + reasoning + tool_calls |
| `Chunk` struct | L175 | Streamed event: text, reasoning, tool call, usage, error |
| `Usage` struct | L120 | Token accounting with cache hit/miss |
| `Pricing` struct | L133 | Per-1M-token rates + cost estimation |
| `ToolSchema` struct | L145 | Tool definition for the model API |
| `StreamInterruptedError` | L187 | Recoverable transport cut |
| `AuthError` | L220 | API key rejection (401/403) |
| `NormalizeMessages(msgs)` | L167 | Repair tool-call pairing for wire safety |
| `SanitizeToolPairing(msgs)` | L161 | Wire-preparation alias for NormalizeMessages |

### Concrete Providers

| Subpackage | File | Details |
|---|---|---|
| `provider/openai/` | [`openai.go`](internal/provider/openai/openai.go) | OpenAI-compatible: streaming, tool calls, thinking, effort, reconnect |
| `provider/anthropic/` | [`anthropic.go`](internal/provider/anthropic/anthropic.go) | Anthropic API: thinking signature, image support, stall detection |
| `provider/retry.go` | [`retry.go`](internal/provider/retry/retry.go) | Exponential backoff retry for transient failures |

### Provider Features
- **Thinking mode:** DeepSeek's `reasoning_content` + Anthropic's signed thinking blocks
- **Effort:** Maps `low`/`medium`/`high`/`xhigh`/`max` to provider-specific parameters
- **Cache:** Normalizes DeepSeek cache tokens + OpenAI `prompt_tokens_details`
- **Stall detection:** Hard timeout on first byte for streamed completions
- **Reconnect:** Attempts reconnection on transient stream cuts
- **Schema canonicalization:** Normalizes JSON Schema across providers

### Tests
15+ test files across provider subpackages:
- [`provider_test.go`](internal/provider/provider_test.go) — core provider tests
- [`openai_test.go`](internal/provider/openai/openai_test.go) — OpenAI streaming
- [`anthropic_test.go`](internal/provider/anthropic/anthropic_test.go) — Anthropic API
- [`retry_test.go`](internal/provider/retry_test.go) — retry logic
- [`think_test.go`](internal/provider/openai/think_test.go) — thinking mode
- [`stall_test.go`](internal/provider/openai/stall_test.go) — stall detection

---

## 8. Skill — Skill Discovery, Frontmatter Parsing, Index

**Package:** [`internal/skill/`](internal/skill/)  
**Key Files:** [`skill.go`](internal/skill/skill.go) (500+ lines), [`index.go`](internal/skill/index.go) (80 lines), [`tools.go`](internal/skill/tools.go) (200+ lines)

Discovers invokable playbooks ("skills") from Markdown files across multiple convention directories. Supports inline (body folded into parent turn) and subagent (isolated child loop) execution modes.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Skill` struct | skill.go L53 | Name, description, body, scope, allowed tools, runAs, model, effort |
| `Store` struct | skill.go L143 | Skill resolution across configured roots |
| `Scope` type | L16 | `project` > `custom` > `global` > `builtin` |
| `RunAs` type | L27 | `inline` | `subagent` |
| `New(opts Options)` | L148 | Build store from home dir, project root, custom paths |
| `List()` | L228 | Discover all skills, deduped, sorted by name |
| `Read(name)` | L248 | Resolve one skill by name |
| `Create(name, scope)` | L280 | Scaffold a new skill (refuses overwrite) |
| `Roots()` | L212 | Expose discovery directories with status |
| `ApplyIndex(base, skills)` | index.go L63 | Fold skills index into system prompt |
| `IndexBlock(skills)` | index.go L43 | Render the skills index (names + descriptions only) |
| `NewRunSkillTool(store, runner)` | tools.go L89 | Build the `run_skill` tool |
| `BuiltinSubagentTools(store, runner)` | tools.go L265 | Build `explore`/`research`/`review`/`security_review` tools |
| `SubagentRunner` type | tools.go L24 | `func(ctx, Skill, task, opts) -> (string, error)` |

### Precedence (highest first)
1. `./.reasonix/skills/` (project)
2. `./.agents/skills/` (project, convention)
3. `./.agent/skills/` (project, convention)
4. `./.claude/skills/` (project, convention, requires skill frontmatter)
5. Custom `[skills] paths` (config)
6. `~/.reasonix/skills/` (global)
7. `~/.agents/skills/` (global, convention)
8. `~/.agent/skills/` (global, convention)
9. `~/.claude/skills/` (global, convention)
10. Built-in skills (always last)

### Frontmatter Fields
| Field | Type | Purpose |
|---|---|---|
| `name:` | string | Overrides filename stem |
| `description:` | string | Required for index appearance |
| `runAs:` | inline/subagent | Execution mode |
| `context:` | fork | Legacy subagent trigger |
| `agent:` | string | Non-empty → subagent |
| `allowed-tools:` | comma-separated | Tool allowlist for subagents |
| `model:` | string | Model override for subagent |
| `effort:` | string | Effort override for subagent |

### Built-in Skills
Defined in [`builtins.go`](internal/skill/builtins.go):
- `init` — Bootstrap AGENTS.md (inline)
- `explore` — Codebase exploration (subagent)
- `research` — Web + code research (subagent)
- `review` — Code review on branch diff (subagent)
- `security-review` — Security-focused review (subagent)
- `test` — Run & fix test suite (inline)
- `install-capability` — Install MCP/skill from source (inline)

### Tests
7 test files:
- [`skill_test.go`](internal/skill/skill_test.go) — core discovery + reading
- [`skill_extra_test.go`](internal/skill/skill_extra_test.go) — edge cases
- [`tools_test.go`](internal/skill/tools_test.go) — skill tool execution
- [`builtins_test.go`](internal/skill/builtins_test.go) — built-in skills
- [`index_test.go`](internal/skill/index_test.go) — index rendering

---

## 9. Memory — Hierarchical Memory (REASONIX.md/AGENTS.md/CLAUDE.md)

**Package:** [`internal/memory/`](internal/memory/)  
**Key Files:** [`memory.go`](internal/memory/memory.go) (250 lines), [`store.go`](internal/memory/store.go) (600 lines)

Loads and composes hierarchical memory: project docs (REASONIX.md, AGENTS.md, CLAUDE.md) and auto-memory (per-fact Markdown notes with frontmatter).

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Set` struct | memory.go L19 | Docs + auto-memory store + index for one session |
| `Load(opts)` | memory.go L42 | Discover all memory for a session |
| `Compose(base, set)` | memory.go L100 | Fold memory into the system prompt |
| `Block()` | memory.go L86 | Render memory as a Markdown section |
| `DocPath(scope)` | memory.go L60 | Resolve writable doc path by scope |
| `WriteDoc(path, body)` | memory.go L82 | Overwrite a doc-memory file |
| `Store` struct | store.go L37 | Per-project auto-memory: directory of one-fact-per-file |
| `StoreFor(userDir, cwd)` | store.go L84 | Resolve auto-memory directory |
| `NewRecallTool(store)` | store.go | `memory` / `recall_memory` tool |
| `NewRememberTool(store)` | store.go | `remember` tool |
| `NewForgetTool(store)` | store.go | `forget` tool |

### Memory Doc Precedence (highest first)
1. `./REASONIX.md` (project, committed)
2. `./AGENTS.md` (fallback name)
3. `./CLAUDE.md` (Claude Code compat)
4. `~/.reasonix/REASONIX.md` (user-global)
5. Ancestor directories (walk up from project root)

### Memory Types
- `user` → `~/.reasonix/memory/global/` (shared across all projects)
- `feedback` → `~/.reasonix/memory/global/`
- `project` → `~/.reasonix/projects/<slug>/memory/`
- `reference` → `~/.reasonix/projects/<slug>/memory/`

### Tests
10+ test files:
- [`memory_test.go`](internal/memory/memory_test.go) — core memory
- [`store_test.go`](internal/memory/store_test.go) — auto-memory store
- [`remember_test.go`](internal/memory/remember_test.go) — remember tool
- [`forget_test.go`](internal/memory/forget_test.go) — forget tool
- [`recall_test.go`](internal/memory/recall_test.go) — recall tool

---

## 10. Memory Compiler — Memory v5 Session-Based Compilation

**Package:** [`internal/memorycompiler/`](internal/memorycompiler/)  
**Key File:** [`runtime.go`](internal/memorycompiler/runtime.go) (3467 lines)

The Memory v5 execution compiler runtime. Uses a planning/execution trace model: compiles IR from memory, executes tool traces, and mutates strategies based on outcomes. This is the most complex subsystem.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Runtime` struct | L56 | One project's Memory v5 state |
| `New(dir)` | L63 | Create/load runtime from directory |
| `PlannerIR` | L83 | Planner intermediate representation |
| `ExecutionTrace` | L148 | Records of one tool execution with outcome |
| `CompilerMutation` | L224 | Proposed mutation to a strategy |
| `MutationEvaluation` | L239 | Score impact of a mutation |
| `ControlPolicy` | L263 | Governs sub-agent behavior during execution |
| `Budget` | — | Resource budgets for compilation |
| `stateFile` / `tracesFile` | L44–47 | Persistence files under the runtime dir |

### Key Constants
- `version = "v5.9"` — current compiler version
- `explorationRatePercent = 10` — exploration vs. exploitation
- `mutationAcceptThreshold = 0.60` — minimum score to accept a mutation
- `mutationFeedbackCooldown = 30 * time.Minute`

### Tests
- [`runtime_test.go`](internal/memorycompiler/runtime_test.go) — core runtime
- [`compression_test.go`](internal/memorycompiler/compression_test.go) — IR compression
- [`hardening_regression_test.go`](internal/memorycompiler/hardening_regression_test.go) — regression hardening
- [`review_fixes_test.go`](internal/memorycompiler/review_fixes_test.go) — review fixes

---

## 11. Plugin — MCP Client (stdio/HTTP/SSE Transports)

**Package:** [`internal/plugin/`](internal/plugin/)  
**Key File:** [`plugin.go`](internal/plugin/plugin.go) (800+ lines)

Reasonix's MCP (Model Context Protocol) client. Connects to external MCP servers and adapts their tools to the `tool.Tool` interface. Supports stdio subprocess, Streamable HTTP, and legacy HTTP+SSE transports.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Spec` struct | L45 | Server declaration: name, transport type, command/args/url |
| `Host` struct | L99 | Owns running plugin connections, aggregates prompts/resources |
| `Client` struct | L262 | One MCP server connection |
| `StartAll(ctx, specs)` | L167 | Connect all plugins, abort on error |
| `StartAvailable(ctx, specs)` | L179 | Connect every plugin possible, record failures |
| `Add(ctx, spec)` | L340 | Hot-add one server to a running host |
| `Close()` | L200 | Terminate all plugin connections |
| `Prompts()` / `Resources()` | L130–143 | Discovered surfaces |
| `Servers()` | L283 | Status summary per connected server |
| `ToolInfo` struct | L271 | Human-facing tool metadata |
| `Failure` struct | L278 | Failed connection record |
| `transport` interface | L89 | Abstracts transport: `call`, `notify`, `close` |
| `protocolVersion` | L33 | `"2024-11-05"` (MCP spec version) |

### Transports
| Transport | File | Details |
|---|---|---|
| stdio | [`transport_stdio.go`](internal/plugin/transport_stdio.go) | Subprocess stdin/stdout JSON-RPC |
| HTTP/Streamable HTTP | [`transport_http.go`](internal/plugin/transport_http.go) | HTTP POST-based JSON-RPC |
| Legacy HTTP+SSE | — | Deprecated, kept for compatibility |

### MCP Tool Naming
- Namespaced `mcp__<server>__<tool>` to prevent collisions
- `SplitMCPName(name)` parses the triple
- `StripRawPrefix` support for servers with redundant prefixes

### Startup Tiers
- **Eager:** Block until handshake (for essential servers)
- **Background:** Registered as lazy/placeholder tools, real spawn deferred
- **Auto-demote:** Chronically slow eager plugins demoted to background

### Cache & Schema
- [`cache.go`](internal/plugin/cache.go) — Cached handshake schemas on disk
- [`lazy.go`](internal/plugin/lazy.go) — Lazy toolset (placeholder until handshake completes)
- [`canonicalize.go`](internal/plugin/canonicalize.go) — Canonicalizes MCP tool specs

### Tests
14 test files:
- [`plugin_test.go`](internal/plugin/plugin_test.go) — core plugin connection
- [`transport_http_test.go`](internal/plugin/transport_http_test.go) — HTTP transport
- [`transport_stdio_test.go`](internal/plugin/transport_stdio_test.go) — stdio transport
- [`cache_test.go`](internal/plugin/cache_test.go) — schema caching
- [`lazy_test.go`](internal/plugin/lazy_test.go) — lazy toolset
- [`canonicalize_test.go`](internal/plugin/canonicalize_test.go) — spec canonicalization
- [`hotadd_test.go`](internal/plugin/hotadd_test.go) — hot-add/remove
- [`stats_test.go`](internal/plugin/stats_test.go) — startup latency telemetry

---

## 12. Security — ContentSanitizer, Honeytokens, AnomalyDetector, AuditLog, Output Validation

**Package:** [`internal/security/`](internal/security/)  
**Files:** 10 `.go` files

Prompt injection defenses for tool outputs and agent inputs. Mirrors the DR1 TypeScript security subsystem.

### Subsystems

| Subsystem | File | Lines | Purpose |
|---|---|---|---|
| `ContentSanitizer` | [`sanitize.go`](internal/security/sanitize.go) | 250 | 5-step sanitization pipeline |
| `HoneytokenManager` | [`honeytoken.go`](internal/security/honeytoken.go) | 100 | Decoy credential detection |
| `AnomalyDetector` | [`anomaly.go`](internal/security/anomaly.go) | 350 | 6-algorithm behavioral detection |
| `AuditLog` | [`auditlog.go`](internal/security/auditlog.go) | 150 | Centralized event log with secret redaction |
| `OutputValidation` | [`outputvalidate.go`](internal/security/outputvalidate.go) | 120 | LLM output scanning |
| `DataNonce` | [`nonce.go`](internal/security/nonce.go) | 15 | `crypto/rand` per-turn boundary tokens |
| `Types` | [`types.go`](internal/security/types.go) | 60 | Shared types (WarningType, SanitizeResult, etc.) |

### ContentSanitizer — 5-Step Pipeline
1. **Unicode normalization** — NFKC (configurable: NFC/NFD/none)
2. **Invisible character stripping** — 17 zero-width/bidi control chars
3. **Shannon perplexity** — entropy-based anomaly detection (threshold: 100)
4. **Base64 detection** — >85% base64 alphabet on strings ≥40 chars
5. **Pattern detection** — 17 main + 3 warn regex patterns

### HoneytokenManager
- 3 detection patterns: `REASONIX_HONEYTOKEN_`, `sk-honey-`, `reasonix-honeytoken`
- 4 planted paths: `~/.ssh/reasonix_honeytoken_key`, `~/.aws/credentials`, `.env`, `.git/config`
- 3 planted env vars with known decoy values

### AnomalyDetector — 6 Algorithms
1. **writeAfterFetch:** fetch → write within 3 calls
2. **toolBurst:** >15 calls in one turn
3. **unusualSequence:** EMA baseline deviation >2σ
4. **privilegeEscalation:** read sensitive path → write
5. **dataExfiltration:** command/shell containing sensitive path patterns
6. **promptExtraction:** tool parameters containing system prompt fragments

### AuditLog
- 10 event types: `tool_execute`, `tool_blocked`, `tool_denied`, `injection_detected`, `honeytool_triggered`, `honeytoken_detected`, `anomaly_detected`, `session_start`, `session_end`
- 6 secret redaction patterns: github_token, aws_key, api_key, jwt, ssh_key, email
- In-memory ring buffer with optional disk flush

### OutputValidation
- URL detection (configurable allowlist)
- Raw HTML/script tag detection
- Base64/hex encoded blob detection

### Tests
7 test files:
- [`sanitize_test.go`](internal/security/sanitize_test.go) — sanitizer pipeline
- [`honeytoken_test.go`](internal/security/honeytoken_test.go) — honeytoken detection
- [`anomaly_test.go`](internal/security/anomaly_test.go) — anomaly detector
- [`auditlog_test.go`](internal/security/auditlog_test.go) — audit log
- [`outputvalidate_test.go`](internal/security/outputvalidate_test.go) — output validation
- [`nonce_test.go`](internal/security/nonce_test.go) — data nonce

---

## 13. CLI / TUI — Bubble Tea Chat Terminal, Slash Commands, Sub-Commands

**Package:** [`internal/cli/`](internal/cli/)  
**Key Files:** [`cli.go`](internal/cli/cli.go) (1922 lines), [`chat_tui.go`](internal/cli/chat_tui.go) (large)

The command-line entry point and interactive Bubble Tea TUI. Uses the Elm-architecture model from `charm.land/bubbletea/v2`.

### CLI Sub-Commands

| Command | File | Purpose |
|---|---|---|
| `reasonix` (bare) | `cli.go` Run() | Interactive chat REPL |
| `reasonix run <task>` | cli.go | Non-interactive agent execution |
| `reasonix serve` | cli.go + `internal/serve/` | HTTP/SSE server |
| `reasonix setup` | cli.go | Init wizard |
| `reasonix config` | cli.go | Config commands |
| `reasonix bot` | cli.go | IM bot gateway |
| `reasonix acp` | cli.go | ACP (Agent Communication Protocol) |
| `reasonix mcp` | cli.go | MCP management commands |
| `reasonix doctor` | cli.go | Diagnostics |
| `reasonix review` | cli.go | Code review |
| `reasonix upgrade` | cli.go | Self-update |

### Chat TUI (Bubble Tea)
The TUI is a complex Bubble Tea model with:
- **Status line:** Model name, cache hit rate, context window, turn/token counters, plan mode indicator
- **Slash commands:** `/model`, `/compact`, `/clear`, `/new`, `/rewind`, `/mcp`, `/skills`, `/theme`, etc.
- **Approval cards:** Tool call approval with diff previews
- **Ask cards:** Structured multiple-choice questions
- **Skill picker:** Interactive skills overlay
- **Session picker:** Resume session selection
- **Diff viewer:** `/diff` inline diff
- **Markdown rendering:** Styled terminal output

### Key TUI Files

| File | Purpose |
|---|---|
| [`chat_tui.go`](internal/cli/chat_tui.go) | Main TUI model + update + view |
| [`mcp.go`](internal/cli/mcp.go) | MCP management overlay |
| [`mcp_manager.go`](internal/cli/mcp_manager.go) | MCP connection manager view |
| [`diffview.go`](internal/cli/diffview.go) | Inline diff viewer |
| [`skill_picker.go`](internal/cli/skill_picker.go) | Skills interactive picker |
| [`chooser.go`](internal/cli/chooser.go) | Generic list chooser |
| [`box.go`](internal/cli/box.go) | Box-drawing utilities |
| [`md.go`](internal/cli/md.go) | Markdown rendering |
| [`latex.go`](internal/cli/latex.go) | LaTeX rendering |
| [`complete.go`](internal/cli/complete.go) | Tab completion |
| [`autoplan.go`](internal/cli/autoplan.go) | Auto-plan mode toggle |

### Tests
40+ test files in the package — one of the most heavily tested.

---

## 14. Event — Typed Event Stream & Sink Interface

**Package:** [`internal/event/`](internal/event/)  
**Key Files:** [`event.go`](internal/event/event.go) (200 lines), [`sync.go`](internal/event/sync.go) (20 lines)

Defines the typed event stream the agent emits as it runs a turn, and the `Sink` interface that decouples "what happened" from "how to show it."

### Event Kinds (20 total)

| Kind | Payload | Purpose |
|---|---|---|
| `TurnStarted` | — | Turn boundary |
| `Reasoning` | Text | Thinking-mode reasoning delta |
| `Text` | Text | Answer text delta |
| `Message` | Text + Reasoning | Full assistant message |
| `ToolDispatch` | Tool | Tool call about to run |
| `ToolResult` | Tool | Finished tool call |
| `Usage` | Usage + Pricing + CacheDiagnostics | Token telemetry |
| `Notice` | Level + Text | Out-of-band message |
| `Phase` | Text | Coordinator boundary |
| `ApprovalRequest` | Approval | Pending tool approval |
| `AskRequest` | Ask | Structured questions |
| `TurnDone` | Err | Turn completion |
| `CompactionStarted` | Compaction | Compaction begins |
| `CompactionDone` | Compaction | Compaction ends |
| `ToolProgress` | Tool (output chunk) | Live tool progress |
| `MCPSurfaceReady` | Text | MCP prompts/resources ready |
| `Retrying` | RetryAttempt/Max | Provider reconnection |
| `Steer` | Text | Mid-turn steer message consumed |
| `MemoryCompilerStatsEvent` | MemoryCompilerStats | Memory v5 metrics |
| `KindCount` | — | Sentinel |

### Key Types

| Type | Purpose |
|---|---|
| `Sink` interface | `Emit(Event)` — serial, non-blocking |
| `FuncSink` | Adapter: function → Sink |
| `Discard` | No-op sink for tests |
| `ReadinessAuditSink` | Optional sink capability |
| `Sync(sink)` | Thread-safe wrapper (concurrent-safe Emit) |

### Wire Format
**Package:** [`internal/eventwire/`](internal/eventwire/) — `ToWire()` converts runtime events to JSON for HTTP/SSE frontends.

---

## 15. Sandbox — macOS Seatbelt Confinement, Bash Sandbox

**Package:** [`internal/sandbox/`](internal/sandbox/)  
**Key Files:** [`sandbox.go`](internal/sandbox/sandbox.go) (50 lines), [`seatbelt_darwin.go`](internal/sandbox/seatbelt_darwin.go) (100+ lines)

Wraps shell commands in an OS-level jail so `bash` calls are confined. Only macOS (Seatbelt via `sandbox-exec`) is implemented; other OSes run unconfined.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Spec` struct | sandbox.go L17 | Mode (enforce/off), write roots, network, shell |
| `Available()` | sandbox.go | Platform check: is sandboxing supported? |
| `Command(spec, cmd, args)` | — | Wraps a command in sandbox-exec on macOS |
| `ResolveShell(prefer, path)` | — | Resolve shell preference (bash/powershell) |
| `Shell` struct | — | Shell kind (bash/powershell) + path |

### macOS Seatbelt
- Uses `sandbox-exec` with a custom `.sb` profile
- Write-allowed: workspace root, configured extras, temp dirs, toolchain caches
- Network: controlled by `Spec.Network` flag
- Read-allowed: everything (reads are unrestricted)

### Tests
- [`sandbox_test.go`](internal/sandbox/sandbox_test.go) — core sandbox tests
- [`seatbelt_darwin_test.go`](internal/sandbox/seatbelt_darwin_test.go) — macOS-specific
- [`shell_test.go`](internal/sandbox/shell_test.go) — shell resolution

---

## 16. Hook — 10 Hook Events (PreToolUse, PostToolUse, PostLLMCall, etc.)

**Package:** [`internal/hook/`](internal/hook/)  
**Key File:** [`hook.go`](internal/hook/hook.go) (400 lines), [`runner.go`](internal/hook/runner.go) (150 lines)

Runs user-configured shell-command hooks around the agent loop. Hooks come from `.reasonix/settings.json` (project, only when trusted) and `~/.reasonix/settings.json` (global).

### Hook Events

| Event | Blocking | Purpose |
|---|---|---|
| `PreToolUse` | Yes | Before each tool call |
| `PostToolUse` | No | After each tool call |
| `PermissionRequest` | No | Before approval prompt |
| `UserPromptSubmit` | Yes | Before a turn |
| `Stop` | No | After a turn |
| `PostLLMCall` | No | After model stream ends |
| `SessionStart` | No | Session begins |
| `SessionEnd` | No | Session ends |
| `SubagentStop` | No | Sub-agent finishes |
| `Notification` | No | User attention needed |
| `PreCompact` | No | Before compaction |

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Event` type | L27 | Hook event identifier |
| `HookConfig` struct | L101 | Match regex, command, description, timeout, cwd |
| `Settings` struct | L114 | JSON shape of settings.json |
| `ResolvedHook` struct | L119 | Loaded hook with scope and source |
| `Payload` struct | L167 | JSON envelope written to hook's stdin |
| `Decision` type | L177 | pass/block/warn/error |
| `Outcome` struct | L183 | One hook invocation's result |
| `Report` struct | L193 | Aggregate of event's hook outcomes |
| `Load(opts)` | L137 | Resolve hooks from project + global |
| `Run(ctx, payload, hooks, spawner)` | L212 | Execute matching hooks |
| `DefaultSpawner` | L233 | Real spawner via platform shell |

### Exit Code Semantics
- `0` = pass
- `2` = block (only on gating events)
- other = warn

### Tests
- [`hook_test.go`](internal/hook/hook_test.go) — core hook logic
- [`runner_test.go`](internal/hook/runner_test.go) — hook runner
- [`trust_test.go`](internal/hook/trust_test.go) — project trust

---

## 17. Permission — Permission Policy Engine, Bash Read-Only Classification

**Package:** [`internal/permission/`](internal/permission/)  
**Key Files:** [`permission.go`](internal/permission/permission.go) (350 lines), [`bash_readonly.go`](internal/permission/bash_readonly.go) (150 lines)

Decides per tool call whether to allow, ask, or deny. Core is a pure `Policy` (no I/O); `Gate` wraps it with an optional interactive `Approver`.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Decision` type | permission.go L17 | Allow / Ask / Deny |
| `Rule` struct | L53 | Tool + Subject (glob or literal) |
| `Policy` struct | L89 | Mode + Allow/Ask/Deny rule lists |
| `New(mode, allow, ask, deny)` | L97 | Build Policy from config strings |
| `Decide(toolName, readOnly, args)` | L114 | Evaluate a tool call |
| `Gate` struct | L210 | Policy + Approver |
| `NewGate(policy, approver)` | L219 | Wire policy to optional approver |
| `Approver` interface | L198 | Interactive approval callback |
| `Subject(args)` | L170 | Extract matchable subject from JSON args |
| `Subjects(args)` | L182 | Extract all subjects (for two-path ops) |
| `BashDangerWarning(subject)` | bash_readonly.go L153 | Danger label for destructive commands |
| `isReadOnlyBashSubject(subject)` | bash_readonly.go L87 | Classify bash command as read-only |
| `IsFileMutationTool(name)` | permission.go L262 | Check if tool edits files |
| `BashCommandPrefix(subject)` | permission.go L234 | Safe prefix for "similar command" grants |

### Bash Read-Only Classification
- 30+ single-word read-only commands (`cat`, `ls`, `grep`, etc.)
- 6 prefix commands with read-only subcommands (`git log`, `go vet`, `npm list`, `cargo check`, `docker ps`, `kubectl get`)
- Shell syntax detection (`;`, `|`, `&`, `$()`, `` ` ``)
- Unsafe argument detection for `find`, `sed`, `sort`, `git diff`, `git show`, `git log`, `go env`

### Tests
5 test files:
- [`permission_test.go`](internal/permission/permission_test.go) — core policy
- [`bash_readonly_test.go`](internal/permission/bash_readonly_test.go) — bash classification
- [`permission_extra_test.go`](internal/permission/permission_extra_test.go) — edge cases

---

## 18. Plan Mode — Plan Mode Policy, Bash Safety Gating, Reconciliation Tests

**Package:** [`internal/planmode/`](internal/planmode/)  
**Key Files:** [`policy.go`](internal/planmode/policy.go) (350 lines), [`reconcile_test.go`](internal/planmode/reconcile_test.go) (100 lines)

Enforces the read-only planning phase. Every tool call is gated through `Policy.Decide()` before it reaches the permission gate.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Policy` struct | L115 | AllowedTools override list |
| `Call` struct | L96 | Tool invocation view: name, readOnly, untrusted, safety, args |
| `Decision` struct | L110 | Blocked + message |
| `Decide(call)` | L151 | Plan mode gate: fail-closed |
| `Classify(name, readOnly, safety)` | L194 | Bucket a tool into a plan-mode class |
| `Class` type | L185 | BashGated / BlockedKnown / AlwaysAllowed / etc. |
| `Marker` const | L1 | Model-facing plan-mode instruction block |

### Plan Mode Decision Flow
1. `bash` → per-argument safety check (`decideBash`)
2. `knownBlockedTools` (25 tools) → blocked
3. Self-reported `PlanSafetyUnsafe` → blocked
4. `alwaysAllowedTools` (`ask`, `todo_write`) → allowed
5. Self-reported `PlanSafetySafe` → allowed (enforces ReadOnly)
6. Trusted `ReadOnly()` built-in → allowed
7. `plan_mode_allowed_tools` override check
8. Untrusted read-only (MCP) → blocked with explanation
9. Anything else → blocked as writer

### Bash Plan-Mode Safety
- Blocks shell operators (`&&`, `||`, `$()`, `` ` ``, `;`, `|`)
- Only allows a curated list of 16 safe command prefixes
- Detects unsafe arguments: `find -delete`, `git --output`, `go -mod=mod`
- Blocks background execution and process preservation

### Tests
- [`policy_test.go`](internal/planmode/policy_test.go) — decision logic
- [`reconcile_test.go`](internal/planmode/reconcile_test.go) — every built-in must be explicitly classified
- [`marker_test.go`](internal/planmode/marker_test.go) — plan mode marker parsing

---

## 19. Checkpoint — File Checkpoint/Rewind Store

**Package:** [`internal/checkpoint/`](internal/checkpoint/)  
**Key File:** [`checkpoint.go`](internal/checkpoint/checkpoint.go) (250 lines)

Snapshot-based edit safety net. Before a writer tool changes a file, the agent records the pre-edit state. Git-free: snapshots live beside the session.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `FileSnap` struct | L22 | One file's pre-edit state (content or nil for creates) |
| `Checkpoint` struct | L33 | One turn's snapshots + prompt + msg index |
| `Meta` struct | L43 | Picker-facing summary (no content) |
| `Store` struct | L49 | In-memory + optional disk persistence |
| `New(dir, root)` | L60 | Build store from checkpoint dir + workspace root |
| `Begin(turn, prompt, msgIndex)` | L76 | Open checkpoint for a new turn |
| `Snapshot(change)` | L91 | Record pre-edit state of a file |
| `RestoreCode(fromTurn)` | L127 | Rewind workspace to earlier turn's state |
| `List()` | L116 | List checkpoint metadata |
| `Bounds()` | L84 | Build turn→msgIndex map for conversation rewind |

### Tests
- [`checkpoint_test.go`](internal/checkpoint/checkpoint_test.go) — core checkpoint operations

---

## 20. i18n — EN, zh-CN, zh-TW Messages

**Package:** [`internal/i18n/`](internal/i18n/)  
**Key Files:** [`i18n.go`](internal/i18n/i18n.go) (150 lines), [`messages_en.go`](internal/i18n/messages_en.go), [`messages_zh.go`](internal/i18n/messages_zh.go), [`messages_zh_tw.go`](internal/i18n/messages_zh_tw.go)

Translatable CLI strings. Architecture: a single `Messages` struct of exported string fields. Each language declares its own `Messages` value.

### Key Types & Functions

| Type/Function | Line | Purpose |
|---|---|---|
| `Messages` struct | i18n.go L22 | 210+ translatable string fields |
| `M` | L283 | Active catalogue (package global) |
| `DetectLanguage(override)` | L389 | Auto-detect from env, replace M |
| `English` | messages_en.go | Default English catalogue |
| `Chinese` | messages_zh.go | Simplified Chinese |
| `ChineseTraditional` | messages_zh_tw.go | Traditional Chinese |

### Detection Priority
1. Override (e.g. `cfg.Language`)
2. `REASONIX_LANG`
3. `LC_ALL`
4. `LC_MESSAGES`
5. `LANG`
6. `"en"` (fallback)

### Test
- [`i18n_test.go`](internal/i18n/i18n_test.go) — test catalog completeness via reflection

---

## 21. LSP — Language Server Protocol Tools

**Package:** [`internal/lsp/`](internal/lsp/)  
**Key Files:** [`manager.go`](internal/lsp/manager.go) (200+ lines), [`tool.go`](internal/lsp/tool.go) (100 lines), [`client.go`](internal/lsp/client.go) (150 lines)

Optional LSP-based code intelligence tools. Servers are never bundled — each resolves on PATH.

### LSP Tools

| Tool | Description |
|---|---|
| `lsp_definition` | Jump to symbol definition |
| `lsp_references` | List all references to a symbol |
| `lsp_hover` | Show type signature and documentation |
| `lsp_diagnostics` | Report compiler/linter diagnostics for a file |

### Key Types & Functions

| Type/Function | File | Purpose |
|---|---|---|
| `Manager` struct | manager.go | Session-scoped LSP server manager |
| `NewManager(root, specs)` | manager.go | Spawns LSP servers per language |
| `Client` struct | client.go | JSON-RPC client for one LSP server |
| `Tools(m)` | tool.go | Adapts Manager to Tool interface |
| `DefaultSpecs()` | — | Built-in language→server map |

### Tests
- [`lsp_test.go`](internal/lsp/lsp_test.go) — core LSP tests
- [`jsonrpc_test.go`](internal/lsp/jsonrpc_test.go) — JSON-RPC framing
- [`manual_test.go`](internal/lsp/manual_test.go) — integration tests

---

## 22. CLI Sub-Commands — `reasonix`, `serve`, `bot`, `acp`, etc.

### HTTP/Serve Frontend

**Package:** [`internal/serve/`](internal/serve/)  
**Key File:** [`serve.go`](internal/serve/serve.go) (200+ lines)

HTTP/SSE server frontend for the Controller. Supports multiple auth modes and CSRF protection.

### IM Bot Gateways

**Package:** [`internal/bot/`](internal/bot/)  
**Files:** Multiple per-channel adapters

Multi-channel IM bot gateways:
- **Feishu/Lark:** [`feishu/feishu.go`](internal/bot/feishu/feishu.go) — webhook + websocket modes
- **WeChat iLink:** [`weixin/weixin.go`](internal/bot/weixin/weixin.go)
- **QQ:** [`qq/gateway.go`](internal/bot/qq/gateway.go) — QQ Bot API v2

### ACP — Agent Communication Protocol

**Package:** [`internal/acp/`](internal/acp/)  
**Key File:** [`server.go`](internal/acp/server.go), [`protocol.go`](internal/acp/protocol.go)

A standardized protocol for agent-to-agent communication. Supports dispatching tasks, partial results, and session management.

### Bot Runtime

**Package:** [`internal/botruntime/`](internal/botruntime/)  
Manages the lifecycle of bot sessions: session creation, turn management, rate limiting, and cleanup.

### Command / Slash Commands

**Package:** [`internal/command/`](internal/command/)  
Custom slash command system. Users define commands as Markdown files with frontmatter in convention directories. The `slash_command` tool lets the model invoke them.

### Event Wire

**Package:** [`internal/eventwire/`](internal/eventwire/)  
JSON contract between Go runtime and frontends (web UI, desktop). `ToWire()` converts typed events to JSON.

### Evidence

**Package:** [`internal/evidence/`](internal/evidence/)  
Readiness audit and command-match verification for plan step completion.

### Jobs

**Package:** [`internal/jobs/`](internal/jobs/)  
Background job manager: create, monitor, cancel long-running tasks. Used by bash background execution and sub-agents.

---

## 23. Desktop — Wails Desktop Application

**Package:** [`desktop/`](desktop/)  
**Key File:** [`app.go`](desktop/app.go) (large, 100+ test files)

Wails-based desktop application with a React/Vite frontend. Shares the same `control.Controller` assembly via `boot.Build()`.

### Key Features
- Multi-project tabs (each tab has its own Controller with its own workspace root)
- Settings panel (model selection, MCP management, theme, hooks)
- Skills sidebar
- Session management (list, resume, delete)
- Cross-platform: macOS, Linux, Windows
- Auto-update (via `internal/update/`)
- Sound notifications
- Crash recovery

### Frontend
Located in `desktop/frontend/`:
- React + TypeScript + Vite
- Tailwind CSS
- Component library with skills panel, settings, chat transcript, etc.

---

## 24. mcp-web-search — Standalone MCP Web Search Server

**Directory:** [`mcp-web-search/`](mcp-web-search/)  
**Key File:** [`main.go`](mcp-web-search/main.go) (200 lines)

A zero-dependency standalone MCP server for web search. Supports multiple search engines:
- **Exa** (primary, API key from `EXA_API_KEY`)
- **Tavily** (fallback, API key from `TAVILY_API_KEY`)
- **DuckDuckGo** (fallback when no API keys configured)

### Key Files

| File | Purpose |
|---|---|
| [`main.go`](mcp-web-search/main.go) | MCP server with JSON-RPC 2.0 |
| [`main_test.go`](mcp-web-search/main_test.go) | Tests |
| [`go.mod`](mcp-web-search/go.mod) | Independent module (no vendor) |
| [`README.md`](mcp-web-search/README.md) | Usage docs |

---

## Tool Summary

| Category | Count | Details |
|---|---|---|
| **Total unique tool surfaces** | 43 | Via Registry + named wrappers |
| **Compile-time built-ins** | 19 | `init()` self-registered |
| **Agent meta-tools** | 4 | task, read_only_task, parallel_tasks, ask |
| **LSP tools** | 4 | definition, references, hover, diagnostics |
| **Skill tools** | 9 | run_skill, read_only_skill, read_skill, install_skill, explore, research, review, security_review, install_source |
| **Memory tools** | 3 | remember, forget, memory |
| **History tools** | 2 | list_sessions, read_session |
| **Other** | 2 | connect_tool_source, slash_command |

---

## Dependency Security

**Vendor directory:** [`vendor/`](vendor/)  
**Policy file:** [`.agents/dependency-security.md`](.agents/dependency-security.md)

- All dependencies vendored — no network fetch at build time
- Every dependency change requires `security-reviewer` skill audit
- Last full audit: 2026-06-25 — 38 packages, 0 critical, 1 high, 8 medium
- Audit reports in `docs/security/`

---

## Key Differences vs DR1 TypeScript Fork

| Dimension | DR1 (TypeScript) | Go Reasonix |
|---|---|---|
| Binary size | ~200 MB (Node.js) | ~20 MB (static binary) |
| TUI framework | Ink (React) | Bubble Tea (Elm architecture) |
| Config format | JSON | TOML |
| Language idioms | TypeScript classes | Go structs + interfaces |
| Tool registration | Dynamic import | Compile-time `init()` + blank imports |
| Lint/format | biome + vitest | gofmt + go vet |
| Testing | vitest | `go test` (standard) |
| Concurrency | Event loop (single-thread) | Goroutines + sync primitives |
| MCP naming | Plugin namespace | `mcp__<server>__<tool>` |
| Plan mode | `submit_plan` tool | Turn's final response → `exit_plan_mode` |
