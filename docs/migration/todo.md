# Go Migration — TODO Checklist

> Ticket index: [go-migration/tickets.md](tickets.md)
> Plan: [go-migration/plan.md](plan.md)

---

## Phase 0: Meta-Planning ✅

- [x] Security scope decided (ContentSanitizer + Honeytokens + AnomalyDetector + AuditLog + ToolPolicy)
- [x] ContextSegmenter → eval harness only
- [x] Local proxy caches removed; proxy deferred
- [x] UI parity approach decided (same look, same keystrokes, Go-idiomatic)
- [x] web_search using Exa + Tavily from api-keys.json
- [x] Migration order: security-first, batched, full testing parity
- [x] All 33 tickets documented in tickets.md

---

## Phase A: Security

- [ ] REX-58 — AnomalyDetector (with fine-tuning + v4-flash review)
- [ ] REX-59 — AuditLog
- [ ] REX-60 — ContextSegmenter eval harness
- [ ] REX-61 — ToolPolicy + shell pipeline discussion
- [ ] REX-62 — ContentSanitizer
- [ ] REX-63 — Honeytokens
- [ ] REX-64 — NUCLEAR-YOLO git block
- [ ] REX-65 — Config backup + session locking

## Phase B: MCP Security

- [ ] REX-66 — MCP influence guard
- [ ] REX-67 — MCP data fence awareness
- [ ] REX-68 — MCP sandbox subagent
- [ ] REX-69 — Tool shadowing prevention

## Phase C: Web Search + Tools

- [ ] REX-70 — Multi-engine web search (Exa + Tavily)
- [x] REX-71 — isolate_workspace + merge_worktree (closed — won't implement)

## Phase D: Sub-Agent + Permission

- [ ] REX-72 — Sub-agent source identity
- [ ] REX-73 — Redirect verdict
- [ ] REX-74 — Path_access gate queuing
- [ ] REX-75 — Sub-agent model fallback

## Phase E: UI Features

- [ ] REX-76 — Ctrl+T thinking mode
- [ ] REX-77 — Word-wrap input
- [ ] REX-78 — Workspace bar (Ctrl+H)
- [ ] REX-79 — Shortcuts modal
- [ ] REX-80 — Mouse events
- [ ] REX-81 — Clipboard enhancements
- [ ] REX-82 — Session picker dialog
- [ ] REX-83 — /defaults command
- [ ] REX-84 — Chat/code mode separation
- [ ] REX-85 — Scroll freeze fix
- [ ] REX-86 — Cursor visibility fix
- [ ] REX-87 — Input history fix

## Post-Migration P0

- [ ] REX-88 — Nonce-based delimiters (StruQ + Spotlighting)
- [ ] REX-89 — Output validation / content allowlist
- [ ] REX-90 — Context minimization router

---

## Verification (per phase)

- [ ] `make vet` passes
- [ ] `make test` passes
- [ ] `make build` produces working binary
- [ ] Manual smoke test of new features
