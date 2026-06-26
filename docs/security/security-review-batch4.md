# Security Review — Batch 4: TUI Framework Dependencies

**Date:** 2025-01-16  
**Scope:** Vendored Go packages under `/home/dev/reasonix/go1/vendor/`  
**Method:** Manual code audit — command execution, filesystem access, network calls, `init()` side effects, unsafe/syscall usage, known CVEs.

---

## Summary

| Package | Version | Overall Risk | Key Concerns |
|---|---|---|---|
| bubbles/v2 | v2.1.0 | None | No I/O, no init, no syscall |
| bubbletea/v2 | v2.0.7 | **Low** | Exec by design, file logging, env-var trace |
| lipgloss/v2 | v2.0.4 | None | Terminal-only I/O (CONIN$/CONOUT$) |
| colorprofile | v0.4.3 | **Low** | Runs `tmux info` when TMUX env var set |
| ultraviolet | dev snapshot | **Low** | Optional debug file via env var, `/dev/tty` open |
| x/ansi | v0.11.7 | **Low** | Env-var init, optional Kitty graphics file ops |
| chroma/v2 | v2.27.0 | **Low** | Lexer registrations in init(), optional font file read |
| xo/terminfo | v0.0.0-... | None | Reads terminfo db files (expected) |
| goleak | v1.3.0 | None | runtime.Stack introspection only |
| gogo/protobuf | v1.3.2 | **Medium** | **Deprecated/unmaintained**, heavy unsafe usage, no CVE-specific patch |
| wincred | v1.2.3 | **Medium** | Credential manager API, unsafe syscalls, Windows-only |

---

## 1. charm.land/bubbles/v2 v2.1.0

**No findings.** Pure TUI widget components (cursor, key, spinner, textarea, viewport). No `os/exec`, no network calls, no filesystem access, no `init()` side effects, no `unsafe`.

---

## 2. charm.land/bubbletea/v2 v2.0.7

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| Command execution API (by design) | **Low** | `exec.go:39-54` | `ExecProcess` wraps `os/exec.Cmd`. This is a **core framework feature** for spawning interactive subprocesses (editors, pagers). The caller provides the command; bubbletea doesn't construct commands from untrusted input. |
| Log-to-file helper | **Low** | `logging.go:36` | `LogToFile()` opens an arbitrary file path provided by the caller for logging. `os.OpenFile(path, O_WRONLY\|O_CREATE\|O_APPEND, 0o600)`. No default path; caller-controlled. |
| Trace env-var file write | **Low** | `tea.go:635` | When `TEA_TRACE` env var is set, opens that path for tracing. Attacker controlling env vars could write to an arbitrary file, but only if they already have env var control. |
| Panic dump file | **Low** | `tea.go:1282,1307` | When `TEA_DEBUG=true`, writes panic logs to `bubbletea-panic-{timestamp}.log` in the CWD. Local-only, opt-in. |

**No network calls, no `unsafe`, no syscall usage.**

---

## 3. charm.land/lipgloss/v2 v2.0.4

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| Terminal console file open (Windows) | **Low** | `query.go:40,47` | Opens `CONIN$` and `CONOUT$` on Windows to detect background color when stdin/stdout are redirected. Expected terminal interaction; no arbitrary file access. |

**No command execution, no network, no init() side effects.**

---

## 4. github.com/charmbracelet/colorprofile v0.4.3

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| External command execution | **Low** | `env.go:248` | `exec.CommandContext(context.Background(), "tmux", "info")` — runs a **hardcoded** command with no user-controlled arguments. Only invoked when `TMUX` env var is found. Risk: if `tmux` binary is compromised, attacker-controlled binary runs. In a vendored context, this is environment-driven and not attacker-triggerable remotely. |

**No network calls, no filesystem writes, no init() side effects.**

---

## 5. github.com/charmbracelet/ultraviolet (dev snapshot)

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| `init()` — decoder registration | **Low** | `decoder.go:1320` | Registers ANSI decoder maps. No I/O, no network. Safe. |
| Debug file via env var | **Low** | `terminal_screen.go:59` | When `UV_DEBUG` env var is set, opens that path for debug logging. Similar to bubbletea's TEA_TRACE: attacker needs env var control. |
| `/dev/tty` open (Unix) | **Low** | `tty_unix.go:16` | Opens `/dev/tty` for terminal I/O. Expected behavior for a TUI terminal screen implementation. |
| `CONIN$`/`CONOUT$` open (Windows) | **Low** | `tty_windows.go:16,20` | Same as lipgloss — expected terminal interaction. |

**No network calls, no arbitrary command execution.**

---

## 6. github.com/charmbracelet/x/ansi v0.11.7

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| `init()` reads env var | **Low** | `method.go:20-27` | Reads `RUNEWIDTH_EASTASIAN` env var and sets package-level state. No security impact — affects only display width calculation. |
| Kitty graphics — file read | **Low** | `kitty/writer.go:69` | `os.Open(o.File)` when `Transmission==File`. Caller provides the file path. Only invoked when the user explicitly calls `EncodeGraphics`. |
| Kitty graphics — temp file | **Low** | `kitty/writer.go:92` | `os.CreateTemp(GraphicsTempDir, GraphicsTempPattern)`. Default dir is `""` (system temp). Only when user calls `EncodeGraphics` with `Transmission==TempFile`. |

**No command execution, no network, no unsafe.**

---

## 7. github.com/alecthomas/chroma/v2 v2.27.0

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| `init()` — DNS lexer registration | **None** | `lexers/dns.go:10` | Registers analyser function on global lexer registry. No I/O. |
| `init()` — MySQL lexer registration | **None** | `lexers/mysql.go:12` | Same pattern — pure function registration. |
| `init()` — Zed lexer registration | **None** | `lexers/zed.go:8` | Same pattern — pure function registration. |
| SVG formatter — font file read | **Low** | `formatters/svg/svg.go:37` | `os.ReadFile(fileName)` in `EmbedFontFile()`. Only triggered by **explicit caller action** to embed a font. The caller controls the path. |

**No command execution, no network calls.**

---

## 8. github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| Terminfo database file reads | **None** | `terminfo.go:226` | `ioutil.ReadFile(f)` reads terminfo database files from well-known system paths (`/usr/share/terminfo/...`). This is expected behavior for terminal capability detection. No arbitrary path injection — reads from constructed paths based on terminal name. |

**No command execution, no network, no init() side effects.**

---

## 9. go.uber.org/goleak v1.3.0

**No findings.** Purely uses `runtime.Stack()` (`internal/stack/stacks.go:73`) for goroutine introspection. No filesystem access, no network, no command execution, no `unsafe`, no `init()` side effects.

---

## 10. github.com/gogo/protobuf v1.3.2 — **DEPRECATED**

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| Deprecated, unmaintained since ~2022 | **Medium** | — | Project is archived. Maintainers explicitly recommend migration to `google.golang.org/protobuf`. No new patches for vulnerabilities. |
| Extensive `unsafe.Pointer` usage | **Medium** | `proto/pointer_unsafe.go:34-307` | Entire marshal/unmarshal hot path uses `unsafe.Pointer` for struct field access. Memory corruption bugs in generated code or crafted input could lead to undefined behavior. |
| `unsafe.Pointer` in gogo extensions | **Medium** | `proto/pointer_unsafe_gogo.go:41` | Gogo-specific unsafe pointer operations. |
| High-risk `init()` — proto registration | **Low** | `gogoproto/gogo.pb.go:708-786` | Registers 60+ proto extensions. No I/O, no network. |
| `init()` — type registration | **Low** | `proto/duration_gogo.go:47`, `proto/timestamp_gogo.go:47`, `proto/wrappers_gogo.go:103` | Registers `time.Duration`, `time.Time`, wrapper types. |
| `init()` — proto file registration | **Low** | `gogoproto/gogo.pb.go:787`, `protoc-gen-gogo/descriptor/descriptor.pb.go:2703` | Registers proto file descriptors. |
| Text format parser — large/recursive input | **Medium** | `proto/text_parser.go:100-200` | The text parser (`text_parser.go`) has no recursion depth limit. Malformed deeply nested input could cause stack exhaustion (DoS). This is a known class of protobuf parsing issues. |
| CVE-2024-24786 (affects all protobuf Go) | **Medium** | — | Denial of service via crafted protobuf message with excessive field nesting. While this CVE was reported against `google.golang.org/protobuf`, gogo/protobuf's similar marshal/unmarshal code paths are structurally the same and not patched. |
| No network calls, no command execution | — | — | Confirmed absent. |

**Recommendation:** Migrate to `google.golang.org/protobuf` (the official Go protobuf module). Gogo/protobuf is archived, and no further security patches will be released.

---

## 11. github.com/danieljoos/wincred v1.2.3

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| Reads credentials from Windows Credential Manager | **Low** (API — depends on caller) | `wincred.go:29-34` | `GetGenericCredential()` calls `sysCredRead()` which invokes Windows `CredReadW`. Reads stored credentials by target name. |
| Reads domain passwords | **Low** (API) | `wincred.go:83-89` | `GetDomainPassword()` — same pattern for domain credentials. |
| Lists all credentials | **Low** (API) | `wincred.go:126-136` | `List()` calls `sysCredEnumerate("", true)` — enumerates **all** stored Windows credentials. |
| Lists credentials with filter | **Low** (API) | `wincred.go:141-150` | `FilteredList(filter)` — enumerates credentials matching a prefix filter. |
| Writes credentials | **Low** (API) | `wincred.go:41-44` | `Write()` calls `sysCredWrite()` — stores credentials. |
| Deletes credentials | **Low** (API) | `wincred.go:48-51` | `Delete()` calls `sysCredDelete()` — removes credentials. |
| Heavy `unsafe.Pointer` + syscall usage | **Medium** | `sys.go:76-142` | Uses `unsafe.Pointer`, `reflect.SliceHeader`, and direct `syscall.SyscallN` to call Windows API (`advapi32.dll`). Memory safety bugs could lead to credential data corruption or leak. |
| `unsafe` in conversion functions | **Medium** | `conversion.go:39-62` | `goBytes()` and `sysToCredential()` construct Go slices from C pointers via `unsafe.Pointer(reflect.SliceHeader{...})`. Risks: incorrect length/capacity could read out-of-bounds memory. |
| No-op stubs on non-Windows | **None** | `sys_unsupported.go` | All credential functions return "Operation not supported" on non-Windows. Safe. |
| No network calls, no command execution | — | — | Confirmed absent. |

**Risk assessment:** wincred is an **API wrapper** — it doesn't leak credentials on its own. The risk depends entirely on how the consuming code uses it. If the main project calls `List()` or `GetGenericCredential()` with attacker-controlled target names and forwards results somewhere, that would be a credential disclosure vulnerability in the **caller**, not in wincred itself. However, the heavy `unsafe`/`syscall` usage in `conversion.go` and `sys.go` is concerning for memory safety.

---

## Cross-Package Summary

### Commands executed externally
- **colorprofile** runs `tmux info` (hardcoded, env-triggered) — `env.go:248`

### Filesystem reads
- **terminfo** reads terminfo DB files — `terminfo.go:226`
- **chroma** optional font file read — `formatters/svg/svg.go:37`
- **x/ansi/kitty** optional image file read — `kitty/writer.go:69`

### Filesystem writes
- **bubbletea** log file (caller path), panic dumps (CWD), trace (env var) — `logging.go:36`, `tea.go:635,1282,1307`
- **ultraviolet** debug file (env var) — `terminal_screen.go:59`
- **x/ansi/kitty** temp file for graphics — `kitty/writer.go:92`

### Network calls
- **None** across all reviewed packages. No HTTP clients, no net.Dial, no net.Listen.

### `init()` functions with side effects
- **x/ansi/method.go:20** — reads `RUNEWIDTH_EASTASIAN` env var (benign)
- **ultraviolet/decoder.go:1320** — registers decoder tables (benign)
- **chroma lexers** (3 files) — register analyser functions on global registry (benign)
- **gogo/protobuf** (6+ files) — register protobuf types, extensions, file descriptors (benign but extensive)

### `unsafe` / `syscall` usage
- **wincred** — extensive (`conversion.go`, `sys.go`)
- **gogo/protobuf** — extensive (`pointer_unsafe.go`, `pointer_unsafe_gogo.go`)

---

## Recommendations by Priority

1. **HIGH:** Migrate from `gogo/protobuf` to `google.golang.org/protobuf` immediately. The project is archived (no security patches), uses unsafe pervasively, and the text parser lacks recursion limits.
2. **MEDIUM:** Audit callers of `danieljoos/wincred` in the main project. Ensure that `List()`, `GetGenericCredential()`, and `FilteredList()` are never called with attacker-controlled target names, and that credential data never reaches logs, error messages, or network output.
3. **LOW:** Env-var-driven file writes (`TEA_TRACE`, `UV_DEBUG`, `TEA_DEBUG`) are low risk but worth documenting — if an attacker controls environment variables and the binary runs with elevated privileges, they could write to arbitrary paths.
