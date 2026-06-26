# Security Review — Batch 3: I/O Surface Vendored Dependencies

**Scope:** 8 packages in `/home/dev/reasonix/go1/vendor/`  
**Date:** 2025-07-17  
**Reviewer:** Security Subagent

---

## Summary

| Package | Version | Risk | Key Concerns |
|---|---|---|---|
| `golang.org/x/image` (webp) | vendored | **Low** | Standard image decoder; no exec/network |
| `github.com/atotto/clipboard` | vendored | **Medium** | Spawns external clipboard tools via `exec.Command`; Windows uses `unsafe.Pointer` |
| `github.com/mattn/go-runewidth` | vendored | **Low** | Reads env vars; Windows calls kernel32.dll |
| `github.com/muesli/cancelreader` | vendored | **Low** | OS-level I/O syscalls (epoll/kqueue/select) |
| `github.com/rivo/uniseg` | vendored | **None** | Pure data; `http.Get` only in `//go:build generate` files |
| `github.com/clipperhouse/displaywidth` | vendored | **None** | Pure computation |
| `github.com/clipperhouse/uax29/v2` | vendored | **None** | Pure computation |
| `github.com/lucasb-eyer/go-colorful` | vendored | **None** | Pure math |

---

## 1. `golang.org/x/image` (WebP Decoder)

**Files:** `webp/`, `vp8/`, `vp8l/`, `riff/`, `draw/`, `math/f64/`

### Findings

**`init()` — benign**
- `vendor/golang.org/x/image/webp/decode.go:294`: Registers `"webp"` format with `image.RegisterFormat`. Standard Go pattern, no side effects.

**Command execution:** None found.  
**File system access:** None.  
**Network calls:** None.  
**`unsafe` / `syscall`:** None.

### Risk: **Low**
Standard image decoder. Input is read from an `io.Reader` and parsed with bounds checks. No command execution, file system, or network surface.

---

## 2. `github.com/atotto/clipboard`

**Files:** `clipboard.go`, `clipboard_darwin.go`, `clipboard_unix.go`, `clipboard_windows.go`, `clipboard_plan9.go`

### Findings

#### **Medium — External command execution on macOS/Linux**

- `vendor/github.com/atotto/clipboard/clipboard_darwin.go:19`: `exec.Command(pasteCmdArgs)` — spawns `pbpaste`
- `vendor/github.com/atotto/clipboard/clipboard_darwin.go:23`: `exec.Command(copyCmdArgs)` — spawns `pbcopy`
- `vendor/github.com/atotto/clipboard/clipboard_unix.go:103`: `exec.Command(pasteCmdArgs[0], pasteCmdArgs[1:]...)` — spawns `xsel`, `xclip`, `wl-paste`, or `termux-clipboard-get`
- `vendor/github.com/atotto/clipboard/clipboard_unix.go:110`: `exec.Command(copyCmdArgs[0], copyCmdArgs[1:]...)` — spawns `xclip`, `xsel`, `wl-copy`, `termux-clipboard-set`, or `clip.exe`

The commands are **hardcoded** (not user-controllable), but subprocess execution from a library is always notable. Data is piped through stdin/stdout to these external tools.

#### **Low — `init()` probes PATH with `exec.LookPath`**

- `vendor/github.com/atotto/clipboard/clipboard_unix.go:51-98`: The `init()` function iterates through available clipboard tools (`wl-copy`, `xclip`, `xsel`, `termux-clipboard-set`, `clip.exe`) using `exec.LookPath`. This only checks PATH existence, does not execute. Sets `Unsupported = true` if none found.

#### **Low — Windows uses `unsafe.Pointer` for clipboard memory**

- `vendor/github.com/atotto/clipboard/clipboard_windows.go:69`: `(*[1 << 20]uint16)(unsafe.Pointer(l))[:]` — standard pattern for reading Windows clipboard data via `GlobalLock`.
- `vendor/github.com/atotto/clipboard/clipboard_windows.go:107`: `uintptr(unsafe.Pointer(&data[0]))` — standard for `lstrcpyW` call.
- `vendor/github.com/atotto/clipboard/clipboard_windows.go:33-47`: Uses `syscall.MustLoadDLL`/`MustFindProc` for `user32.dll` and `kernel32.dll`. These are expected Windows API calls. The `Must*` variants **panic on failure** at startup — a denial-of-service vector if the DLLs are unavailable, but this is normal for Windows clipboard libraries.

#### **Low — Plan9 reads/writes `/dev/snarf`**

- `vendor/github.com/atotto/clipboard/clipboard_plan9.go:11`: `os.Open("/dev/snarf")`
- `vendor/github.com/atotto/clipboard/clipboard_plan9.go:23`: `os.OpenFile("/dev/snarf", os.O_WRONLY, 0666)`

Fixed device path, not attacker-controllable. `/dev/snarf` is the Plan 9 clipboard device.

### Risk: **Medium**
Subprocess execution on Unix systems is the primary concern. The commands are hardcoded to well-known clipboard utilities, but the library will execute whatever binary is found first on `PATH`. In a compromised environment, `PATH` poisoning could redirect `xclip` → malicious binary.

---

## 3. `github.com/mattn/go-runewidth`

**Files:** `runewidth.go`, `runewidth_posix.go`, `runewidth_windows.go`, `runewidth_js.go`, `runewidth_appengine.go`

### Findings

#### **Low — `init()` reads environment variables**

- `vendor/github.com/mattn/go-runewidth/runewidth.go:35-42`: Builds Unicode lookup tables, then calls `handleEnv()`.
- `vendor/github.com/mattn/go-runewidth/runewidth.go:77`: `os.Getenv("RUNEWIDTH_EASTASIAN")` in `handleEnv()`.
- `vendor/github.com/mattn/go-runewidth/runewidth_posix.go:64-69`: `IsEastAsian()` reads `LC_ALL`, `LC_CTYPE`, `LANG` env vars. Only reads, never writes.

#### **Low — Windows uses `syscall.NewLazyDLL` for code page detection**

- `vendor/github.com/mattn/go-runewidth/runewidth_windows.go:16-25`: Calls `GetConsoleOutputCP` via `kernel32.dll` to detect CJK locale. Reads `WT_SESSION` env var. Benign.

#### **Low — posix locale parsing uses `regexp`**

- `vendor/github.com/mattn/go-runewidth/runewidth_posix.go:15`: `regexp.MustCompile(...)` — compiles a fixed regex at startup. Minor, no user input involved.

### Risk: **Low**
No command execution, file I/O, or network. Only reads environment variables and Windows code page. Safe.

---

## 4. `github.com/muesli/cancelreader`

**Files:** `cancelreader.go`, `cancelreader_linux.go`, `cancelreader_bsd.go`, `cancelreader_select.go`, `cancelreader_windows.go`, `cancelreader_unix.go`, `cancelreader_default.go`

### Findings

#### **Low — OS-level I/O syscalls**

- `vendor/github.com/muesli/cancelreader/cancelreader_linux.go:34`: `unix.EpollCreate1(0)` — creates epoll FD
- `vendor/github.com/muesli/cancelreader/cancelreader_linux.go:40`: `os.Pipe()` — creates cancel signal pipe
- `vendor/github.com/muesli/cancelreader/cancelreader_linux.go:46-60`: `unix.EpollCtl(...)` — registers reader and pipe FDs
- `vendor/github.com/muesli/cancelreader/cancelreader_linux.go:110`: `unix.EpollWait(...)` — blocking wait

- `vendor/github.com/muesli/cancelreader/cancelreader_bsd.go:45`: `unix.Kqueue()` — BSD/macOS equivalent
- `vendor/github.com/muesli/cancelreader/cancelreader_select.go:94`: `unix.Select(...)` — fallback for Solaris and `/dev/tty`

- `vendor/github.com/muesli/cancelreader/cancelreader_windows.go:40-62`: Opens `CONIN$` with `windows.CreateFile`, creates cancel event with `windows.CreateEvent`, calls `windows.WaitForMultipleObjects`, `windows.ReadFile`, `windows.GetOverlappedResult`.
- `vendor/github.com/muesli/cancelreader/cancelreader_windows.go:98-107`: `flushConsoleInputBuffer` uses raw `syscall.Syscall`.

All operations are constrained to file descriptors passed by the caller (or `os.Stdin.Fd()` on Windows). No command execution, no file system writes, no network.

### Risk: **Low**
Standard OS-level I/O multiplexing. Expected behavior for a cancel-reader library. The library does not open arbitrary files or execute commands.

---

## 5. `github.com/rivo/uniseg`

**Files:** All `.go` files in `vendor/github.com/rivo/uniseg/`

### Findings

#### **None — `http.Get` only in `//go:build generate` files**

- `vendor/github.com/rivo/uniseg/gen_properties.go:115`: `http.Get(propertyURL)` — downloads Unicode data
- `vendor/github.com/rivo/uniseg/gen_properties.go:149`: `http.Get(emojiURL)` — downloads emoji data
- `vendor/github.com/rivo/uniseg/gen_breaktest.go:70`: `http.Get(url)` — downloads test data

These files have `//go:build generate` constraint (`vendor/github.com/rivo/uniseg/gen_properties.go:1`, `gen_breaktest.go:1`), so they are **excluded from production builds**. They are only invoked by `go generate` during development.

#### Production code
All production `.go` files are pure data: hardcoded Unicode tables (`graphemeproperties.go`, `wordproperties.go`, `sentenceproperties.go`, `lineproperties.go`, `eastasianwidth.go`, `emojipresentation.go`) and string-processing logic (`step.go`, `grapheme.go`, `line.go`, `word.go`, `sentence.go`, `width.go`, `properties.go`, `graphemerules.go`, etc.).

No `init()`, no exec, no file I/O, no network, no `unsafe`, no `syscall`.

### Risk: **None**
All production code is pure string/Unicode processing with no external interactions.

---

## 6. `github.com/clipperhouse/displaywidth`

**Files:** `width.go`, `truncate.go`, `graphemes.go`, `options.go`, `trie.go`, `gen.go`

### Findings

- **`init()`:** None found.
- **Command execution:** None.
- **File system:** None.
- **Network:** None.
- **`unsafe` / `syscall`:** None.

`gen.go` (line 3) only contains `//go:generate go run -C internal/gen .` — generates the trie at development time.

The `trie.go` file is a large generated lookup table (17.25 KiB). Pure data with a `lookup()` function.

The `truncate.go` function (`TruncateString`, `TruncateBytes`) manipulates string/byte slices based on width calculations. No alignment or buffer-overflow risks — standard Go slice operations.

### Risk: **None**
Pure computation on input strings. No OS interaction.

---

## 7. `github.com/clipperhouse/uax29/v2`

**Files:** `graphemes/splitfunc.go`, `graphemes/reader.go`, `graphemes/iterator.go`, `graphemes/ansi.go`, `graphemes/ansi8.go`, `graphemes/trie.go`

### Findings

- **`init()`:** None found.
- **Command execution:** None.
- **File system:** None.
- **Network:** None.
- **`unsafe` / `syscall`:** None.

The `ansi.go` and `ansi8.go` files parse ANSI escape sequences from terminal data. They recognize control sequences (CSI, OSC, DCS, etc.) per ECMA-48. This is a **parser on untrusted input** but operates on `[]byte`/`string` with normal bounds checks. No unsafe pointer manipulation.

The `splitfunc.go` implements UAX#29 grapheme cluster boundary rules. Pure algorithmic processing.

### Risk: **None**
Pure computation. The ANSI parser handles raw bytes but with safe Go slice operations.

---

## 8. `github.com/lucasb-eyer/go-colorful`

**Files:** `colors.go`, `hexcolor.go`, `hsluv.go`, `colorgens.go`, `rand.go`, `soft_palettegen.go`, `happy_palettegen.go`, `warm_palettegen.go`, `sort.go`, `widegamut.go`

### Findings

- **`init()`:** None found.
- **Command execution:** None.
- **File system:** None.
- **Network:** None.
- **`unsafe` / `syscall`:** None.

The `HexColor` type in `hexcolor.go` implements `database/sql.Scanner`, `encoding/json.Marshaler/Unmarshaler`, and `yaml.Marshaler/Unmarshaler`. These parse hex color strings (e.g., `#ff0080`) from external data sources (JSON, YAML, database columns). The `Hex()` function validates format (`len == 4` or `7`, `starts with #`), then parses with `strconv.ParseUint`. Input is bounded and validated.

The `rand.go` file uses `math/rand` (default global source) for color generation in palettes. This is a cryptographically **insecure** PRNG, but for color palette generation this is acceptable. Not used for security-sensitive purposes.

### Risk: **None**
Pure math and color parsing. All external input goes through standard Go parsing (`strconv.ParseUint`, `json.Unmarshal`) with format validation.

---

## Overall Assessment

| Severity | Count | Details |
|---|---|---|
| **Critical** | 0 | — |
| **High** | 0 | — |
| **Medium** | 1 | `atotto/clipboard`: spawns external clipboard tools via `exec.Command` |
| **Low** | 3 | `mattn/go-runewidth`: env var reads + Windows DLL calls; `muesli/cancelreader`: OS I/O syscalls; `atotto/clipboard`: Windows `unsafe.Pointer` + Plan9 `/dev/snarf` |
| **None** | 5 | `rivo/uniseg`, `clipperhouse/displaywidth`, `clipperhouse/uax29/v2`, `lucasb-eyer/go-colorful` are pure computation; `golang.org/x/image` is a standard decoder |

**Notable:** The `atotto/clipboard` package is the only one with a meaningful attack surface. Its subprocess execution on Unix systems is inherent to how clipboard access works on those platforms (no native API). If this is acceptable for the application, the risk is contained. The commands are hardcoded to standard clipboard utilities.

No package uses reflection in a dangerous way, no package evaluates user input as code, and no package makes outbound network connections in production builds.
