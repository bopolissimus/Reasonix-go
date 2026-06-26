# Gap Analysis: TypeScript Reasonix vs Go Reasonix

> **Date:** 2026-07-25
> **Source:** DR1 TypeScript fork (reference) vs Go Reasonix at `/home/dev/reasonix/go1`

This document tracks feature parity between the Go Reasonix project and the
original DR1 TypeScript fork. Each feature is classified as **Exists in Go**
(full implementation), **Partial** (exists but differs in scope/approach), or
**Missing** (no Go equivalent). See the [tools comparison table](tools-comparison.md)
for per-tool differences.

## Legend

- ✅ **Exists** — Full feature parity, equivalent behavior.
- 🔶 **Partial** — Exists but differs in scope, implementation approach, or
  surface area.
- ❌ **Missing** — Not yet implemented; planned ticket or deferred.
- ⚪ **N/A** — Not applicable to the Go design (e.g. Node.js-specific).

---

## 1. Security Subsystem

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| ContentSanitizer pipeline (5-step) | ✅ | ✅ | Go is line-for-line equivalent. 17+3 regex patterns (TS had 20). |
| Honeytoken detection | ✅ | ✅ | 3 detection patterns, 4 planted paths, 3 env vars. |
| AnomalyDetector (6 algorithms) | ✅ | ✅ | EMA baseline, sliding window, 6 algorithms. |
| AuditLog (10 event types) | ✅ | ✅ | Go has in-memory ring buffer; TS had optional disk flush. |
| DataNonce (per-turn boundary tokens) | ✅ | ❌ | Go added REX-88 (Dynamic nonce delimiters) which TS never had. |
| OutputValidation (URL/HTML/blob) | ✅ | ❌ | Go added REX-89 (output validation / content allowlist). |
| ToolPolicy / per-tool allowlist | 🔶 | ✅ | Go has `plan_mode_allowed_tools` config; DR1 had richer dynamic policy. |
| ContextSegmenter | ❌ | ✅ | Deferred to eval harness only (not a security defense per DR1 research). |
| NUCLEAR-YOLO git block | ❌ | ✅ | REX-64; queued for Phase A. |
| MCP influence guard (read/write tier) | ❌ | ✅ | REX-66; Phase B. |
| MCP data fence awareness | ❌ | ✅ | REX-67; Phase B. |
| MCP sandbox subagent | ❌ | ✅ | REX-68; Phase B. |
| Tool shadowing prevention | ❌ | ✅ | REX-69; Phase B. |

## 2. Tool System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| `ask` (structured questions) | ✅ | ✅ | Go combines TS's `ask_user` + `ask_choice` into one multi-question batch tool. |
| `bash` / `run_command` | ✅ | ✅ | Go adds deep safety: quoting-aware parsing, argument-level gating, sandbox confinement. |
| Background jobs | ✅ | ✅ | Go uses 4 tools (`bash`, `bash_output`, `kill_shell`, `wait`); TS used 5. |
| `code_index` / `get_symbols` | ✅ | ✅ | Go uses Go AST only; TS uses tree-sitter (multi-language). |
| `complete_step` / `mark_step_complete` | ✅ | ✅ | Go adds evidence-backed completion with verification struct. |
| `connect_tool_source` | ✅ | ❌ | **Go-only** — token-economy connector for optional tool sources. |
| `confine` | ✅ | ❌ | **Go-only** — workspace/sandbox confinement toggle. |
| `delete_range` | ✅ | ❌ | **Go-only** — delete text by anchor text range. |
| `delete_symbol` | ✅ | ❌ | **Go-only** — delete Go symbol via AST parsing. |
| `edit_file` | ✅ | ✅ | Same — SEARCH/REPLACE with exact text anchoring. |
| `glob` | ✅ | ✅ | Same — file pattern matching. |
| `grep` / `search_content` | ✅ | ✅ | Same — regex content search; different name. |
| `install_skill` / `create_skill` | 🔶 | ✅ | Go's `install_skill` installs but does not scaffold. TS's `create_skill` scaffolds new skills. |
| `install_source` | ✅ | ✅ | Both install from URL/source. |
| `ls` / `list_directory` | ✅ | ✅ | Same; different name. |
| `list_sessions` | ✅ | ❌ | **Go-only** — list saved conversation sessions. |
| `lsp_definition` | ✅ | ❌ | **Go-only** — LSP go-to-definition (gopls, rust-analyzer, etc.). |
| `lsp_diagnostics` | ✅ | ❌ | **Go-only** — LSP compiler/linter diagnostics. |
| `lsp_hover` | ✅ | ❌ | **Go-only** — LSP type/doc on hover. |
| `lsp_references` | ✅ | ❌ | **Go-only** — LSP find-all-references. |
| `move_file` | ✅ | ✅ | Same — rename/move. |
| `multi_edit` | ✅ | ✅ | Same — atomic multi-edit across files. |
| `notebook_edit` | ✅ | ❌ | **Go-only** — edit Jupyter notebook cells. |
| `parallel_tasks` | ✅ | ❌ | **Go-only** — concurrent sub-agent spawning. |
| `read_file` | ✅ | ✅ | Same — read file with line offset. |
| `read_session` | ✅ | ❌ | **Go-only** — read saved session by filename. |
| `read_skill` | ✅ | ❌ | **Go-only** — load skill body without executing (plan-mode-safe). |
| `read_only_skill` | ✅ | ❌ | **Go-only** — run skill in read-only mode. |
| `read_only_task` | ✅ | ❌ | **Go-only** — read-only sub-agent. |
| `remember` / `recall` / `forget` | ✅ | ✅ | Same — memory management tools. |
| `run_skill` | ✅ | ✅ | Same — invoke a playbook. |
| `slash_command` | ✅ | ❌ | **Go-only** — invoke project slash commands as a tool. |
| `todo_write` | ✅ | ✅ | Same; Go adds `level` field for hierarchical plans. |
| `web_fetch` | ✅ | ✅ | Same — fetch URL. |
| `web_search` | ❌ | ✅ | REX-70; Phase C. Go has `mcp-web-search/` standalone server but no built-in tool. |
| `workspace` | ✅ | ❌ | **Go-only** — return workspace root path. |
| `write_file` | ✅ | ✅ | Same — write/overwrite file. |
| `copy_file` | ❌ | ✅ | Go uses `bash cp` instead. |
| `create_directory` | ❌ | ✅ | Go uses `bash mkdir -p`. |
| `delete_directory` | ❌ | ✅ | Go uses `bash rm -rf`. |
| `delete_file` | ❌ | ✅ | Go uses `bash rm`. |
| `directory_tree` | ❌ | ✅ | Go uses `ls` + `glob` composition. |
| `get_file_info` | ❌ | ✅ | Go gets partial info via `ls`. |
| `isolate_workspace` | ❌ | ✅ | REX-71; Phase C. |
| `merge_worktree` | ❌ | ✅ | REX-71; Phase C. |
| `search_files` | ❌ | ✅ | Go uses `glob` + pipe; no dedicated filename-only search. |
| `submit_plan` | ❌ | ✅ | Go uses turn's final response + `exit_plan_mode`. |
| `revise_plan` | ❌ | ✅ | Go stays in plan mode, re-presents via `todo_write`. |
| `add_mcp_server` (model tool) | ❌ | ✅ | Go exposes MCP registration only as CLI command (`reasonix mcp add`). |

**Summary: 25 shared, 17 Go-only, 11 TS-only.**

## 3. Sub-Agent System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| `task` tool (sub-agent delegation) | ✅ | ✅ | Same fundamental pattern. |
| `read_only_task` | ✅ | ❌ | **Go-only** — read-only restricted sub-agent. |
| `parallel_tasks` | ✅ | ❌ | **Go-only** — concurrent sub-agent spawning. |
| Sub-agent transcript persistence | ✅ | ✅ | Go uses `sa_` transcripts on disk. |
| Sub-agent model override | ✅ | ✅ | Frontmatter `model:` / `effort:` fields. |
| Sub-agent tool boundary | ✅ | ✅ | Go explicitly removes meta tools and job tools. |
| Sub-agent identity on prompts | ❌ | ✅ | REX-72 (REX-31, REX-32); Phase D. |
| Sub-agent model fallback (v4-pro→flash) | ❌ | ✅ | REX-75; Phase D. |
| Redirect verdict | ❌ | ✅ | REX-73; Phase D. |
| Path_access gate queuing | ❌ | ✅ | REX-74; Phase D. |

## 4. Controller / Turn Lifecycle

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Transport-agnostic Controller | ✅ | ✅ | Same design; Go's is cleaner (single `control.Controller`). |
| Event stream (typed events) | ✅ | ✅ | Same pattern; Go adds `CompilerStats`, `Steer`, `MCPSurfaceReady`, `Retrying`. |
| Approval manager | ✅ | ✅ | Headless vs interactive gates; Go adds `ApprovalTimeout`. |
| Auto-plan classifier | ✅ | ✅ | Go version in `internal/control/auto_plan.go`. |
| Goal machine | ✅ | ✅ | Go version in `internal/control/goal.go`. |
| Turn orchestration | ✅ | ✅ | `turn_orchestrator.go` manages sub-agent event nesting. |
| Session persistence | ✅ | ✅ | JSONL format. |
| Checkpoint/rewind | ✅ | ✅ | Go adds encoding-aware file snaphots. |
| Context compaction | ✅ | ✅ | Go adds archive dir for traceability. |

## 5. Agent Loop

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Single-agent loop | ✅ | ✅ | Same. |
| Two-model coordinator (planner + executor) | ✅ | ✅ | Go's is in `internal/agent/coordinator.go`. |
| Plan mode | ✅ | ✅ | Go adds `read_only_task`, `read_only_skill`, bash safety gating. |
| NUCLEAR-YOLO mode | ✅ | ✅ | Same concept. |
| Retry/backoff | ✅ | ✅ | Go's in `internal/provider/retry.go`. |
| Outcome classification | ✅ | ✅ | Go's in `internal/agent/outcome_test.go`. |
| Cache diagnostics | ✅ | ✅ | `cache_shape.go` tracks prefix stability. |
| Reasoning language | ✅ | ✅ | `reasoning_language.go`. |

## 6. Provider System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| OpenAI-compatible provider | ✅ | ✅ | Both. |
| Anthropic provider | ✅ | ✅ | Both. |
| Stream completion | ✅ | ✅ | Both. |
| Tool-call normalization | ✅ | ✅ | Go adds truncated-JSON repair, backfill of empty names. |
| Auth error detection | ✅ | ✅ | `AuthError` type. |
| Retry logic | ✅ | ✅ | Go's `retry.go`. |
| Model fetch / listing | ✅ | ✅ | Both. |
| Token usage / pricing | ✅ | ✅ | Both. |
| Thinking mode | ✅ | ✅ | Go forwards `reasoning_content` and `reasoning_signature`. |
| Pairing probe | ❌ | ✅ | REX-73; Phase D. |

## 7. Configuration

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| TOML config | ✅ | ❌ | Go uses TOML; TS uses JSON. |
| Config resolution (flag > project > user > defaults) | ✅ | ✅ | Same precedence order. |
| API keys from env | ✅ | ✅ | Both. |
| Provider entries | ✅ | ✅ | Both. |
| Security config | ✅ | ✅ | Both. |
| Skills config (paths, exclusions) | ✅ | ✅ | Both. |
| Config migration from legacy | ✅ | ✅ | Go handles v1/v0.5 legacy config. |
| Backup rotation | ✅ | ❌ | **Go-only** — `internal/config/backup.go`. |

## 8. Skill System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Built-in skills (explore, research, review, security-review, test, init, install-capability) | ✅ | ✅ | Same set. |
| Skill discovery across convention dirs | ✅ | ✅ | Go adds `.reasonix`, `.agents`, `.agent`, `.claude`. |
| Frontmatter parsing | ✅ | ✅ | Go supports: name, description, allowed-tools, model, effort, runAs. |
| Skills index in system prompt | ✅ | ✅ | Same approach (names+descriptions only; bodies on demand). |
| `run_skill` tool | ✅ | ✅ | Same. |
| `read_skill` tool | ✅ | ❌ | **Go-only** — load inline skill without executing. |
| `read_only_skill` tool | ✅ | ❌ | **Go-only** — read-only skill invocation. |
| Dedicated subagent wrappers | ✅ | ✅ | `explore`/`research`/`review`/`security_review` as top-level tools. |
| `install_skill` tool | ✅ | ✅ | Both. |
| Skill file references (`references/` dir) | ✅ | ✅ | Both append sibling markdown files. |
| Skill scripts (`scripts/` dir listing) | ✅ | ❌ | **Go-only** — appends scripts directory listing to skill body. |
| Skill `create` scaffolding | 🔶 | ✅ | Go has `Create()` (programmatic) and `install_skill`; no `create_skill` model tool. |
| Skill `delete` / `disable` | ✅ | ✅ | Go's `/skill picker` supports toggling and deleting. |

## 9. Memory System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Hierarchical docs (REASONIX.md / AGENTS.md / CLAUDE.md) | ✅ | ✅ | Same precedence. |
| Auto-memory store | ✅ | ✅ | Same. |
| Memory v5 compiler | ✅ | ✅ | Full Memory v5 with runtime, state persistence, traces. |
| Remember/forget tools | ✅ | ✅ | Same. |
| Memory index in system prefix | ✅ | ✅ | Same. |
| `quick-add` memory | ✅ | ✅ | Both. |
| `write_doc` / `append_doc` | ✅ | ✅ | Both. |
| Memory citations in event stream | ✅ | ✅ | Both. |

## 10. Plugin / MCP System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| MCP stdio transport | ✅ | ✅ | Both. |
| MCP HTTP/Streamable HTTP transport | ✅ | ✅ | Both. |
| MCP SSE transport (legacy) | ✅ | ✅ | Both. |
| Tool namespace via `mcp__<server>__<tool>` | ✅ | ✅ | Same. |
| Lazy/background startup | ✅ | ✅ | Go adds `lazy_test.go` + `LazyToolset`. |
| Schema caching | ✅ | ✅ | Both. |
| Startup latency telemetry | ✅ | ❌ | **Go-only** — `RecordStartup` + auto-demotion. |
| Prompts and resources discovery | ✅ | ✅ | Both. |
| Cached schema for fast launch | ✅ | ❌ | **Go-only** — MCP schema cache on disk. |
| Low-priority stdio | ✅ | ❌ | **Go-only** — `LowPriority` Spec field. |
| `StripRawPrefix` for redundant names | ✅ | ❌ | **Go-only** — cleaner MCP tool names. |
| Hot-add via `Add` with spawn guards | ✅ | ✅ | Go adds concurrent spawn guard. |
| `ReadOnlyToolNames` override | ✅ | ❌ | **Go-only** — trust known read-only MCP tools. |
| MCP influence guard | ❌ | ✅ | REX-66; Phase B. |
| MCP data fence | ❌ | ✅ | REX-67; Phase B. |
| MCP sandbox subagent | ❌ | ✅ | REX-68; Phase B. |
| Tool shadowing prevention | ❌ | ✅ | REX-69; Phase B. |

## 11. CLI / TUI

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Bubble Tea TUI | ✅ | ❌ | Go uses Bubble Tea (Elm architecture); TS used Ink (React). |
| Slash commands | ✅ | ✅ | Go adds `/skill`, `/sandbox`, `/effort`, `/auto-plan`, `/reasoning-language`, `/memory-v5`. |
| Ctrl+C handling | ✅ | ✅ | Same. |
| Status line (model, cache, tokens, cost) | ✅ | ✅ | Same. |
| Diff viewer | ✅ | ✅ | Both. |
| Markdown rendering | ✅ | ✅ | Go uses `charmbracelet/glamour`-style rendering via `lipgloss`. |
| Image paste support | ✅ | ✅ | Both. |
| Theme support | ✅ | ✅ | Go: graphite, aurora, slate, carbon, nocturne, amber, ember, midnight, sandstone, porcelain, linen, glacier. |
| Ctrl+T thinking toggle | ❌ | ✅ | REX-76; Phase E. |
| Word-wrap input | ❌ | ✅ | REX-77; Phase E. |
| Persistent workspace bar | ❌ | ✅ | REX-78; Phase E. |
| Shortcuts modal | ❌ | ✅ | REX-79; Phase E. |
| Mouse events (SGR) | ❌ | ✅ | REX-80; Phase E. |
| Clipboard enhancements | ❌ | ✅ | REX-81; Phase E. |
| Session picker dialog | ❌ | ✅ | REX-82; Phase E. |
| `/defaults` slash command | ❌ | ✅ | REX-83; Phase E. |
| Chat/code mode separation | ❌ | ✅ | REX-84; Phase E. |
| Scroll freeze fix | ❌ | ✅ | REX-85; Phase E. |
| Cursor movement fix | ❌ | ✅ | REX-86; Phase E. |
| Input history DOWN fix | ❌ | ✅ | REX-87; Phase E. |

## 12. Bot / IM Gateway

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Feishu/Lark bot | ✅ | ✅ | Both. |
| WeChat iLink bot | ✅ | ✅ | Both. |
| QQ bot | ✅ | ✅ | Both. |
| Bot connection management | ✅ | ✅ | Both. |
| Bot allowlist | ✅ | ✅ | Both. |
| Desktop bot runtime | ✅ | ✅ | Both. |

## 13. Hook System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| 10 hook events | ✅ | ✅ | Same events: PreToolUse, PostToolUse, PermissionRequest, UserPromptSubmit, Stop, PostLLMCall, SessionStart, SessionEnd, SubagentStop, Notification, PreCompact. |
| Hook verdicts (pass/block/warn/error) | ✅ | ✅ | Same. |
| Project + global scopes | ✅ | ✅ | Same. |
| `Match` regex filtering | ✅ | ✅ | Same. |
| Timeout per hook | ✅ | ✅ | Same. |
| Output cap (256 KiB) | ✅ | ✅ | Same. |
| Trust model | ✅ | ✅ | Same. |

## 14. Permission / Policy

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Policy engine (allow/ask/deny) | ✅ | ✅ | Same. |
| Bash read-only classification | ✅ | ✅ | Go's is more detailed: `readOnlyBashCommands`, `readOnlyBashPrefixes`, dangerous patterns. |
| Plan mode policy | ✅ | ✅ | Go adds audited `planSafeReadOnly` whitelist, bash argument-level gating. |
| Session grants | ✅ | ✅ | Both. |
| Glob-based subject matching | ✅ | ✅ | Same. |
| Bash command prefix rules | ✅ | ✅ | Same. |
| `BashDangerWarning` | ✅ | ✅ | Same. |

## 15. Sandbox / Confinement

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| macOS Seatbelt sandbox | ✅ | ✅ | Both use `sandbox-exec`. |
| Write-root confinement | ✅ | ✅ | Same. |
| Network egress control | ✅ | ✅ | Same. |
| Shell path probe | ✅ | ✅ | Both. |
| Process hiding (console/visibility) | ✅ | ✅ | Both. |
| Process killing | ✅ | ✅ | Both. |
| Linux sandbox | ❌ | ✅ | Deferred; falls back to unconfined. |

## 16. LSP System

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| LSP definition | ✅ | ❌ | **Go-only**. |
| LSP references | ✅ | ❌ | **Go-only**. |
| LSP hover | ✅ | ❌ | **Go-only**. |
| LSP diagnostics | ✅ | ❌ | **Go-only**. |
| Configurable servers | ✅ | ❌ | **Go-only** — `[lsp]` TOML config with language → server map. |
| JSON-RPC framing | ✅ | ❌ | **Go-only** — `internal/lsp/jsonrpc.go`. |

## 17. Event / Wire Protocol

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Typed event stream | ✅ | ✅ | Same; Go adds 5 new event kinds. |
| JSON wire format | ✅ | ✅ | Both. |
| Event sync adapter | ✅ | ✅ | Both. |
| Cache diagnostics in events | ✅ | ✅ | Both. |
| Memory citations in events | ✅ | ✅ | Both. |

## 18. Testing

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Unit tests | ✅ | ✅ | Extensive in both. |
| E2E tests | ✅ | ✅ | Go benchmarks under `benchmarks/`. |
| Integration tests | ✅ | ✅ | Both. |
| Reconciliation tests (plan mode) | ✅ | ❌ | **Go-only** — `planmode/reconcile_test.go` enforces explicit classification of every built-in. |

## 19. Build / Release

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| Single static binary (~20 MB) | ✅ | ❌ | Go compiles ~20 MB; TS was 200 MB+ Node.js. |
| Vendored dependencies | ✅ | ❌ | Go vendors all deps; TS used npm. |
| Cross-platform release | ✅ | ✅ | Both. |
| Desktop (Wails) | ✅ | ✅ | Both. |
| Docker / production | ✅ | ✅ | Both. |
| Self-update | ✅ | ✅ | Both. |
| Crash reporting | ✅ | ✅ | Both. |
| Code signing | ✅ | ❌ | **Go-only** — `.signpath/` Windows installer signing. |

## 20. Diagnostics / Monitoring

| Feature | Go | DR1 TS | Notes |
|---------|----|--------|-------|
| `reasonix doctor` | ✅ | ✅ | Both. |
| MCP startup telemetry | ✅ | ❌ | **Go-only** — `RecordStartup` with phase A duration. |
| Cache health diagnostics | ✅ | ✅ | Both. |
| Session metadata | ✅ | ✅ | Both. |
| Crash report worker | ✅ | ✅ | Both. |

---

## Phase Tracking (from `docs/migration/plan.md`)

| Phase | Focus | Status |
|-------|-------|--------|
| **Phase A** | Security (ContentSanitizer, Honeytokens, AnomalyDetector, AuditLog, ToolPolicy, NUCLEAR-YOLO, backup rotation) | **Mostly done** — ContentSanitizer, Honeytokens, AnomalyDetector, AuditLog, nonces, output validation implemented. REX-64 (NUCLEAR-YOLO git block) and REX-65 (backup rotation) still queued. |
| **Phase B** | MCP Security (influence guard, data fence, sandbox subagent, shadowing prevention) | **Assessed** — MCP client implemented; security hardening queued. |
| **Phase C** | Web Search + Tools (multi-engine search, isolate_workspace, merge_worktree) | **Partial** — standalone `mcp-web-search/` server exists; built-in tool not yet wired. |
| **Phase D** | Sub-Agent + Permission (source identity, redirect verdict, path_access gating, model fallback) | **Partial** — sub-agent system works; identity and fallback queued. |
| **Phase E** | UI Features (thinking toggle, word-wrap, workspace bar, shortcuts modal, mouse events, session picker, etc.) | **Partial** — core TUI works; 12 enhancement tickets queued. |
| **Post-P0** | Nonce delimiters, output validation, context minimization router | **Done** — REX-88 and REX-89 already implemented. REX-90 (context minimization) pending. |
