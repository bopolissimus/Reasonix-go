# Security Review: github.com/atotto/clipboard v0.1.4

- **Target Type**: Vendored dependency (Go library)
- **Modes Used**: A (read, grep, glob — static source analysis)
- **Findings**: 4 (0 Critical, 0 High, 2 Medium, 2 Low)
- **Risk Level**: Low
- **Rule of Two Violation**: No — clipboard is inherently an OS-mediated I/O surface; the
  library has no network access, credential storage, or state-changing sinks beyond
  the clipboard itself.
- **Confidence**: High
- **Dependencies reviewed**: 1 direct (stdlib only — no external packages)
- **CVEs checked**: GitHub Advisory DB: 0; OSV: 0
- **Release notes analyzed**: v0.1.3 → v0.1.4 diff inspected — Plan 9 support, WSL
  `clip.exe`/`powershell.exe` support, CI changes only; no security fixes.
- **Cross-dependency chains identified**: 0

---

## Scope

The library provides two public functions:

| Function | Signature | Behaviour |
|----------|-----------|-----------|
| `ReadAll()` | `() (string, error)` | Reads the system clipboard as a UTF-8 string |
| `WriteAll(text)` | `(string) error` | Writes a UTF-8 string to the system clipboard |

Platform-specific backends:

| Platform | Read | Write |
|----------|------|-------|
| **macOS** | `pbcopy` / `pbpaste` subprocess | same |
| **Linux/X11** | `xclip` or `xsel` subprocess (auto-detected in `init()`) | same |
| **Linux/Wayland** | `wl-copy` / `wl-paste` subprocess | same |
| **Windows** | Win32 `GetClipboardData` + `GlobalLock` → `UTF16ToString` | `SetClipboardData` via `GlobalAlloc(GMEM_MOVEABLE)` |
| **WSL (Linux)** | `powershell.exe Get-Clipboard` + `clip.exe` subprocess | same |
| **Plan 9** | `/dev/snarf` file read/write | same |
| **Termux (Android)** | `termux-clipboard-get` / `termux-clipboard-set` subprocess | same |

**Reasonix usage**: `chat_tui.go` calls `ReadAll()` for paste; `transcript.go` calls
`WriteAll()` for copy. Both are user-initiated actions in the desktop TUI.

---

## Findings

### [SEC-001] Unsafe fixed-size pointer cast — potential out-of-bounds read (Medium)

- **Category**: Memory safety
- **Location**: `clipboard_windows.go:85`
- **Confidence**: High
- **Issue**: The `GlobalLock` return value is cast to a fixed-size `(*[1 << 20]uint16)`
  array (1,048,576 `uint16` values = 2 MiB of raw memory). `UTF16ToString` is then
  called on the full slice. If clipboard data exceeds this size, `UTF16ToString`
  walks beyond the allocated Win32 heap block — this is undefined behaviour and
  can cause a segmentation fault or silent data corruption.
- **Attack Path**:
  1. A local process (or another user on a shared Windows system) places
     > 2 MiB of UTF-16 text onto the clipboard.
  2. Reasonix user triggers paste (Ctrl+V in the TUI).
  3. `readAll()` calls `GlobalLock`, casts to the fixed array, and calls
     `UTF16ToString` on the unbounded memory.
  4. If the null terminator lies beyond the 1 Mi boundary, Go's unsafe pointer
     dereference reads past the allocated block.
- **Guard/Mitigation Present**: `UTF16ToString` stops at the first `0x0000` null
  terminator. If the clipboard data is ≤ 1 Mi and properly terminated, the read
  stops in-bounds. Typical clipboards do not contain megabytes of text.
- **Residual Exploitability**: Low — requires either a deliberately malicious
  clipboard payload without a null terminator, or clipboard text > ~2 MiB.
- **Evidence**:
  ```go
  // clipboard_windows.go:85
  text := syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(l))[:])
  ```
- **Remediation**: Use `kernel32.GlobalSize(h)` to obtain the actual allocation
  size, then cap the slice at `min(globalSize/sizeof(uint16), 1<<20)` — or,
  more robustly, walk the pointer with a sentinel loop reading one `uint16`
  at a time until null, bounded by the actual allocation size.

### [SEC-002] PATH-based command resolution — untrusted `$PATH` injection (Medium)

- **Category**: Command injection via PATH
- **Location**: `clipboard_darwin.go:18-21`, `clipboard_unix.go:72-106`
- **Confidence**: Medium
- **Issue**: On macOS, Linux, and Termux, clipboard commands (`pbcopy`, `pbpaste`,
  `xclip`, `xsel`, `wl-copy`, `wl-paste`, `termux-clipboard-get`,
  `termux-clipboard-set`) are resolved via `exec.Command(name)` which searches
  `$PATH`. If an attacker places a malicious binary with one of these names
  earlier in the `PATH` than the system binary, that binary executes with the
  Reasonix process's privileges whenever the user copies or pastes.
- **Attack Path**:
  1. Attacker gains the ability to write to a directory early in the user's
     `$PATH` (e.g. `~/.local/bin` via a compromised install script, or a
     writable `/tmp` directory on PATH).
  2. Attacker places a script named `xclip` (or `pbcopy`, etc.) that exfiltrates
     data or executes malicious code.
  3. User triggers paste → Reasonix calls `clipboard.ReadAll()` → `exec.Command("xclip", ...)`.
  4. The attacker's binary runs instead of the real `xclip`, with full access to
     the Reasonix process's environment, filesystem, and network.
- **Guard/Mitigation Present**: `exec.LookPath()` is used during `init()` to detect
  which tools are available, but the actual command execution still uses the
  plain command name (PATH resolution). The library does not pin absolute paths.
- **Residual Exploitability**: Moderate — requires PATH poisoning, which is a
  broader system compromise indicator, but the library provides no defense in depth.
- **Evidence**:
  ```go
  // clipboard_unix.go:72 — init() probes with LookPath, but execution uses bare name
  if _, err := exec.LookPath(xclip); err == nil {
      return // pasteCmdArgs/copyCmdArgs set to ["xclip", ...]
  }
  // clipboard_unix.go:107
  return exec.Command(pasteCmdArgs[0], pasteCmdArgs[1:]...)
  // clipboard_darwin.go:18-21 — same pattern
  func getPasteCommand() *exec.Cmd {
      return exec.Command(pasteCmdArgs)  // pasteCmdArgs = "pbpaste"
  }
  ```
- **Remediation**: Use `exec.LookPath()` to resolve the absolute path at init
  time, store the resolved path, and execute with the absolute path. Example:
  ```go
  path, err := exec.LookPath("xclip")
  if err == nil {
      pasteCmdArgs = []string{path, "-out", "-selection", "clipboard"}
  }
  ```
  If the upstream library is not maintained, Reasonix should consider this a
  known limitation and note that the attacker already has filesystem write on
  a PATH directory (itself a higher-severity compromise).

### [SEC-003] `trimDos` unconditional trailing-byte removal (Low)

- **Category**: Data corruption
- **Location**: `clipboard_unix.go:147-149`
- **Confidence**: High
- **Issue**: When running under WSL (detected by the presence of `clip.exe` and
  `powershell.exe`), the `trimDos` flag is set, and `readAll()` unconditionally
  strips the last two bytes from the clipboard result to remove the CR+LF
  (`\r\n`) that Windows clipboard tools append. If the clipboard text legitimately
  ends with `\r\n`, those characters are silently deleted.
- **Attack Path**:
  1. User copies text ending with `\r\n` (e.g. a code snippet with Windows line
     endings, or a string that happens to end with `\x0d\x0a`).
  2. Under WSL, `readAll()` strips the last two bytes regardless of whether they
     are actually `\r\n`.
  3. User pastes corrupted data without realizing it.
- **Guard/Mitigation Present**: None — the trim is unconditional.
- **Residual Exploitability**: N/A (data integrity, not exploitation).
- **Evidence**:
  ```go
  // clipboard_unix.go:147-149
  if trimDos && len(result) > 1 {
      result = result[:len(result)-2]
  }
  ```
- **Remediation**: Check that `result[len(result)-2:] == "\r\n"` before trimming.
  ```go
  if trimDos && len(result) >= 2 && result[len(result)-2:] == "\r\n" {
      result = result[:len(result)-2]
  }
  ```

### [SEC-004] Unbounded memory allocation on read (Linux/macOS) (Low)

- **Category**: Resource exhaustion
- **Location**: `clipboard_darwin.go:30-32`, `clipboard_unix.go:138`
- **Confidence**: Medium
- **Issue**: On macOS and Linux, `ReadAll()` calls `pasteCmd.Output()`, which
  reads the entire stdout of the paste command into a `[]byte` buffer with no
  size limit. A process that places multi-gigabyte data on the clipboard can
  cause Reasonix to allocate and crash (OOM).
- **Attack Path**:
  1. Malicious local process fills the clipboard with 4 GiB of text.
  2. User triggers paste → `readAll()` → `pasteCmd.Output()` allocates a 4 GiB
     buffer.
  3. Reasonix process is killed by the OOM killer (or panics on allocation
     failure).
- **Guard/Mitigation Present**: None. The paste command (`xclip -out`, `pbpaste`,
  `wl-paste`) has no inherent output limit.
- **Residual Exploitability**: Low — requires a local attacker with clipboard
  write access (which implies OS-level compromise or physical access). On macOS,
  `pbcopy`/`pbpaste` has practical limits. The Windows backend is bounded at
  ~2 MiB by the fixed array cast (SEC-001).
- **Evidence**:
  ```go
  // clipboard_darwin.go:30-31
  pasteCmd := getPasteCommand()
  out, err := pasteCmd.Output() // unbounded allocation
  ```
- **Remediation**: Wrap the read with `io.LimitReader` or use a streaming
  approach with a configurable cap (e.g. 16 MiB). For Reasonix's paste-into-chat
  use case, even 1 MiB is generous.

---

## Integration Contract

### Input bounds

- **`WriteAll(text string)`**
  - **Accepted**: Any Go `string` (UTF-8). Length is bounded only by process memory.
  - **Rejected**: None — `WriteAll` does not validate, sanitize, or bound the input.
  - **Max safe size**: Platform-dependent. macOS `pbcopy` is ~practical-OS-limit.
    Linux `xclip`/`xsel`/`wl-copy` accept arbitrary sizes. Windows `GlobalAlloc`
    can fail for very large allocations. Callers should limit to ≤ 16 MiB for
    portability.

### Output shape

- **`ReadAll() (string, error)`**
  - **Return type**: Go `string` (UTF-8). May contain any Unicode including
    control characters, null bytes (Go strings allow `\x00`), and arbitrary
    binary data the clipboard may hold.
  - **Nil/empty guarantees**: Returns `("", nil)` for empty clipboard. Returns
    `("", error)` on failure — never returns a non-nil error with a non-empty
    string.
  - **Encoding guarantees**: UTF-8. Windows backend decodes UTF-16LE to UTF-8
    via `syscall.UTF16ToString`. macOS/Linux backends pass through whatever the
    paste command outputs — typically UTF-8 but no re-encoding is performed.
  - **Size bounds**: Windows: ~1 MiB (due to fixed array cast). macOS/Linux:
    **unbounded** — caller must be prepared for arbitrarily large results.

### Side effects

- **I/O**: Reads/writes the system clipboard via OS APIs or subprocesses. No
  file I/O (except Plan 9 `/dev/snarf`). No network I/O.
- **Subprocesses spawned**: macOS: `pbcopy`/`pbpaste`. Linux: `xclip`, `xsel`,
  `wl-copy`, `wl-paste`, `clip.exe` (WSL), `powershell.exe` (WSL),
  `termux-clipboard-get`, `termux-clipboard-set`. Windows: none (pure Win32 API).
- **Global state**: `clipboard_unix.go` `init()` probes PATH and sets package-level
  vars (`pasteCmdArgs`, `copyCmdArgs`, `Unsupported`, `trimDos`). Importing the
  package triggers these probes once.
- **Goroutine safety**: `ReadAll` and `WriteAll` are goroutine-safe. On Windows,
  `runtime.LockOSThread()` is used internally to prevent clipboard deadlocks
  when goroutines migrate OS threads.

### Error modes

| Error | Condition |
|-------|-----------|
| `"No clipboard utilities available…"` | No supported clipboard tool found in PATH at `init()` time |
| OS/tool error (wrapped) | `pbcopy`, `xclip`, `xsel`, `wl-copy`, `wl-paste`, `termux-clipboard-*`, or Win32 API failure |
| Panic | Only if `unsafe` pointer cast on Windows reads beyond heap allocation (SEC-001) — low probability, high severity |

No returned error is typed — all errors are opaque `error` values.

### Resource bounds

- **Memory (WriteAll)**: Copies the full input `string` to a `[]byte` before
  writing to the clipboard tool's stdin pipe. O(n) where n = input length.
- **Memory (ReadAll)**: macOS/Linux: O(n) where n = clipboard content length
  (unbounded). Windows: bounded to ~2 MiB by fixed array cast.
- **CPU**: O(n) for the copy/write path. Negligible CPU beyond string→bytes
  conversion and UTF-16→UTF-8 transcoding on Windows.
- **Subprocess lifetime**: Paste/copy commands are run synchronously. No timeout
  is enforced by the library. If a command hangs (e.g. `xclip` waiting on a
  broken X11 connection), the calling goroutine blocks indefinitely.

### Explicit non-guarantees

- **Does NOT validate or sanitize clipboard content.** `ReadAll()` returns
  whatever is on the clipboard — including binary garbage, ANSI escapes, control
  characters, null bytes, or extremely long strings. The caller is responsible
  for treating the return value as untrusted user data.
- **Does NOT limit output size on macOS/Linux.** Callers that cannot handle
  arbitrarily large strings should impose their own cap (see SEC-004).
- **Does NOT pin tool paths.** Commands are resolved via `$PATH` (see SEC-002).
- **Does NOT escape or quote data passed to subprocesses.** Text is written to
  the subprocess's stdin pipe, not to argv — this is the correct pattern and
  avoids command injection. However, the Windows `WriteAll` uses `EMPTYCLIPBOARD`
  followed by `SETCLIPBOARDDATA`, which clears **all** clipboard formats, not
  just text — any non-text clipboard content (images, files, rich text) is lost.

### Integration examples

**CORRECT — cap clipboard paste size:**
```go
text, err := clipboard.ReadAll()
if err != nil {
    return err
}
if len(text) > 1<<20 { // 1 MiB
    text = text[:1<<20]
}
// text is now safe to render / insert
```

**CORRECT — wrap WriteAll with a size guard:**
```go
func safeWriteAll(text string) error {
    const maxClipboard = 16 << 20 // 16 MiB
    if len(text) > maxClipboard {
        return fmt.Errorf("clipboard content too large: %d bytes", len(text))
    }
    return clipboard.WriteAll(text)
}
```

**INCORRECT — passing unsanitized clipboard data to a shell or SQL:**
```go
// DANGEROUS: clipboard text may contain shell metacharacters, SQL, or escape sequences
text, _ := clipboard.ReadAll()
exec.Command("sh", "-c", "echo "+text).Run()
db.Exec("INSERT INTO notes VALUES ('" + text + "')")
```

**INCORRECT — assuming clipboard data is plain text:**
```go
text, _ := clipboard.ReadAll()
fmt.Printf("\033]0;%s\007", text) // OSC sequence injection via terminal title
```

### Caller impact assessment for Reasonix

Reasonix uses clipboard in two user-initiated paths:

1. **Paste** (`chat_tui_paste.go:122`) — `ReadAll()` result goes into the chat
   composer. The TUI input widget is the sink. Control characters and ANSI
   escapes are not dangerous in a Bubble Tea `textinput` (they render as literal
   text, not interpreted codes). No exfiltration path exists.
2. **Copy** (`transcript.go:37`) — User-selected transcript text is written to
   clipboard via `WriteAll()`. The source is Reasonix's own rendered output,
   not attacker-controlled content.

**Verdict**: The library's risk profile is acceptable for Reasonix's use case.
The PATH resolution concern (SEC-002) exists in every clipboard utility of this
class. The Windows buffer issue (SEC-001) is unlikely to manifest in practice
given typical clipboard sizes. Both the paste and copy paths are
user-initiated, not automated.

---

### Assessment

**Safe to use** with the caveats documented above. No blocking issues.

### Additional notes

- v0.1.4 is the latest release (no updates available). The library has been
  stable since its last release.
- No external dependencies — the entire attack surface is stdlib + OS clipboard
  APIs, which is appropriate for a library of this nature.
- The `Unsupported` exported variable is set to `true` when no clipboard tool
  is found. Reasonix already handles this gracefully via OSC 52 fallback in
  `copyToClipboard()` (`transcript.go:36`).
