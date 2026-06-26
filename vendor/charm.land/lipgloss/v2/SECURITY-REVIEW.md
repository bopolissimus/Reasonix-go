# Security Review: charm.land/lipgloss/v2

**Date:** 2026-06-27  
**Scope:** `vendor/charm.land/lipgloss/v2` at v2.0.4 + transitive dependencies  
**Reviewer:** deepseek-v4-pro sub-agent  
**Methodology:** Static source analysis — all 24 `.go` files (4,200+ LOC) plus transitive dependency audit

---

## Summary

- **Target Type**: Go library — terminal styling (ANSI escape sequence emitter)
- **Modes Used**: A (Read-only static analysis)
- **Findings**: 0 Critical, 0 High, 2 Medium, 1 Low
- **Risk Level**: Low
- **Rule of Two Violation**: No — lipgloss is a pure string→string transformation; it has no filesystem write access, no network access, and no credential context
- **Confidence**: High
- **Dependencies reviewed**: 5 direct (`x/ansi v0.11.7`, `ultraviolet` dev, `colorprofile v0.4.3`, `x/term v0.2.2`, `lucasb-eyer/go-colorful v1.4.0`) + 7 transitive — all previously reviewed
- **CVEs checked**: 0 known CVEs against charm.land/lipgloss/v2 or any charmbracelet TUI library
- **Release notes analyzed**: lipgloss/v2 v2.0.4 is the tagged release; ultraviolet is a dev snapshot from 2026-06-01
- **Cross-dependency chains identified**: 1 (ANSI passthrough chain — see SEC-001)

---

## Findings

### [SEC-001] Unsanitized ANSI Escape Passthrough (Medium)

- **Category**: Improper Output Handling
- **OWASP Reference**: LLM05
- **Location**: `style.go:194-290` (`Style.Render()`), `writer.go:24-115` (all Print/Sprint variants)
- **Confidence**: High
- **Issue**: `Style.Render()` applies ANSI SGR styling (bold, colors, borders) to arbitrary input strings but does **not** strip or sanitize existing ANSI escape sequences in the input. If the input string already contains escape sequences (CSI, OSC, DCS), they pass through unchanged to the terminal output.

- **Attack Path**:
  1. Attacker controls content that is eventually styled via `Style.Render(str)`. In Reasonix, this is **model output** rendered in the chat TUI.
  2. Attacker (via prompt injection) causes the model to include ANSI escape sequences in its response — e.g., `\x1b]0;MALICIOUS\x07` (terminal title), `\x1b]8;;https://evil.com\x07` (OSC 8 hyperlink), or `\x1b]52;c;<base64>\x07` (OSC 52 clipboard write).
  3. Reasonix's chat TUI passes the model output to `lipgloss.Style.Render()`, which adds its own SGR styling but leaves existing escapes intact.
  4. The combined string is written to stdout (the user's terminal), where the injected sequences execute.

- **Attacker-Controlled**: Yes — model output is attacker-influenced via prompt injection, and the model may inadvertently echo escape sequences present in training data or user-supplied content.

- **Guard/Mitigation Present**: None in lipgloss itself. The library's contract is "I style your text" — not "I sanitize your text." The consumer (Reasonix chat TUI) should strip ANSI escapes from untrusted strings before passing them to lipgloss. The `ansi.Strip()` function from `x/ansi` is available for this purpose.

- **Residual Exploitability**: Real, but bounded:
  - **Terminal title injection** (OSC 0/2): Low impact — cosmetic, social engineering at worst.
  - **Hyperlink injection** (OSC 8): Medium impact — a user clicking a link they think is benign could visit a malicious URL. Terminal emulators typically show the URL target on hover/menu.
  - **Clipboard injection** (OSC 52): Medium impact — reads/writes the system clipboard. Modern terminal emulators (iTerm2, Kitty, WezTerm) typically require explicit user confirmation or configuration to enable OSC 52. Windows Terminal disables it by default.
  - **Cursor movement / screen clearing** (CSI sequences): Low impact — cosmetic disruption; the terminal state resets on next prompt.

- **Evidence**:
  ```go
  // style.go:239 — input string is used directly without ANSI stripping
  func (s Style) Render(strs ...string) string {
      // ...
      str = joinString(strs...)  // no ansi.Strip() here
      // ... SGR wrapping applied around str ...
      b.WriteString(te.Styled(line))  // line may contain raw escape sequences
  ```

- **Remediation**: In Reasonix's chat TUI, call `ansi.Strip(untrustedString)` on model output before passing it to lipgloss for styling. This preserves the library's intended behavior (styling plain text) while preventing escape sequence injection. Example:
  ```go
  safeContent := ansi.Strip(modelOutput)
  rendered := myStyle.Render(safeContent)
  ```
  If preserving intentional styling from model output is desired, use a whitelist approach (allow only SGR sequences, strip everything else).

---

### [SEC-002] Package-Level Writer Reads `os.Environ()` at `init` Time (Medium)

- **Category**: Sensitive Information Disclosure (environment visibility)
- **OWASP Reference**: LLM02
- **Location**: `writer.go:12`
- **Confidence**: Medium
- **Issue**: The package-level `Writer` variable is initialized at import time:
  ```go
  var Writer = colorprofile.NewWriter(os.Stdout, os.Environ())
  ```
  This calls `os.Environ()` during package initialization, which reads **all** environment variables into memory for color profile detection. The environment data is not exfiltrated (no network), but it is loaded into a package-level variable accessible to any code that imports lipgloss.

- **Attack Path**:
  1. Attacker sets environment variables with sensitive values (`AWS_SECRET_ACCESS_KEY`, `GITHUB_TOKEN`, etc.) before Reasonix is launched.
  2. Reasonix imports lipgloss → `os.Environ()` is called → all env vars are read into the `colorprofile.Profile` detection path.
  3. No exfiltration occurs (the data stays in-process), but the full environment is now resident in Go heap memory — visible to any package with access to `lipgloss.Writer`.

- **Attacker-Controlled**: Partial — attacker controls environment variables on the host, but not lipgloss's internal behavior.

- **Guard/Mitigation Present**: `os.Environ()` is the standard Go API for environment access. The `colorprofile` package only uses it to detect terminal color capabilities (checking `COLORTERM`, `TERM`, `NO_COLOR`, etc.). It does not log or transmit env data.

- **Residual Exploitability**: None in the current code. This is a defense-in-depth observation: if `colorprofile.Writer` ever acquired logging or telemetry behavior, this init-time env read would become a leak vector. The risk is bounded because `os.Environ()` is called by nearly every Go program that launches subprocesses anyway.

- **Remediation**: Accept the risk. If defense-in-depth is desired, pass only the specific env vars needed for color detection (`os.Environ()` → filter to `COLORTERM`, `TERM`, `NO_COLOR`, `TERM_PROGRAM`, `TMUX`, etc.) but this is a low-priority hardening.

---

## Low / Informational

| # | Finding | Location | Detail |
|---|---------|----------|--------|
| L1 | `BackgroundColor()` opens `CONIN$`/`CONOUT$` on Windows | `query.go:40-47` | Opens the Windows console via special filenames when stdin/stdout are redirected. This is by design — the documented way to access the console on Windows when handles are not available. `os.OpenFile("CONIN$", os.O_RDWR, 0o644)` is the standard pattern. No path traversal possible (these are reserved device names, not filesystem paths). |
| L2 | `transform` callback execution | `set.go:347` (`Style.Transform`) | `Style.Transform(fn func(string) string)` stores an arbitrary callback that executes during `Render()`. If a caller accidentally passes an attacker-controlled function, this is arbitrary code execution. In practice, transforms are developer-written string manipulation functions (e.g., `strings.ToUpper`). Not attacker-controlled in normal usage. |
| L3 | `Wrap()` allocates `ansi.GetParser()` from pool | `wrap.go:32` | Uses `ansi.GetParser()`/`ansi.PutParser()` — parser pool return is correctly handled in `WrapWriter.Close()`. No use-after-free risk. |

---

## Dependency Audit Summary

| Dependency | Version | Risk | Notes |
|---|---|---|---|
| `charmbracelet/x/ansi` | v0.11.7 | Low | Env var read at init (`method.go`), Kitty graphics file ops (caller-controlled). Previously reviewed — no new findings. |
| `charmbracelet/ultraviolet` | dev (2026-06-01) | Low | Debug file via `UV_DEBUG` env var, `/dev/tty` open on Unix, `CONIN$`/`CONOUT$` on Windows. Previously reviewed — no new findings. |
| `charmbracelet/colorprofile` | v0.4.3 | Low | `exec.Command("tmux", "info")` when `TMUX` set. Hardcoded command, no user-controlled args. Previously reviewed — no new findings. |
| `charmbracelet/x/term` | v0.2.2 | None | Terminal state management (tcgetattr/tcsetattr, Windows console mode). Pure syscall wrappers. |
| `charmbracelet/x/termios` | v0.1.1 | None | Terminal I/O speed constants and syscall definitions. No executing code. |
| `charmbracelet/x/windows` | v0.2.2 | None | Windows console API wrappers (`kernel32.dll`, `GetConsoleMode`, etc.). Standard pattern. |
| `lucasb-eyer/go-colorful` | v1.4.0 | None | Pure math — color space conversions (RGB↔HSL↔HSV). No I/O, no syscalls. |
| `clipperhouse/displaywidth` | v0.11.0 | None | Unicode display width calculation. Pure computation. |
| `clipperhouse/uax29/v2` | v2.7.0 | None | Unicode grapheme cluster segmentation. Pure computation. |
| `rivo/uniseg` | v0.4.7 | None | Unicode grapheme/word/sentence segmentation. Pure computation. |
| `mattn/go-runewidth` | v0.0.24 | Low | Reads `LC_ALL`/`LC_CTYPE`/`RUNEWIDTH_EASTASIAN` env vars. Standard Go locale detection. |

None of the transitive dependencies introduce new attack surface beyond what was already documented in the batch reviews.

---

## Reachability Analysis (Reasonix Context)

Lipgloss is used in `internal/cli/`:
- `chat_tui.go` (19 call sites) — chat TUI rendering
- `theme.go` (16 call sites) — theme/style definitions
- `transcript.go` (6 call sites) — transcript rendering
- `chooser.go`, `complete.go` — UI components

The primary attack surface is **model output rendered through lipgloss styles** (SEC-001). The chat TUI constructs styled strings from model responses and writes them to the terminal. If the model produces ANSI escape sequences in its output, they will reach the user's terminal.

---

## Integration Contract

### Input bounds
- **Accepted**: `string` of arbitrary length. Input is the "content" text to be styled — not the style directives themselves.
- **Rejected**: Nothing is rejected. Empty strings produce empty styled output.
- **Max safe size**: Unbounded. `Render()` allocates proportional to input size (`strings.Builder`, `strings.Split`, `strings.Repeat`). For untrusted input, caller should apply a size limit before passing to `Render()`. A 100MB input would cause OOM.

### Output shape
- **Return type**: `string` containing ANSI-styled text suitable for terminal output.
- **Encoding guarantees**: Output is valid UTF-8 (input is UTF-8 Go strings). Contains ANSI escape sequences (`\x1b[...m` SGR, `\x1b]8;;...\x07` hyperlinks, `\x1b[...` CSI for cursor movement in some operations).
- **Nil guarantees**: `Render("")` returns `""` (empty string, not nil). All public functions return Go `string` values.
- **Style reset guarantee**: Styled output is bounded by `ansi.ResetStyle` / `ansi.ResetHyperlink()` in appropriate places (WrapWriter.Close, border rendering, hyperlink rendering). Each styled segment is self-contained — no style bleed to subsequent output.
- **Newline handling**: `\r\n` is normalized to `\n`. In inline mode, all `\n` are stripped.

### Side effects
- **I/O**: None. `Render()` is a pure string→string transformation. The `Print`/`Println`/`Printf` functions write to `os.Stdout` (or a caller-provided `io.Writer`). `BackgroundColor()` reads from stdin and writes query sequences to stdout.
- **Allocations**: `Render()` allocates a `strings.Builder` and intermediate string slices proportional to input size and line count. `Wrap()` additionally allocates an `ansi.Parser` from a pool.
- **Global state**: `Writer` variable initializes at package import (reads `os.Environ()`). `EnableLegacyWindowsANSI()` modifies the Windows console mode of the passed file descriptor.
- **Goroutine safety**: All `Style` methods return new `Style` values (immutable pattern). The `Style` struct is safe for concurrent use. `WrapWriter` is NOT safe for concurrent use (single `ansi.Parser` instance, single `io.Writer`).

### Error modes
- **Returned errors**: `BackgroundColor()` returns an error if the terminal query times out or the input is not a TTY. `HasDarkBackground()` swallows errors and defaults to `true`.
- **Panics**: Type assertions in `Style.set()` can panic if called with wrong types — but these are all internal methods, not public API. Public API methods use typed parameters (`int`, `bool`, `color.Color`). No known panic paths for valid public API calls.
- **Timeouts**: `queryTerminal()` has a 2-second timeout. `BackgroundColor()` blocks for up to 2 seconds waiting for terminal response.

### Resource bounds
- **Memory**: O(input length) — one `strings.Builder`, intermediate splits/wraps proportional to line count and line length. Border rendering adds O(width × lines) for border blend gradients.
- **CPU**: O(input length) — single-pass string splitting, tab conversion, styling. `Wrap()` adds ANSI parsing overhead (byte-at-a-time state machine). Border blend rendering adds O(border cells) gradient slice operations.
- **Stack**: No recursion. All functions are iterative or use bounded loops.

### Explicit non-guarantees
- **Does NOT validate that the input string is free of ANSI escape sequences.** Input strings may contain any byte sequence, including escape sequences that will pass through to the terminal output unchanged. Callers rendering untrusted content MUST strip ANSI escapes before passing to `Render()`.
- **Does NOT limit output size.** A large input produces a proportionally large output. Caller must apply size limits for untrusted input.
- **Does NOT guarantee thread safety for `WrapWriter`.** Each `WrapWriter` has a single `ansi.Parser` and single output writer — concurrent `Write()` calls will interleave ANSI state and corrupt output.
- **Does NOT guarantee stable output across versions.** ANSI escape sequences emitted may change between versions (different escape codes for the same visual effect).
- **`BackgroundColor()` blocks the calling goroutine for up to 2 seconds** and requires the terminal to be in raw mode. Caller must handle timeout.

### Integration examples

```go
// CORRECT: strip ANSI from untrusted input before styling
import "github.com/charmbracelet/x/ansi"

safeContent := ansi.Strip(untrustedModelOutput)
styled := lipgloss.NewStyle().
    Foreground(lipgloss.Color("#6a00ff")).
    Render(safeContent)
// safeContent contains no escape sequences — only plain text
```

```go
// CORRECT: wrap long model output with size limit
import "github.com/charmbracelet/x/ansi"

const maxOutput = 100_000 // 100KB
if len(untrustedModelOutput) > maxOutput {
    untrustedModelOutput = ansi.Truncate(untrustedModelOutput, maxOutput, "…")
}
styled := lipgloss.NewStyle().MaxWidth(80).Render(untrustedModelOutput)
```

```go
// INCORRECT: no ANSI stripping — model output escape sequences reach terminal
styled := lipgloss.NewStyle().
    Bold(true).
    Render(untrustedModelOutput)
// If modelOutput contains "\x1b]8;;https://evil.com\x07Click here\x1b]8;;\x07",
// the terminal renders a clickable link to evil.com
```

---

## Assessment

**Safe to use.** Lipgloss v2.0.4 is a terminal styling library with a tightly scoped attack surface: it transforms strings into strings via ANSI escape sequences. It has no network access, no filesystem writes, no subprocess execution, and no deserialization of untrusted data. Its only external interaction is reading environment variables (for color profile detection) and terminal queries (OSC 11 for background color).

The only meaningful finding is SEC-001 (ANSI escape passthrough), which is inherent to any terminal styling library — the library's purpose is to emit ANSI sequences, and it cannot distinguish between its own sequences and sequences already present in the input. The consumer (Reasonix chat TUI) is responsible for sanitizing model output before styling.

No known CVEs exist against any charmbracelet TUI library in the dependency chain.
