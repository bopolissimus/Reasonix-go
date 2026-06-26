# Go Migration — Jira Tickets

All tickets to be created in REX project (bopolissimus.atlassian.net).
Created from `go-migration/plan.md` gap analysis + `.research/prompt-injection-synthesis.md`.

---

## Phase A — Security (Critical Infrastructure)

### REX-62: ContentSanitizer — 5-step pipeline for tool output sanitization
**Type:** Task **Priority:** High

Port DR1 `src/security/content-sanitizer.ts` (312 lines):
1. Unicode normalization (NFKC)
2. Invisible char stripping (16 zero-width/control chars)
3. Perplexity check (Shannon entropy threshold)
4. Base64 detection (>85% base64 chars, min 40 length)
5. Pattern detection (22 regex patterns across 5 attack categories: instruction_override, role_reassignment, system_prompt_override, prompt_extraction, config_poisoning)

Plus 3 warn-only patterns: pipe-to-shell, sudo/chmod 777.

Wire into tool output pipeline — every tool result passes through before entering LLM context.

### REX-63: Honeytokens — decoy credentials for exfiltration detection
**Type:** Task **Priority:** High

Port DR1 `src/security/honeytokens.ts` (85 lines):
- 3 detection patterns (honeytoken name, fake API key format, literal string)
- Planted in 4 file locations (SSH key, AWS creds, .env, git config)
- 3 env vars (API key, DB URL, encryption key)
- `HoneytokenManager.scan()` checks tool params + tool outputs
- Fires `honeytoken_detected` audit event on match

### REX-64: NUCLEAR-YOLO git block
**Type:** Task **Priority:** Medium

Block `git add`, `git commit`, `git push` in bash tool when nuclear-yolo mode active.
Wire `/nuclear-yolo` slash command. Add UI indicator.

### REX-65: Config backup rotation + session PID-file locking
**Type:** Task **Priority:** Medium

- Config backup before every write (rotating backups)
- PID-file session locking for concurrent safety
- Session lock check before switch via picker
- EPERM handling for stale PID (sandbox compat)
- `REASONIX_CONFIG_PATH` + `REASONIX_API_KEYS_PATH` env var support

---

## Phase B — MCP Security Hardening

### REX-66: MCP influence guard — read/write tier tracking
**Type:** Task **Priority:** Medium

Port DR1 `src/mcp/influence-guard.ts`:
- Read/write tier tracking per MCP server
- Subagent fork for untrusted MCP
- Bypass guard in YOLO/nuclear-yolo mode

### REX-67: MCP <data> fence awareness
**Type:** Task **Priority:** Medium

From synthesis (§4.2), the-main-thread, aws-bedrock-guardrails:
- Wrap MCP tool descriptions in `<data source="mcp-description:...">`
- Wrap MCP results in `<data>` fences with per-request nonces
- Generate fence instructions for system prompt
- Note: standalone fences are NOT a security boundary (see REX-60). Use as defense-in-depth only, paired with nonces.

### REX-68: MCP sandbox subagent
**Type:** Task **Priority:** Low

- StubRegistry for sandbox tools
- MCP sandbox subagent spawner
- Enforcement gate
- `mcp-sandbox` subagent type

### REX-69: Tool shadowing prevention + poisoning blocklist
**Type:** Task **Priority:** Medium

- Prevent MCP tools from shadowing built-in tool names
- Expand poisoning blocklist
- Propagate security posture into forked registries

---

## Phase C — Web Search + Tools

### REX-70: Multi-engine web search (Exa + Tavily)
**Type:** Task **Priority:** High

Implement `web_search` tool using APIs in `~/.reasonix/api-keys.json`:
- **Exa** (`exa` key) — neural search with content extraction
- **Tavily** (`tavily` key) — AI-optimized search
- Random engine selection per call (avoid rate limits)
- Health gate: if either engine fails, pause both for 12h cooldown, lightweight probe on expiry
- Config in `[tools]` TOML section: engines, cooldown, probe interval
- Go-idiomatic: use `net/http`, not undici

### REX-71: isolate_workspace + merge_worktree tools
**Type:** Task **Priority:** Low **Status:** Closed — won't implement

- `isolate_workspace` — create Git worktree for safe refactors
- `merge_worktree` — merge completed worktree back and cleanup
- **Decision:** Not implementing. Git worktrees add complexity for marginal gain — users can manually `git worktree` if needed.

---

## Phase D — Sub-Agent + Permission Hardening

### REX-72: Sub-agent source identity on permission prompts
**Type:** Task **Priority:** Medium **Refs:** REX-31, REX-32

- Thread sub-agent identity through permission Gate
- Show sub-agent identity on approval prompts
- Add sub-agent source infrastructure to pause gate payloads

### REX-73: Redirect verdict for permission prompts
**Type:** Task **Priority:** Low **Refs:** REX-21

- Add redirect verdict type to Decision enum
- Wire redirect UI: "tell me what to do instead"
- User can redirect the agent's action without denying entirely

### REX-74: Path_access gate queuing
**Type:** Task **Priority:** Low

- Queue parallel path_access gates
- Prevent interleaving of gate prompts

### REX-75: Sub-agent model fallback (v4-pro → flash)
**Type:** Task **Priority:** Medium **Refs:** REX-15

- Auto-fallback to flash on v4-pro API failure
- Per-subagent model error handling
- Already have per-subagent model config in Go; add fallback logic

---

## Phase E — UI Features

### REX-76: Ctrl+T thinking mode toggle
**Type:** Task **Priority:** Medium

- Toggle reasoning visibility with Ctrl+T
- Full reasoning scroll when expanded
- Tool outputs stay elided in thinking mode
- Keyboard shortcut: Ctrl+T
- Underlying Go feature: `m.showReasoning` already exists; needs Ctrl+T binding

### REX-77: Word-wrap input at word boundaries
**Type:** Task **Priority:** Low

- Word-wrap input area at word boundaries (not mid-word)
- Reflow on terminal resize

### REX-78: Persistent workspace bar (Ctrl+H)
**Type:** Task **Priority:** Low **Refs:** REX-22

- Show workspace path in bottom dock
- Ctrl+H toggles visibility
- Persists across turns

### REX-79: Shortcuts modal (synced)
**Type:** Task **Priority:** Low

- Ctrl+H or dedicated key to show shortcuts overlay
- Lists all keyboard shortcuts
- Synced with actual keybindings

### REX-80: Mouse events (SGR modifiers)
**Type:** Task **Priority:** Low

- Extract SGR modifier bits for Ctrl/Shift/Meta mouse events
- Per-card expand/collapse on click
- Copy via Ctrl+click

### REX-81: Clipboard enhancements (tmux fallback, Ctrl+Y)
**Type:** Task **Priority:** Low

- Tmux fallback when OSC 52 not available
- Mouse text selection
- Ctrl+Y copies last response
- Gate behind `REASONIX_CLIPBOARD_VERBOSE`

### REX-82: Session picker dialog
**Type:** Task **Priority:** Medium

- Full-screen modal with filter, sort, inline rename
- Scroll support in picker
- Accessible from idle and during resume

### REX-83: /defaults slash command
**Type:** Task **Priority:** Low

- Interactive settings picker UI
- Model selection, effort, language, output style
- i18n descriptions (EN, zh-CN)

### REX-84: Chat/code session mode separation
**Type:** Task **Priority:** Low

- Session mode separation (chat vs code)
- Skills gating per mode
- Different tool availability per mode

### REX-85: Scroll freeze fix
**Type:** Bug **Priority:** Medium **Refs:** REX-2

- Fix scroll position jumping during streaming output

### REX-86: Cursor movement visibility fix
**Type:** Bug **Priority:** Low **Refs:** REX-8

- Fix cursor visibility during movement operations

### REX-87: Input history DOWN after UP restore
**Type:** Bug **Priority:** Low

- Restore original input when navigating down past the end of history

---

## Post-Migration P0 (from Prompt Injection Synthesis)

### REX-88: Dynamic nonce-based delimiters (StruQ + Spotlighting)
**Type:** Task **Priority:** High

From `.research/prompt-injection-synthesis.md` §4.2:
- Per-request random nonce for untrusted data wrapping
- Cryptographic boundary tokens (SQL prepared statement analog)
- Sanitize nonce from user input before wrapping
- Integrate into message assembly path in `internal/agent/`
- Sources: the-main-thread, AWS Bedrock Guardrails

### REX-89: Output validation / content allowlist
**Type:** Task **Priority:** High

From `.research/prompt-injection-synthesis.md` §4.3:
- Scan LLM responses for unexpected URLs, encoded data, HTML before display/execution
- Block or flag disallowed patterns
- Addresses Bunq-class attacks where LLM generates attacker-controlled output

### REX-90: Context minimization router
**Type:** Task **Priority:** High

From `.research/prompt-injection-synthesis.md`:
- Only pass data fields the current task needs to the LLM
- Reduce injection surface by not dumping entire records into context
- Semantic routing to determine which fields are needed per query type

---

## Summary

| Phase | Tickets | Count |
|-------|---------|-------|
| A — Security | REX-62 through REX-65 | 4 new |
| B — MCP Security | REX-66 through REX-69 | 4 new |
| C — Web Search + Tools | REX-70, REX-71 | 2 new |
| D — Sub-Agent + Permission | REX-72 through REX-75 | 4 new |
| E — UI Features | REX-76 through REX-87 | 12 new |
| Post-Migration P0 | REX-88 through REX-90 | 3 new |
| **Total new tickets** | | **29** |
| **Already created** | REX-58, REX-59, REX-60, REX-61 | 4 |
| **Grand total** | | **33** |
