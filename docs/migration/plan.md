# Go Migration Plan

> **Status:** Decisions locked. Tickets filed. Implementation queued.
> **Date:** 2026-07-24 (plan), updated 2026-07-25 (decisions locked)
> **Ticket index:** [tickets.md](tickets.md)

---

## Resolved Decisions

| # | Question | Answer |
|---|----------|--------|
| 1 | Security scope | ContentSanitizer + Honeytokens + AnomalyDetector (with fine-tuning + v4-flash review) + AuditLog + ToolPolicy. ContextSegmenter → eval harness only (fences not reliable per research) |
| 2 | Local proxy | Exact + semantic caches removed (redundant with DeepSeek prefix cache). Proxy itself deferred TBD |
| 3 | Constraints system | TBD |
| 4 | UI parity | Feature parity where possible; Go-idiomatic Bubble Tea equivalents where not. Same keystrokes, same look. Every feature gets a ticket; unimpl features get follow-ups |
| 5 | web_search | Use Exa + Tavily APIs from `~/.reasonix/api-keys.json` |
| 6 | Migration order | Security first (A → B) → Web Search (C) → Sub-agent (D) → UI (E) |
| 7 | Planning | Batched. Tickets created for all features before any implementation |
| 8 | Testing | Full parity with DR1. Flag blockers as tickets |
| 9 | Features to skip | Exact cache, semantic cache, ContextSegmenter as security defense, local proxy (deferred) |
| 10 | Go-idiomatic | Prioritize idiomatic Go over 1:1 port fidelity |

---

## Phases

### Phase A — Security (Critical Infrastructure)

| Ticket | Feature | Status |
|--------|---------|--------|
| REX-58 | AnomalyDetector — behavioral detection (6 algorithms, EMA baseline, v4-flash review) | Created |
| REX-59 | AuditLog — centralized event log with secret redaction (10 event types) | Created |
| REX-60 | ContextSegmenter — eval harness only (NOT a security defense) | Created |
| REX-61 | ToolPolicy — per-tool allowlist, shell pipeline splitting discussion | Created |
| REX-62 | ContentSanitizer — 5-step pipeline, 22 regex patterns | Queued |
| REX-63 | Honeytokens — decoy credentials for exfiltration detection | Queued |
| REX-64 | NUCLEAR-YOLO git block | Queued |
| REX-65 | Config backup rotation + session PID-file locking | Queued |

### Phase B — MCP Security Hardening

| Ticket | Feature | Status |
|--------|---------|--------|
| REX-66 | MCP influence guard (read/write tier tracking) | Queued |
| REX-67 | MCP data fence awareness (defense-in-depth, paired with nonces) | Queued |
| REX-68 | MCP sandbox subagent | Queued |
| REX-69 | Tool shadowing prevention + poisoning blocklist | Queued |

### Phase C — Web Search + Tools

| Ticket | Feature | Status |
|--------|---------|--------|
| REX-70 | Multi-engine web search (Exa + Tavily, keys in api-keys.json) | Queued |
| REX-71 | isolate_workspace + merge_worktree | Queued |

### Phase D — Sub-Agent + Permission Hardening

| Ticket | Feature | Status |
|--------|---------|--------|
| REX-72 | Sub-agent source identity on prompts (REX-31, REX-32) | Queued |
| REX-73 | Redirect verdict (REX-21) | Queued |
| REX-74 | Path_access gate queuing | Queued |
| REX-75 | Sub-agent model fallback v4-pro→flash (REX-15) | Queued |

### Phase E — UI Features

| Ticket | Feature | Status |
|--------|---------|--------|
| REX-76 | Ctrl+T thinking mode toggle | Queued |
| REX-77 | Word-wrap input at word boundaries | Queued |
| REX-78 | Persistent workspace bar (Ctrl+H) (REX-22) | Queued |
| REX-79 | Shortcuts modal | Queued |
| REX-80 | Mouse events (SGR modifiers) | Queued |
| REX-81 | Clipboard enhancements (tmux fallback, Ctrl+Y) | Queued |
| REX-82 | Session picker dialog | Queued |
| REX-83 | /defaults slash command | Queued |
| REX-84 | Chat/code session mode separation | Queued |
| REX-85 | Scroll freeze fix (REX-2) | Queued |
| REX-86 | Cursor movement visibility fix (REX-8) | Queued |
| REX-87 | Input history DOWN after UP restore | Queued |

### Post-Migration P0 (from Prompt Injection Research Synthesis)

| Ticket | Feature | Status |
|--------|---------|--------|
| REX-88 | Dynamic nonce-based delimiters (StruQ + Spotlighting) | Queued |
| REX-89 | Output validation / content allowlist | Queued |
| REX-90 | Context minimization router | Queued |

---

## Implementation Order

1. **Phase A** — Security (ContentSanitizer first, then Honeytokens, AnomalyDetector, AuditLog, ToolPolicy)
2. **Phase B** — MCP Security
3. **Phase C** — Web Search + Tools
4. **Phase D** — Sub-Agent + Permission
5. **Phase E** — UI Features
6. **Post-migration P0** — Nonce delimiters, output validation, context minimization

---

## Reference Documents

| Document | Path |
|---|---|
| Ticket index | [go-migration/tickets.md](tickets.md) |
| TODO checklist | [go-migration/todo.md](todo.md) |
| Go codebase survey | `docs/survey/GO-CODEBASE-SURVEY.md` |
| Gap analysis | `docs/survey/gap-analysis.md` |
| Prompt injection synthesis | `docs/security/prompt-injection-synthesis.md` |
| DR1 codebase survey | `../DR1/.agents/survey/CODEBASE-SURVEY.md` |
