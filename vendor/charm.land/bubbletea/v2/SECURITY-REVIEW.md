# Security Review: charm.land/bubbletea v2.0.7 (vendored)

## Summary
- **Target Type**: Source Code / Vendored Dependency
- **Modes Used**: A (read-only static analysis)
- **Findings**: 0 Critical, 0 High, 1 Medium
- **Risk Level**: Low
- **Rule of Two Violation**: No — bubbletea is a pure TUI framework. It processes only terminal input (stdin) and writes only to terminal output (stdout) plus optional debug log files. It holds no secrets, makes no network calls, and has no agentic tooling.
- **Confidence**: High
- **Dependencies reviewed**: 0 transitive (vendored as leaf — no bubbletea-owned `go.mod` in vendor tree; `ultraviolet`, `colorprofile`, `x/ansi`, `x/term`, `cancelreader` live in sibling vendor dirs)
- **CVEs checked**: 0 applicable (searched `bubbletea` in NVD/GitHub Advisory/OSV — no known CVEs against v2.0.7 at time of review)
- **Release notes analyzed**: Not applicable (pinned vendored version)
- **Cross-dependency chains identified**: 0

---

## Findings

### [SEC-001] Environment-controlled debug logging writes to attacker-chosen path (Medium)

- **Category**: Information Disclosure
- **OWASP Reference**: LLM02 (Sensitive Information Disclosure)
- **Location**: `tea.go:632-638`, `tea.go:1281-1289`, `tea.go:1306-1314`
- **Confidence**: Medium
- **Issue**: Two environment variables control file writes with attacker-chosen paths:

  1. **`TEA_TRACE`** (`tea.go:632-638`): If set to a non-empty path, the framework opens that file and logs **all terminal output** (every escape sequence, every rendered frame) to it. This includes whatever text the application renders — which in Reasonix's case is chat messages, model output, and potentially sensitive data.

  2. **`TEA_DEBUG`** (`tea.go:1281-1289`, `tea.go:1306-1314`): On panic, creates `bubbletea-panic-{unix_ts}.log` in the **current working directory** and writes the full panic message plus a stack trace (including file paths, line numbers, and goroutine states).

- **Attack Path**:
  1. Attacker can set `TEA_TRACE` to a path in a world-readable or attacker-accessible directory (e.g., `/tmp/`, a shared NFS mount, a container volume mount).
  2. The application runs, and all rendered terminal output — including sensitive chat content — is written to that file.
  3. Attacker reads the file and exfiltrates the data.
  
  *For `TEA_DEBUG`:*
  1. Attacker triggers a panic in the application (e.g., by sending crafted terminal input to an edge case).
  2. The CWD is attacker-readable (e.g., `/tmp/` or a shared volume).
  3. Attacker reads the panic log for stack traces and internal state.

- **Attacker-Controlled**: Partial — the env-var name is fixed, but both the **path** (`TEA_TRACE`) and the **CWD** (`TEA_DEBUG`) can be attacker-controlled if the attacker shares a filesystem, a container runtime, or sets environment variables before the process starts.
- **Guard/Mitigation Present**: `TEA_TRACE` uses `os.LookupEnv` (not `os.Getenv`), so the env var must be explicitly set. File permissions are `0o600` (owner read/write only), which prevents other local users from reading the file directly — but does not protect against same-user processes or container escape.
- **Residual Exploitability**: **Medium** — requires local access or same-machine attacker. The `0o600` permission is a real mitigation but insufficient if the path points to a shared volume or `/tmp/` (where `0o600` still allows the owning UID to read, and in containerized environments the UID may be shared).
- **Evidence**:
  ```go
  // tea.go:632-638
  tracePath, traceOk := os.LookupEnv("TEA_TRACE")
  if traceOk && len(tracePath) > 0 {
      if f, err := os.OpenFile(tracePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600); err == nil {
          p.logger = log.New(f, "bubbletea: ", log.LstdFlags|log.Lshortfile)
      }
  }
  ```
- **Remediation**: If Reasonix doesn't use `TEA_TRACE`/`TEA_DEBUG` in production, the simplest fix is to **not ship** these features — conditionally compile them out (`//go:build debug`). If they're needed, restrict `TEA_TRACE` to an allowlist of safe directories (`/var/log/`, a hardcoded log dir) or add a compile-time flag to disable them entirely. For `TEA_DEBUG`, write panic logs to `os.TempDir()` rather than CWD, where they're less likely to be world-readable.

---

### No other findings

Searched for and **confirmed absent** in this codebase (grep confirmed each):

| Pattern | Result |
|---------|--------|
| `exec.Command` with user input | Only `exec.go`'s `ExecProcess` — intentional API, caller provides command |
| `os.exec` / `syscall.Exec` | Not present |
| SQL / database | Not present |
| `pickle` / `yaml.load` / deserialization | Not present (Go library, no deserialization sinks) |
| `eval` / template injection | Not present |
| HTTP client / `net.Dial` | Not present |
| Hardcoded secrets, API keys, tokens | Not present |
| Path traversal (`filepath.Join` with `../`) | Not present |
| `innerHTML` / `dangerouslySetInnerHTML` | N/A (not a web framework) |
| Cryptography (homemade or weak) | Not present |

**Input handling note**: All terminal input flows through `charmbracelet/ultraviolet`'s `TerminalReader`, which parses ANSI escape sequences, OSC52, and keyboard/mouse protocols. Input is delivered as strongly-typed Go messages (`KeyPressMsg`, `MouseClickMsg`, `PasteMsg`, etc.) — not as raw strings that could be misinterpreted. No injection surface exists between terminal input and the application's `Update` function.

---

## Integration Contract

### Input bounds
- **Accepted**: `io.Reader` via `WithInput()` (defaults to `os.Stdin`). Terminal input in raw mode — ANSI escape sequences, OSC52, keyboard protocols (XTerm, Kitty, Windows Console API), mouse protocols (X10, SGR), bracketed paste.
- **Rejected**: None at the framework level. All input bytes are forwarded to `ultraviolet.TerminalReader` for parsing. Parse errors are sent as errors to the program's error channel; malformed sequences may produce no message rather than crashing.
- **Max safe size**: Individual terminal events are bounded by the terminal emulator's buffer (typically 4 KiB for paste events, much smaller for key/mouse events). No explicit size limit in bubbletea itself. For paste events, the full paste content arrives as a single `PasteMsg` — caller should validate size if pasted content could be large.

### Output shape
- **Return type**: `tea.Model` → `tea.View` → rendered to `io.Writer` (defaults to `os.Stdout`). The output is a stream of ANSI escape codes (cursor positioning, styling, screen updates) and the rendered text content.
- **Nil guarantees**: `Program.Run()` returns `(Model, error)`. Error is non-nil on panic (`ErrProgramPanic`), kill (`ErrProgramKilled`), interrupt (`ErrInterrupted`), or context cancellation. Model is the final model state.
- **Encoding guarantees**: Output is valid UTF-8 with ANSI escape sequences. `View.Content` is treated as a "styled string" — text with embedded ANSI style codes. The framework does **not** strip, escape, or sanitize content; it renders it as-is.
- **Enum guarantees**: All message types are exported typed wrappers. Mouse buttons are X11-based (1–11). Key codes are enumerated constants. Mode reports use `ansi.Mode` and `ansi.ModeSetting`.

### Side effects
- **I/O**: Writes to `p.output` (default `os.Stdout`). Reads from `p.input` (default `os.Stdin`). Opens files only when `TEA_TRACE` is set or `TEA_DEBUG` is enabled on panic. Opens files via `LogToFile()` when called by the application. Reads/writes terminal state via `term.GetState`/`term.Restore`/`term.MakeRaw`.
- **Allocations**: Per-frame: view render buffer, ticker channel. Per-input: message allocation for each terminal event. Unbounded goroutine spawn for `BatchMsg` — `len(msg)` goroutines per batch.
- **Global state**: Modifies terminal mode (raw mode, mouse mode, bracketed paste, keyboard enhancements). Installs signal handlers for `SIGINT`, `SIGTERM`, `SIGWINCH` (Unix). Restores terminal state on shutdown via `sync.Once`.
- **Goroutine safety**: `Program.Send()` is safe for concurrent use. `Program.Run()` must be called from a single goroutine. Internal mutex (`p.mu`) protects the output buffer. The program's `Update` function is called serially from the event loop — caller's model must not have concurrent access from other goroutines.

### Error modes
- **Returned errors**:
  - `ErrProgramPanic` — the program's `Update`, `View`, or `Init` panicked, or a `Cmd` panicked. Terminal state is restored.
  - `ErrProgramKilled` — `Program.Kill()` was called or the external context was cancelled.
  - `ErrInterrupted` — `SIGINT` received or `InterruptMsg` sent.
  - `fmt.Errorf("bubbletea: ...")` — TTY initialization failures, input reader errors.
- **Panics**: The framework itself does not panic intentionally. It catches panics in user code (`Update`, `View`, `Init`, `Cmd`) and recovers. When `WithoutCatchPanics()` is used, panics propagate to the caller and the terminal is left in raw mode (unusable state).
- **Timeouts**: Input read loop has no timeout — blocks until input arrives. Render ticker runs at `fps` Hz (default 60, max 120). `waitForReadLoop` has a 500ms deadline.

### Resource bounds
- **Memory**: Output buffer grows with frame size (proportional to terminal dimensions). Render buffer allocates per frame. No limit on concurrent `BatchMsg` goroutines — caller should bound batch sizes.
- **CPU**: Event loop is single-threaded. Render runs at `fps` Hz. Command goroutines run concurrently with no bound — a `BatchMsg` with N commands spawns N goroutines.
- **Stack**: User callbacks (`Update`, `View`, `Init`, `Cmd`) run on the event loop's stack. No recursion in the framework itself.

### Explicit non-guarantees
- Does **NOT** sanitize, strip, or escape ANSI escape sequences in `View.Content` — caller is responsible for not rendering attacker-controlled text as styled content. If an attacker can inject text into the view, they can inject arbitrary terminal escape sequences (cursor repositioning, screen clearing, link creation).
- Does **NOT** validate the content of `RawMsg` — caller can send arbitrary bytes to the terminal.
- Does **NOT** validate clipboard content from `ReadClipboard()` / `ClipboardMsg` — the terminal's OSC52 response is trusted as-is.
- Does **NOT** limit paste event size — `PasteMsg` may contain arbitrarily large strings.
- Does **NOT** authenticate, encrypt, or integrity-check terminal I/O — the terminal is assumed trusted.
- Does **NOT** bound goroutine count for `BatchMsg` or concurrent command execution.
- `TEA_TRACE` does **NOT** filter or redact content — all terminal output including sensitive rendered text is logged.
- `TEA_DEBUG` does **NOT** restrict the output directory — panic logs are written to CWD.

### Integration examples

```go
// CORRECT: Wrap View.Content setting to strip ANSI from user input
func safeView(unsafeContent string) tea.View {
    v := tea.NewView("")
    // Strip ANSI escape sequences from user-supplied content before rendering
    v.SetContent(stripANSI(unsafeContent))
    return v
}

// CORRECT: Bound paste size
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.PasteMsg:
        if len(msg.String()) > 1<<20 { // 1 MiB limit
            return m, nil // drop oversized paste
        }
        // process paste
    }
    return m, nil
}
```

```go
// INCORRECT: Rendering attacker-controlled text without stripping ANSI
// If `userInput` contains "\x1b[2J" (clear screen) or "\x1b]8;;file:///etc/passwd\x1b\\",
// the terminal will execute those escape sequences.
func (m model) View() tea.View {
    v := tea.NewView("")
    v.SetContent("You said: " + m.userInput) // UNSAFE
    return v
}
```

## Assessment

**Safe to use.** Bubble Tea v2.0.7 is a well-architected TUI framework with a minimal and well-defined attack surface. It operates purely on terminal I/O with no network access, no persistence, and no injection sinks. The only finding (`TEA_TRACE`/`TEA_DEBUG` debug logging) is a defense-in-depth concern that requires local attacker access to exploit and is mitigated by file permissions (`0o600`). For Reasonix's chat interface use case, this dependency does not introduce meaningful risk.
