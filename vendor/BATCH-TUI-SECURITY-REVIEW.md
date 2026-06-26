# Batch Security Review: Vendored TUI Utility Dependencies

**Review date:** 2025-07-17
**Scope:** Six vendored TUI/terminal utility packages
**Method:** Static analysis (Mode A — read, grep, glob)
**Verdict:** **No blocking issues.** All packages are pure computation / terminal I/O with no network, no secrets, and bounded attack surface.

---

## 1. `github.com/xo/terminfo`

**Path:** `vendor/github.com/xo/terminfo/`
**Files:** 8 `.go` files (caps.go, capvals.go, color.go, dec.go, load.go, param.go, stack.go, terminfo.go)

### Summary
Pure-Go terminfo database parser and capability string interpolator. Reads terminfo binary files from the local filesystem per the terminfo(5) search path (`$TERMINFO`, `~/.terminfo`, `$TERMINFO_DIRS`, `/etc/terminfo`, `/lib/terminfo`, `/usr/share/terminfo`).

### Security Findings

**No critical, high, or medium findings.**

| Check | Result |
|-------|--------|
| Network I/O | None |
| Shell execution | None |
| Hardcoded secrets | None |
| Unsafe / syscall | None |
| User-controlled path traversal | The search path is driven by `$TERMINFO`, `$TERMINFO_DIRS`, `$HOME/.terminfo` + hardcoded fallbacks. These are server-controlled env vars, not attacker-controlled input. The `Open()` function constructs paths via `path.Join(dir, name[0:1], name)` — the `name` comes from `$TERM` which is under user control, but `path.Join` prevents traversal outside `dir`. |
| Binary parsing robustness | The `Decode()` function has bounds checks throughout (`if len(buf) >= maxFileLength`, `if d.n-d.pos < capLength(h)`, etc.) and returns typed errors on malformed input. No panics on attacker-controlled binary data. |
| Concurrency | Uses `sync.RWMutex` around the terminfo cache — safe for concurrent loads. |

### Integration Contract

#### Input bounds
- **Accepted:** Valid terminfo binary format (byte slice), terminfo capability name strings for lookups
- **Rejected:** Files exceeding `maxFileLength`, files with invalid magic numbers, truncated headers, unterminated name strings
- **Max safe size:** Bounded by `maxFileLength` (checked in Decode)

#### Output shape
- **Return type:** `*Terminfo` struct with maps of bool/num/string capabilities, or typed `Error` constants
- **Nil guarantees:** `Decode` never returns a nil `*Terminfo` with a nil error; all error paths return `nil, err`

#### Side effects
- **I/O:** Reads files from the local filesystem only (terminfo database directories). No writes.
- **Allocations:** Allocates maps proportional to capability counts in the terminfo file (bounded by file size)
- **Global state:** Populates an in-memory cache (`termCache`) protected by `sync.RWMutex`
- **Goroutine safety:** Safe for concurrent use (cache reads under RLock, writes under Lock)

#### Error modes
- **Returned errors:** All errors are typed `Error` constants (e.g. `ErrInvalidMagic`, `ErrFileNotFound`, `ErrEmptyTermName`)
- **Panics:** None. All error conditions return typed errors.

#### Resource bounds
- **Memory:** O(file-size) — maps scale with capability counts, bounded by `maxFileLength`
- **CPU:** O(file-size) — single-pass binary decode; no recursion

#### Explicit non-guarantees
- Does NOT validate the semantic correctness of terminfo entries — only parses the binary format
- Does NOT verify that string capabilities produce valid ANSI escape sequences
- Does NOT limit the number of names per terminfo entry

#### Integration examples

```go
// CORRECT: Load terminfo by name (name comes from os.Getenv("TERM"))
ti, err := terminfo.Load(os.Getenv("TERM"))
if err != nil {
    // handle: ErrEmptyTermName, ErrDatabaseDirectoryNotFound, or parse errors
}

// CORRECT: Use parametrized string cap with formatted args
s := ti.Printf(terminfo.CursorAddress, row, col)

// INCORRECT: Using untrusted input as the capability index without bounds check
// The Printf method does not bounds-check `i` against the Strings map.
s := ti.Printf(userSuppliedCapIndex, args...) // may panic if cap index is absent
```

### Assessment: **Safe to use.**

---

## 2. `github.com/charmbracelet/x/ansi`

**Path:** `vendor/github.com/charmbracelet/x/ansi/`
**Files:** ~50 `.go` files + `parser/` sub-package (~4 files) + `kitty/` sub-package (~5 files)

### Summary
DEC ANSI-compatible escape sequence parser and writer. Covers: CSI/OSC/DCS/APC sequences, SGR styling, cursor control, screen modes, Kitty graphics protocol, clipboard, hyperlinks, mouse events, terminal notifications, and width measurement.

### Security Findings

**No critical or high findings.**

| Check | Result |
|-------|--------|
| Network I/O | None |
| Shell execution | None |
| Hardcoded secrets | None |
| Unsafe usage | `unsafe.Slice` and `unsafe.Pointer` used internally in `Parser.Params()` and `Parser.Rune()` for zero-copy access to parser buffer. These operate exclusively on parser-owned arrays and never on caller-provided memory. The `params` slice is sized by `SetParamsSize()` (default 32 entries) and `paramsLen` is bounded by that size in all state machine transitions. |
| `os.Open` / `os.CreateTemp` | Used in `kitty/writer.go` for the Kitty graphics protocol's File and TempFile transmission modes. `os.Open(o.File)` reads a caller-supplied file path — the caller controls this, not an attacker. `os.CreateTemp(GraphicsTempDir, GraphicsTempPattern)` uses configurable but developer-controlled defaults (empty dir → os.TempDir). |
| Binary parsing | The state-machine parser (`parser/transition_table.go`) is table-driven with no recursion. Input bytes are consumed one at a time. Buffer sizes are capped at `SetParamsSize` and `SetDataSize` (default 64KB). |
| Logging | None |

### Integration Contract

#### Input bounds
- **Accepted:** `io.Writer` for output; raw byte sequences for parsing; `image.Image` for Kitty graphics encoding
- **Rejected:** Malformed escape sequences are handled gracefully (state machine transitions to ground state on invalid bytes)
- **Max safe size:** Data buffer capped at configured `SetDataSize` (default 64KB); params buffer capped at `SetParamsSize` (default 32)

#### Output shape
- **Return type:** ANSI escape sequence strings / writes to `io.Writer`; parsed parameters as `Params` (slice of `Param`); raw data as `[]byte`
- **Nil guarantees:** `NewParser()` never returns nil. String generators allocate and return valid escape sequences.

#### Side effects
- **I/O:** Writes to the provided `io.Writer` only. Kitty graphics File mode reads from a caller-specified file path. TempFile mode writes to `os.CreateTemp` (defaults to `os.TempDir`).
- **Allocations:** Per-sequence string allocations in generators; buffer growth in parser (capped)
- **Global state:** None (except `parserPool` sync.Pool for parser reuse)
- **Goroutine safety:** Individual `Parser` instances are NOT goroutine-safe (stateful). Generator functions (producing strings) are safe for concurrent use.

#### Error modes
- **Returned errors:** Kitty graphics encoding returns wrapped errors for file I/O failures; parse errors are communicated via the Handler interface, not returned
- **Panics:** None in normal operation. `Params()` panics if called when `paramsLen` is 0 (internal invariant).

#### Resource bounds
- **Memory:** Bounded by configured buffer sizes (default 64KB data + 32 params)
- **CPU:** O(input-length) — single-pass state machine, no backtracking

#### Explicit non-guarantees
- Does NOT validate that generated escape sequences are safe for any particular terminal
- Does NOT sanitize or escape content written via OSC sequences (e.g., window title, clipboard)
- The Kitty graphics `File` mode reads whatever path the caller provides — no path traversal prevention at this layer
- Does NOT limit the total output size of string generators

#### Integration examples

```go
// CORRECT: Parse ANSI input with bounded buffers
p := ansi.NewParser()
p.SetParamsSize(32)
p.SetDataSize(64 * 1024) // 64KB
p.SetHandler(myHandler)

// CORRECT: Write ANSI escape to known writer
ansi.SetCursorPosition(w, row, col)

// INCORRECT: Passing unsanitized user input as OSC content
// OSC sequences can set terminal title, clipboard, etc.
ansi.SetWindowTitle(w, userSuppliedTitle) // Okay if user controls their own terminal
```

### Assessment: **Safe to use.**

---

## 3. `github.com/charmbracelet/ultraviolet` (uv)

**Path:** `vendor/github.com/charmbracelet/ultraviolet/`
**Files:** 52 `.go` files

### Summary
Full terminal UI framework providing: raw terminal I/O (`/dev/tty`, `CONIN$`/`CONOUT$`), ANSI rendering, input event parsing, signal handling, screen buffer management, mouse support, clipboard, and poll-based event loop.

### Security Findings

**No critical or high findings.**

| Check | Result |
|-------|--------|
| Network I/O | None |
| Shell execution | None |
| Hardcoded secrets | None |
| `os.OpenFile("/dev/tty", ...)` | Opens `/dev/tty` for read-write — standard for raw terminal access. Path is hardcoded, not user-controlled. |
| `syscall.Kill(0, SIGTSTP)` | Suspends the process group — standard terminal job control. No attacker-controlled signal target. |
| `signal.Notify(c, SIGWINCH)` | Signal notification for window resize — standard. |
| `os.OpenFile("CONIN$"/"CONOUT$")` | Windows console handles — hardcoded paths, standard TUI pattern. |
| `os.OpenFile(debugFile, ...)` | Controlled by `UV_DEBUG` environment variable — only enables debug logging if explicitly set. Writes append-only. Not a secret leak vector since the env var is developer-controlled. |
| `syscall` usage (Windows) | `syscall.Syscall(procFlushConsoleInputBuffer.Addr(), ...)` in cancelreader — calls `FlushConsoleInputBuffer` from kernel32.dll. Standard Windows console API. |
| Poll mechanisms | Platform-specific poll using `unix.Select`, `windows.WaitForMultipleObjects`, or `poll(2)` — all are local I/O multiplexing with no attack surface. |

### Integration Contract

#### Input bounds
- **Accepted:** Terminal input (bytes from `/dev/tty` / `CONIN$`); ANSI escape sequences for rendering; `image/color.Color` for styling
- **Rejected:** N/A — terminal input is always untrusted byte streams; the framework parses and dispatches them as events
- **Max safe size:** Render buffer dimensions bounded by terminal window size (queried via `TIOCGWINSZ` / `GetConsoleScreenBufferInfo`)

#### Output shape
- **Return type:** Rendered content written to `io.Writer` (terminal output); parsed input events dispatched as structured Go values
- **Nil guarantees:** `NewTerminalScreen(w, env)` never returns nil

#### Side effects
- **I/O:** Reads from `/dev/tty` or Windows console handles; writes to the provided `io.Writer`. Optionally writes debug logs to `UV_DEBUG` file.
- **Allocations:** Render buffer allocated proportional to terminal dimensions; event channel buffering
- **Global state:** Signal handlers registered via `signal.Notify`; Windows console mode modified and restored on close
- **Goroutine safety:** Screen methods are designed for single-goroutine use (standard Bubble Tea pattern). Poll goroutines are internal.

#### Error modes
- **Returned errors:** TTY open failures, console mode failures, write failures — all returned as errors
- **Panics:** None identified in normal code paths
- **Timeouts:** Poll implementations use configurable timeouts; cancelreader uses `INFINITE` wait on Windows (cancelable via event)

#### Resource bounds
- **Memory:** Render buffer = O(width × height) cells; event queues bounded
- **CPU:** Per-frame rendering cost; poll waits block on I/O (efficient)

#### Explicit non-guarantees
- Does NOT sanitize terminal input — all bytes from the terminal are passed through as events
- Does NOT validate that ANSI output sequences are safe for the terminal
- The `UV_DEBUG` file grows unboundedly if set — caller should use log rotation if enabling in production
- Windows console mode changes are best-effort on restoration; process crash may leave console in raw mode

#### Integration examples

```go
// CORRECT: Create screen with bounded environment
screen := uv.NewTerminalScreen(os.Stdout, uv.Environ(os.Environ()))
screen.Resize(width, height)

// CORRECT: Do NOT set UV_DEBUG in production
// It writes all terminal I/O to a file — could leak sensitive display content.

// INCORRECT: Sharing a screen across goroutines without synchronization
go screen.Resize(w, h) // race with render goroutine
```

### Assessment: **Safe to use.** Standard terminal I/O patterns; no elevated risk.

---

## 4. `github.com/lucasb-eyer/go-colorful`

**Path:** `vendor/github.com/lucasb-eyer/go-colorful/`
**Files:** 10 `.go` files

### Summary
Pure-math color manipulation library. Provides: sRGB ↔ HSL/HSV/CIELAB/CIELUV/XYZ conversions, color distance (Delta E), palette generation, HSLuv, linear/wide-gamut RGB. Zero I/O, zero side effects.

### Security Findings

**No findings.** This is a pure-function math library:

| Check | Result |
|-------|--------|
| Network I/O | None |
| Shell execution | None |
| Hardcoded secrets | None |
| Unsafe / syscall | None |
| File I/O | None |
| Logging | None |
| Global mutable state | None (only const reference white points) |
| Panic risk | `math.Pow`, `math.Sqrt`, `math.Log` on float64 — standard floating-point behavior; no panic on edge values (NaN/Inf propagate via IEEE 754). `strconv.ParseFloat` in hex parsing returns errors, never panics. |

### Integration Contract

#### Input bounds
- **Accepted:** `float64` RGB components (nominally 0–1 but not clamped); `image/color.Color` interface values; hex strings (`#RGB`, `#RRGGBB`, etc.)
- **Rejected:** Invalid hex strings return `Color{}` + `error`; `MakeColor` returns `false` for fully transparent colors
- **Max safe size:** N/A — scalars only

#### Output shape
- **Return type:** `Color` struct (R, G, B float64), or `[]Color` for palette generators
- **Nil guarantees:** Methods never return pointers; no nil risk
- **Encoding guarantees:** RGB components may exceed [0, 1] for wide-gamut colors (this is intentional)

#### Side effects
- **I/O:** None
- **Allocations:** Per-color allocations in conversions; palette generators allocate slices
- **Global state:** None (reference white points are `const`)
- **Goroutine safety:** All methods are pure functions; safe for concurrent use

#### Error modes
- **Returned errors:** `Hex` returns `errInvalidHexColor` for malformed input
- **Panics:** None

#### Resource bounds
- **Memory:** O(1) per color; O(n) for n-color palettes
- **CPU:** O(1) per conversion (trigonometry + floating-point); palette generation uses k-means clustering (iterative)

#### Explicit non-guarantees
- Does NOT clamp RGB values to [0, 1] — callers must clamp before converting to 8-bit sRGB
- Does NOT guarantee perceptual uniformity of distance metrics across color spaces (Delta E is an approximation)
- HSLuv implementation may diverge slightly from the reference C implementation at extreme values

#### Integration examples

```go
// CORRECT: Parse a hex color with error handling
c, err := colorful.Hex("#ff0044")
if err != nil {
    // handle invalid hex
}

// INCORRECT: Using .RGB255() without clamping
c := colorful.Color{R: 2.5, G: -0.1, B: 0.5}
r, g, b := c.RGB255() // r=255 (clamped), g=0 (clamped) — may be surprising
```

### Assessment: **Safe to use.** Pure math; zero attack surface.

---

## 5. `github.com/muesli/cancelreader`

**Path:** `vendor/github.com/muesli/cancelreader/`
**Files:** 7 `.go` files

### Summary
Provides a cancelable `io.Reader` wrapper. On Unix, uses `select(2)` to multiplex between the file descriptor and a pipe/signal for cancellation. On Windows, uses `WaitForMultipleObjects` with overlapped I/O on `CONIN$`. Falls back to a no-op cancel wrapper for non-file readers.

### Security Findings

**No critical or high findings.**

| Check | Result |
|-------|--------|
| Network I/O | None |
| Shell execution | None |
| Hardcoded secrets | None |
| `syscall.Syscall(procFlushConsoleInputBuffer.Addr(), ...)` | Calls `FlushConsoleInputBuffer` from kernel32.dll on Windows — standard Win32 console API. The function pointer is resolved from `kernel32.dll` via `windows.NewLazySystemDLL`, which is a system DLL — not attacker-controllable. |
| `windows.CreateFile("CONIN$", ...)` | Opens the Windows console input handle — hardcoded path, standard pattern. |
| `windows.SetConsoleMode(input, newMode)` | Modifies console mode (disables echo, line input, mouse input, window input, processed input; enables virtual terminal input). Original mode is saved and restored on `Close()`. |
| File descriptor leak | `Close()` closes the cancel event handle and the `CONIN$` handle. The `resetConsole` callback restores the original console mode. |

### Integration Contract

#### Input bounds
- **Accepted:** Any `io.Reader`; special-cases `File` interface implementers whose `Fd()` matches `os.Stdin.Fd()`
- **Rejected:** N/A
- **Max safe size:** Passthrough to underlying reader

#### Output shape
- **Return type:** `CancelReader` interface (`io.ReadCloser` + `Cancel() bool`)
- **Nil guarantees:** `NewReader` returns `nil, error` on failure

#### Side effects
- **I/O:** Reads from the underlying reader; on Windows, opens `CONIN$` in overlapped mode
- **Allocations:** Creates Windows event handles, overlapped structs
- **Global state:** None
- **Goroutine safety:** `Cancel()` is safe to call from any goroutine. `Read()` should be called from a single goroutine.

#### Error modes
- **Returned errors:** `ErrCanceled` when read is canceled; `fmt.Errorf` wraps for system call failures
- **Panics:** None
- **Timeouts:** Windows `Cancel()` has a 100ms timeout for detecting uncancelable reads

#### Resource bounds
- **Memory:** O(1) — fixed-size structs and handles
- **CPU:** Blocks on I/O (efficient)

#### Explicit non-guarantees
- Does NOT guarantee cancellation on Windows when `ENABLE_VIRTUAL_TERMINAL_INPUT` mode is active (documented limitation — `Cancel()` returns `false` in this case)
- The fallback reader never truly cancels an in-flight read (only prevents future reads)
- Windows console mode restoration is best-effort; if the process crashes, the console may be left in raw mode

#### Integration examples

```go
// CORRECT: Cancel a blocking read from a goroutine
cr, err := cancelreader.NewReader(os.Stdin)
go func() {
    time.Sleep(5 * time.Second)
    cr.Cancel()
}()
cr.Read(buf) // returns ErrCanceled after cancel

// CORRECT: Always close to restore console mode
defer cr.Close()
```

### Assessment: **Safe to use.** Standard OS I/O patterns; platform-specific code is well-scoped.

---

## 6. `charm.land/bubbles/v2`

**Path:** `vendor/charm.land/bubbles/v2/`
**Files:** 9 `.go` files (key.go, cursor.go, textarea.go, viewport.go, spinner.go + internal helpers)

### Summary
TUI component library for Bubble Tea. Provides: `key.Binding` (keymap definitions), `cursor.Cursor` (blinking cursor model), `viewport.Model` (scrollable viewport with highlighting), `textarea.Model` (multi-line text input), `spinner.Model` (animated spinner).

### Security Findings

**No findings.**

| Check | Result |
|-------|--------|
| Network I/O | None |
| Shell execution | None |
| Hardcoded secrets | None |
| Unsafe / syscall | None |
| File I/O | None |
| Logging | None |
| Input handling | `key.Binding` uses string comparison (not eval/exec). `textarea` processes rune input. All input is handled as terminal events, not system commands. |
| Global mutable state | None |

### Integration Contract

#### Input bounds
- **Accepted:** Terminal key events (`tea.KeyPressMsg`), rune input, style definitions, dimension constraints
- **Rejected:** N/A — components handle unexpected input gracefully (no-ops for unknown keys)
- **Max safe size:** Viewport content is stored as strings in memory — bounded by Go string max length; textarea has no explicit input length limit (caller should enforce if needed)

#### Output shape
- **Return type:** `tea.Model` interface (Update/View); `string` for rendered output
- **Nil guarantees:** Constructor functions never return nil models

#### Side effects
- **I/O:** None (pure model updates and view rendering)
- **Allocations:** String operations on every update/view cycle
- **Global state:** None
- **Goroutine safety:** All models are designed for single-goroutine use (standard Bubble Tea ELM architecture)

#### Error modes
- **Returned errors:** None (models return `tea.Cmd` for side effects; errors are surfaced through the Bubble Tea runtime)
- **Panics:** None in normal operation

#### Resource bounds
- **Memory:** O(content-size) — viewport and textarea store full content in memory
- **CPU:** Per-frame rendering cost; textarea word-wrap is O(line-count)

#### Explicit non-guarantees
- Does NOT enforce input length limits on textarea
- Does NOT sanitize textarea/viewport content for terminal escape sequences — displaying raw ANSI in content may produce unexpected terminal behavior
- Viewport highlight uses substring matching — may have O(n×m) worst-case for long content with many matches

#### Integration examples

```go
// CORRECT: Define a keymap
var keys = struct {
    Up    key.Binding
    Down  key.Binding
}{
    Up:   key.NewBinding(key.WithKeys("k", "up")),
    Down: key.NewBinding(key.WithKeys("j", "down")),
}

// CORRECT: Match key events
if key.Matches(msg, keys.Up) { ... }

// NOTE: No security-relevant misuse patterns for these components.
```

### Assessment: **Safe to use.** Pure model/view components with no I/O or side effects.

---

## Overall Verdict

**All six packages are safe to use.** They implement standard terminal I/O and color math — no network, no shell execution, no secrets, no deserialization of untrusted data. The attack surface is limited to:

1. **terminfo binary parsing** — well-bounded with explicit size/Bounds checks
2. **Terminal device I/O** — standard paths (`/dev/tty`, `CONIN$`) with platform-appropriate APIs
3. **Environment variable consumption** (`$TERM`, `$TERMINFO`, `UV_DEBUG`) — server/developer-controlled, not attacker-controlled

| Package | Risk Level | Attack Surface |
|---------|-----------|----------------|
| `xo/terminfo` | Clean | Local file read (terminfo DB), bounded binary parse |
| `charmbracelet/x/ansi` | Clean | State-machine parser (bounded), writer to `io.Writer` |
| `charmbracelet/ultraviolet` | Clean | Local TTY I/O, signal handling, Windows console APIs |
| `lucasb-eyer/go-colorful` | Clean | None — pure math |
| `muesli/cancelreader` | Clean | Local TTY I/O, `select(2)` / `WaitForMultipleObjects` |
| `charm.land/bubbles/v2` | Clean | None — pure model/view components |
