# Go Reasonix Integration Contracts

Generated 2026-06-26. Each contract documents what a package guarantees and does NOT guarantee at its public API boundary, enabling cross-module attack-chain analysis.

Confidence markers: `[full-read]` = verified in source, `[inferred]` = from interface + call sites.

---

## agent

### Agent: turn execution loop

- **Accepts**: `ctx context.Context`, `input string` (user message text) via `Run(ctx, input) error` [`agent.go:740`]
- **Returns**: `error` — nil on clean completion, non-nil on provider error, stream failure, or cancellation
- **Side effects**: Appends user + assistant + tool messages to `Session`. Emits events to `Sink` (TurnStarted, Reasoning, Text, ToolDispatch, ToolResult, Usage, TurnDone). Reads nonce from context via `DataNonceFromContext(ctx)` [`agent.go:778`]. Runs ContentSanitizer on tool output via `maybeSanitize()` [`agent.go:2057`]. Runs HoneytokenManager.Scan() before sanitization [`agent.go:2041`].
- **Error modes**: Provider stream error → returned as error. Max steps exceeded → returns nil (graceful stop, not error). Context cancelled → returns context error. Stream interrupted → auto-retry up to `maxStreamRecoveries` (3) [`agent.go:789`].
- **Panics**: None in normal operation. `wrapNonce` is defensive (empty nonce = passthrough) [`agent.go:2333`].
- **Does NOT**: Does NOT apply system prompt — caller (boot) sets it via `Session`. Does NOT manage conversation compaction — `control.Controller` does. Does NOT validate tool arguments — each tool's `Execute` does. Does NOT guarantee nonce is cryptographically unique — 4-byte random (1 in 4B chance of collision) [`security/nonce.go:18`].
- **Refs**: `agent.go:740-810` (Run loop), `agent.go:2031-2097` (maybeSanitize), `agent.go:2327-2341` (wrapNonce), `agent.go:606-660` (Options)

### Runner interface

- **Accepts**: `ctx context.Context`, `input string` — the composed turn text
- **Returns**: `error`
- **Side effects**: Runs the agent turn end-to-end. May block on tool approvals (the Gate interface).
- **Refs**: `agent.go:66-68` (Runner type definition)

---

## control

### Controller: session driver

- **Accepts**: Commands via `Send(input)`, `Submit(input)`, `Run(ctx, input)`, `Approve(id, ...)`, `Cancel()`, `SetPlanMode(bool)`, etc. Construction via `New(Options)` [`controller.go:219-310`].
- **Returns**: Events via `Sink`. `Send`/`Submit` return immediately (non-blocking); `Run` blocks until turn completes.
- **Side effects**: Owns session lifecycle (NewSession, Resume, Compact). Manages plan-mode state, approval posture (ask/auto/yolo), goals FSM, MCP plugin host, checkpoints, hooks. Compose() injects turn-tail blocks (plan-mode marker, reasoning language, memory updates, background jobs, nonce context) [`input.go:145-200`].
- **Error modes**: Concurrency guard: `Send` while running → notice emitted, input discarded [`controller.go:740`]. Turn errors surfaced via `TurnDone.Err`. Plan-mode plan approval timeout → plan discarded.
- **Goroutine safety**: `mu` protects run-state (planMode, autoApprove, cancel). `approval`, `goals`, `checkpoints`, `memory`, `mcp` each have independent locks — off `c.mu` [`controller.go:83-138`].
- **Does NOT**: Does NOT parse slash commands — `Submit` dispatches. Does NOT resolve @-references — `runRefTurn` does. Does NOT own tool registry — `executor` does. Does NOT persist sessions — `agent.Session` does via `SaveSession`.
- **Refs**: `controller.go:66-160` (struct), `controller.go:1103-1128` (Run), `input.go:145-200` (Compose), `controller.go:521-530` (turn lifecycle)

---

## planmode

### Policy: plan-mode safety gate

- **Accepts**: `Call{Name, ReadOnly, Untrusted, Safety, Args}` via `Decide(call) Decision` [`policy.go:145`]
- **Returns**: `Decision{Blocked bool, Message string}` — blocked=true with reason if the tool is unsafe in plan mode
- **Side effects**: Pure function — no I/O, no mutable state. Decision logic: ReadOnly+tools with PlanSafety=Unsafe are blocked; Untrusted tools (MCP) fail closed unless declared safe; unknown tools → blocked.
- **Error modes**: Malformed bash args (invalid JSON) → blocked with "malformed shell quoting" message [`policy.go:336`]. Command not in safe list → blocked with command-specific message [`policy.go:345`].
- **Panics**: None
- **Does NOT**: Does NOT execute commands — only decides. Does NOT use AST parsing for shell commands — current implementation is substring matching on `bashMetachars` [`policy.go:110`]. Does NOT handle `;`, `&&`, `||` — blocked. Does NOT validate file paths for path traversal.
- **Refs**: `policy.go:108-112` (bashMetachars + safeBashCommands), `policy.go:145-350` (Decide), `policy.go:297-350` (decideBash)

---

## tool

### Tool interface

- **Accepts**: `Name() string`, `Description() string`, `Schema() json.RawMessage`, `ReadOnly() bool`, `Execute(ctx, args) (string, error)`
- **Returns**: `Execute` returns output string + error. Error strings are distinct per failure mode (e.g. "old_string not found" vs "old_string is not unique") for LLM self-correction.
- **Side effects**: `ReadOnly() bool` governs parallel dispatch — agent parallelises only when ALL calls in a batch are ReadOnly [`tool.go:24-28`]. Writers are serialized.
- **Refs**: `tool.go:16-28` (interface), `tool.go:45-60` (Registry)

---

## sandbox

### Shell sandbox

- **Accepts**: `Spec{WorkspaceRoot, AllowWrite, Network}` — defines confinement boundaries [`sandbox.go:30`]
- **Returns**: `ResolveShell()` returns shell path. `Available()` returns bool (Seatbelt on macOS, bwrap on Linux).
- **Side effects**: macOS: spawns sandbox-exec. Linux: spawns bwrap. Enforces that file writes stay within workspaceRoot ∪ allowWrite. Network: if disabled, blocks egress.
- **Does NOT**: Does NOT parse shell syntax — delegates to OS sandbox. Does NOT prevent `rm -rf /` inside workspaceRoot — tool-level policy must gate destructive commands.
- **Refs**: `sandbox.go:30-80`

---

## security

### ContentSanitizer: 5-step injection detection

- **Accepts**: `Sanitize(content string, source string) SanitizeResult` — tool output text [`sanitize.go:115`]
- **Returns**: `SanitizeResult{Content, Warnings, Stats}` — cleaned content + any warnings
- **Pipeline**: (1) Unicode normalization, (2) Strip invisible chars, (3) Perplexity/entropy check, (4) Base64 detection, (5) 17+3 regex pattern detection [`sanitize.go:117-162`]
- **Side effects**: None — pure function
- **Error modes**: All errors are returned as warnings, not errors. Never panics.
- **Does NOT**: Does NOT block output — only warns. Does NOT detect tag-closure attacks (`</data>`) — that's the nonce system's job. Does NOT scan for prompt extraction patterns inside fenced code blocks.
- **Refs**: `sanitize.go:17-80` (patterns), `sanitize.go:100-165` (Sanitize)

### DataNonce: per-turn boundary tokens

- **Accepts**: `DataNonce() string` — no input [`nonce.go:18`]
- **Returns**: 8 hex chars (4 random bytes from crypto/rand)
- **DataNonceSafe(content string) string**: guarantees returned nonce does NOT appear in content [`nonce.go:29`]
- **Refs**: `nonce.go:1-35`

---

## shell (REX-61)

### Parse: shell command → AST

- **Accepts**: `Parse(cmd string) (*syntax.File, error)` — shell command string [`parse.go:18`]
- **Returns**: `*syntax.File` AST on success, error on parse failure (including empty command). AST nodes: File→Stmts→Cmd (CallExpr, BinaryCmd, Subshell, Block, etc.).
- **Side effects**: None — pure parser. Uses `mvdan.cc/sh/v3/syntax.NewParser()`.
- **Error modes**: Empty command → "empty command". Invalid syntax → "shell parse error: <details>". Parse failures = blocked (fail-closed).
- **Resource bounds**: No input size limit internally — **caller MUST wrap with `io.LimitReader`** to prevent OOM from unterminated heredoc or deeply nested input.
- **Does NOT**: Does NOT limit recursion depth on `$($(...))` — caller must bound. Does NOT validate commands are safe to execute. Does NOT reject shell metacharacters — AST faithfully represents `|`, `;`, `&&`, `$()`, etc. Does NOT canonicalize file paths in redirections.
- **Refs**: `parse.go:1-28`, `analyze.go:1-260`, `policy.go:1-45`

### Analyze: AST → safety decisions

- **Accepts**: `Analyze(f *syntax.File, policy *Policy) Result` — parsed AST + policy [`analyze.go:32`]
- **Returns**: `Result{Allowed, Message, Segments}` — per-segment decisions
- **Policy**: `AllowPipe`, `AllowInputRedir`, `AllowSubshell`, `MaxDepth`, `IsSafeCommand func(argv []string) bool` [`policy.go:5-35`]
- **Tier 1 (allowed)**: `|` (pipeline — each segment checked independently), `<`, `<<` (input redirections), `()` (subshells — recursed). **Tier 2 (blocked)**: `>`, `>>` (output redirections), `&` (background). **Tier 3 (blocked — stub)**: `;`, `&&`, `||`, `$()`, backticks — await interp sandbox.
- **Does NOT**: Does NOT execute commands — only classifies. Does NOT track `cd` state across `;` boundaries (Tier 3 will add this). Does NOT detect path traversal in redirections.
- **Refs**: `analyze.go:1-260`, `policy.go:1-45`

---

## config

### Config loading

- **Accepts**: `LoadForRoot(root string) (*Config, error)` — workspace root. Resolution order: flag → `./reasonix.toml` → `~/.reasonix/config.toml` → legacy XDG → built-in defaults. ConfigVersion=3 schema check.
- **Returns**: `*Config` with all sections populated, or error on parse failure.
- **Side effects**: Reads files from disk. Merges legacy `~/.reasonix/config.json` for MCP servers + API keys. Expands `${VAR}` in string values.
- **Does NOT**: Does NOT validate provider connectivity — boot.go does. Does NOT decrypt or manage secrets — `CredentialsStore` delegates to keyring.
- **Refs**: `config.go:62-70` (struct), `config.go:1240-1280` (Default)

---

## boot

### Build: assembles Controller from config

- **Accepts**: `Build(opts Options) (*Controller, error)` — config + CLI options [`boot.go:140`]
- **Returns**: Fully wired `*control.Controller` ready to accept turns, or error.
- **Side effects**: Resolves provider from config → creates HTTP client. Loads memory (REASONIX.md hierarchy). Discovers skills. Sets up MCP plugin host. Creates agent with security components, sandbox, hooks. Assembles system prompt (base + memory + skills index + decision/language policies). All one-time setup — not per-turn.
- **Does NOT**: Does NOT start a run loop — Controller does. Does NOT handle per-turn state — Compose() does.
- **Refs**: `boot.go:140-300`, `boot.go:940-1050`

---

## event

### Event stream

- **Accepts**: `Event{Kind, Text, Tool, Approval, ...}` — producer constructs typed events [`event.go:80-120`]
- **Returns**: Via `Sink.Emit(Event)` — dispatched to frontend. 21 event kinds: TurnStarted, Reasoning, Text, Message, ToolDispatch, ToolResult, Usage, Notice, Phase, ApprovalRequest, AskRequest, TurnDone, CompactionStarted, CompactionDone, ToolProgress, MCPSurfaceReady, Retrying, Steer, etc. [`event.go:27-79`]
- **Side effects**: `Sink` implementations may render to terminal, forward to websocket, or buffer. `Sync()` wrapper adds a channel → synchronous semantics.
- **Does NOT**: Does NOT carry session state — event stream is fire-and-forget. Does NOT buffer — Sink must not block (use Sync wrapper if needed).
- **Refs**: `event.go:1-305`

---

## provider

### Provider interface

- **Accepts**: `Stream(ctx, msgs []Message) (<-chan Chunk, error)` — messages + context [`provider.go:25`]
- **Returns**: Channel of `Chunk{Type, Text, ToolCall, Usage, ...}`. Channel closes on completion or error.
- **Chunk types**: `ChunkText`, `ChunkReasoning`, `ChunkToolCall`, `ChunkToolCallDone`, `ChunkDone`, `ChunkError`.
- **Side effects**: Makes HTTP requests to LLM API. `NormalizeMessages(msgs)` fast-path: returns input slice unchanged (zero allocation) when well-formed [`provider.go:131-134`].
- **Does NOT**: Does NOT manage API keys — caller provides via `api_key_env`. Does NOT handle retries — agent does. Does NOT compact context — controller does.
- **Refs**: `provider.go:1-150`
