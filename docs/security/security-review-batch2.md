# Security Review — Batch 2: I/O Surface

**Scope:** 8 vendored dependencies at `/home/dev/reasonix/go1/vendor/`

| # | Package | Version | Files |
|---|---------|---------|-------|
| 1 | `golang.org/x/sys` | — | 368 Go files |
| 2 | `golang.org/x/term` | — | 13 Go files |
| 3 | `golang.org/x/text` | — | 29 Go files |
| 4 | `golang.org/x/mod` | — | 3 Go files (semver only) |
| 5 | `golang.org/x/sync` | — | 3 Go files (errgroup only) |
| 6 | `github.com/BurntSushi/toml` | v2 | 14 Go files |
| 7 | `github.com/dlclark/regexp2` | v2 | 31 Go files |
| 8 | `github.com/yuin/goldmark` | — | 57 Go files |

---

## 1. `golang.org/x/sys` (system calls, OS interaction)

**os/exec usage:** None found. The `Exec()` function in `unix/syscall_unix.go:581-582` and `syscall_zos_s390x.go:2734-2735` are thin wrappers around `syscall.Exec()` — they provide the API for callers, they do not invoke exec themselves. This is expected for a syscall library.

**Path traversal / symlinks / temp files:** All symlink/readlink/Lstat operations are low-level OS wrappers (providing the API surface). No active path traversal or symlink-following vulnerabilities. No temp file creation.

**init() functions (2 found):**
- `unix/fcntl_linux_32bit.go:9` — Sets `fcntl64Syscall = SYS_FCNTL64` on 32-bit Linux. **Safe.**
- `unix/syscall_zos_s390x.go:36-50` — On Z/OS only: reads env vars `__ZOS_XSYSTRACE` and `__ZOS_XSYSTRACEFD`, opens a trace file descriptor. **Z/OS specific, not reachable on Linux.** Risk: **Low** (platform-gated).

**Windows `ComposeCommandLine` — silent quote stripping (Medium):**
`windows/exec_windows.go:120-121` — When the program name needs quoting (`mustQuote`), interior `"` characters are silently stripped with a `continue`. An attacker who controls the program name can inject escape-breaking quotes that are dropped, changing argument boundaries. **Risk: Medium.**

**Windows `CommandLineToArgv` — fixed-size array type (Low):**
`windows/exec_windows.go:188` — The return type is `*[8192]*[8192]uint16` per the Windows API docs, but the actual count may exceed 8192 (documented in issue #63236). An overflow here would be caught by Go's bounds checking on `unsafe.Slice`. **Risk: Low** (defense in depth).

**Overall: Low** — standard system-call library; no active exec calls, no file I/O, no network.

---

## 2. `golang.org/x/term` (terminal I/O)

**os/exec usage:** None.

**File I/O:** None. Operates exclusively on file descriptors passed by the caller.

**Network:** None.

**init():** None.

**Unsafe/reflect:** None. `terminal.go` uses only `unicode/utf8`, `bytes`, `io`, `strconv`.

**Input parsing:**
- `terminal.go:bytesToKey()` (line ~140-220) — VT100 escape sequence parser. Properly bounds-checks all slices before access. Only matches alphanumeric terminator characters. Unrecognized sequences return `utf8.RuneError`. No injection vector.

**Memory safety:**
- `terminal.go:maxLineLength = 4096` (line ~360) — Hard cap on input line length. **Safe.**
- `terminal.go:inBuf [256]byte` (line ~125) — Fixed-size read buffer. **Safe.**

**Overall: Low** — minimal attack surface; read-only descriptor operations and line-editing.

---

## 3. `golang.org/x/text` (text processing, encoding)

**os/exec usage:** None.

**File I/O:** None.

**Network:** None.

**init() (2 found):**
- `secure/bidirule/bidirule.go:250` — Precomputes ASCII bidi property lookup table (128 entries). **Safe.**
- `encoding/simplifiedchinese/gbk.go:268` — Validates encoding table count. **Safe.**

**Unsafe/reflect:** None.

**Memory exhaustion:**
- `unicode/bidi/bidi.go:97,326` — `bytes.Runes(p.p)` allocates a full `[]rune` slice proportional to input size. A large input string causes O(n) allocation with no streaming path. **Risk: Medium.**
- `transform/transform.go:550-561` — `grow()` doubles buffer up to 256 bytes, then 50% growth. Zero-progress loops are possible if a transformer silently accepts input without producing output. **Risk: Medium.**

**Encoding validation:**
- GBK/GB18030 decoder (`encoding/simplifiedchinese/gbk.go:37-104`) — Invalid byte sequences produce `utf8.RuneError` (`\uFFFD`). No crash, no undefined bytes. Single-byte forward progress guaranteed per error. **Safe.**
- `UTF8Validator` (`encoding/encoding.go:241-275`) — Correctly distinguishes incomplete vs. invalid sequences. **Safe.**

**Overall: Medium** — O(n) rune allocations in bidi; otherwise clean.

---

## 4. `golang.org/x/mod` (module path manipulation)

**What's vendored:** Only the `semver` subpackage (3 files, `semver/semver.go`). The `mod/module`, `modfile`, `sumdb`, and `zip` subpackages are **not present**.

**os/exec:** None (stdlib imports only: `slices`, `strings`).

**File I/O:** None.

**Network:** None.

**init():** None.

**Path traversal risk:** None. The semver parser enforces strict grammar: leading `v`, then `MAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]]`. A string like `../../../etc/passwd` would fail immediately at `v[0] != 'v'`.

**Memory:** No dynamic allocations beyond string slicing. All loops bounded by input length.

**Overall: None** — minimal, pure-string parser with no exploitable surface.

---

## 5. `golang.org/x/sync` (concurrency primitives)

**What's vendored:** Only the `errgroup` subpackage (`errgroup/errgroup.go`, 169 lines). The `singleflight`, `semaphore`, and `syncmap` packages are **not present**.

**os/exec:** None (imports only: `context`, `fmt`, `sync`).

**File I/O:** None.

**Network:** None.

**init():** None.

**Unsafe/reflect:** None.

**Concurrency concerns:**
- `errgroup/errgroup.go:157` — `SetLimit` reads `len(g.sem)` without synchronization. If caller violates documented contract (modifies limit while goroutines active), this races with concurrent channel ops. **Risk: Medium** (contract violation only).
- `errgroup/errgroup.go:159` — `g.sem = make(chan token, n)` replaces the semaphore channel while old goroutines may hold references to the orphaned channel, causing silent goroutine leaks. **Risk: Medium** (contract violation only).

**Overall: Low** — well-engineered, documented; risks only on documented-contract violation.

---

## 6. `github.com/BurntSushi/toml` (config parsing)

**os/exec usage:** None.

**Network:** None.

**init():** None.

**Unsafe:** None.

**File I/O:**
- `decode.go:41` — `DecodeFile(path, v)` calls `os.Open(path)` with no path sanitization, symlink checking, or size limit. **Risk: Medium** (caller-dependent; if path is attacker-controlled, arbitrary file read).
- `decode.go:162` — `io.ReadAll(dec.r)` reads entire input into memory with no size cap. A multi-gigabyte TOML file causes OOM. **Risk: Medium.**

**Input parsing — reflect exploitation:**
- `decode.go:337-339` — `unifyAnything()` blindly assigns any parsed TOML structure into an `interface{}` target, bypassing all type checks. Data is limited to parser output (maps, slices, primitives), but this undermines type safety. **Risk: Medium.**

**Lexer/parser robustness:**
- No call-stack recursion from nesting (state-function machine in `lex.go`). Stack overflow is not possible.
- `lex.go:225-231` — `skip()` has a panic-on-EOF-after-EOF guard, not a hang. **Safe.**
- `parse.go:288-310` — Tries 6 datetime formats in sequence with `time.ParseInLocation`. Each failed parse is O(1). No ReDoS. **Safe.**

**Overall: Medium** — unbounded `io.ReadAll` and unvalidated file path in `DecodeFile` are the main concerns.

---

## 7. `github.com/dlclark/regexp2` (regex engine)

**os/exec:** None.

**File I/O:** None.

**Network:** None.

**init():** None.

### Critical: Catastrophic Backtracking / ReDoS

- `runner.go:188-938` — `executeDefault()` implements a backtracking VM with **no backtracking step limit**. Nested quantifiers like `(a+)+b` on input `"aaaaac"` cause stack/doubling loops until timeout or OOM. **Critical.**
- `runner.go:946-955` — `doubleIntSlice()` doubles backtrack/stack/crawl arrays with **no upper bound**. A 10 KB input can cause many MB of allocation. **Critical.**
- `regexp.go:15-16` — `DefaultMatchTimeout = math.MaxInt64` (effectively infinite). Caller must explicitly set `MatchTimeout` to a finite duration. **High.**
- `fastclock.go:85` — Timeout granularity is ~100 ms (`DefaultClockPeriod`). An attacker can get millions of backtracking steps before the first check. **High.**

### High: Memory exhaustion

- `regexp.go:284-302` — `getRunes()` allocates `[]rune(len(s))` for every string input. 1 GB UTF-8 input → ~1 GB rune allocation. No pooling on the hot path. **High.**

### Medium: Unsafe pointer casts

- `helpers/indexof.go:364-365` — `unsafe.Slice((*byte)(unsafe.Pointer(&a[0])), len(a)*4)` casts `[]rune` to `[]byte` for fast comparison. Assumes `len(a) > 0` and rune width = 4 bytes. Panics on empty slice. **Medium.**

**Overall: Critical** — intrinsic ReDoS risk from backtracking; no step limit; timeout defaults to infinite.

---

## 8. `github.com/yuin/goldmark` (markdown parser)

**os/exec:** None.

**File I/O:** None.

**Network:** None.

**init() (1 found):**
- `util/unicode_case_folding.go:8` — Initializes unicode case-folding map from generated data. **Safe.**

### Critical: XSS via raw HTML (when Unsafe is enabled)

- `renderer/html/html.go:597-611` — `renderRawHTML()` writes raw HTML directly when `r.Unsafe` is true. Default (`Unsafe=false`) replaces raw HTML with `<!-- raw HTML omitted -->`. **Critical when WithUnsafe() is used.**
- `renderer/html/html.go:495-511` — Same for raw HTML blocks. **Critical when WithUnsafe() is used.**

### High: HTML block parser allows `<script>`, `<iframe>`

- `parser/html_block.go:20-39` — Recognizes `<script>`, `<pre>`, `<style>`, `<textarea>` as HTML block tags. The renderer respects `Unsafe` flag by default. **High** (if Unsafe is enabled or if a vulnerability bypasses the Unsafe gate).

### Medium: URL protocol filtering

- `renderer/html/html.go:915-933` — `IsDangerousURL()` filters `javascript:`, `vbscript:`, `file:`, and `data:` (except known image types). The filter is case-insensitive via `bytes.Equal(bytes.ToLower(...))`. Uses a fixed allowlist (`data:image/png`, `data:image/gif`, `data:image/jpeg`, `data:image/webp`, `data:image/svg+xml`). Other `data:` URIs are blocked. **Medium** — relies on exact protocol prefix matching; custom URI schemes not covered.

### Medium: Unbounded memory on unclosed HTML comments

- `parser/raw_html.go:98-118` — `parseComment()` appends all remaining lines to the node if `<!--` is opened without a closing `-->`. O(n) memory proportional to remaining input. **Medium.**
- `parser/raw_html.go:124-139` — Same pattern for `<?`/`?>`, CDATA, and declaration blocks. **Medium.**

### Low: Unsafe zero-copy string/byte conversions

- `util/util_unsafe_go120.go:12-20` — `BytesToReadOnlyString` and `StringToReadOnlyBytes` use `unsafe.Pointer` and `reflect.StringHeader`/`SliceHeader` for zero-copy conversion. The "ReadOnly" contract is consistently respected across the codebase. Go 1.21 variant uses `unsafe.String` / `unsafe.Slice`. **Low** (correct usage, build-tag guarded).

### Medium: Table row unbounded column append

- `extension/table.go:190-255` — `parseRow()` loops on `|` delimiters with no column cap. A line with thousands of `|` characters creates thousands of `TableCell` nodes. **Medium.**

**Overall: Medium** — safe by default (Unsafe=false); risks are opt-in. Memory unboundedness on malformed HTML blocks is the main concern.

---

## Summary Table

| Package | Highest Risk | Key Finding | File:Line |
|---------|-------------|-------------|-----------|
| `golang.org/x/sys` | **Medium** | `ComposeCommandLine` silently strips quotes | `windows/exec_windows.go:120-121` |
| `golang.org/x/term` | **Low** | No findings of significance | — |
| `golang.org/x/text` | **Medium** | `bytes.Runes` allocates O(input) | `unicode/bidi/bidi.go:97,326` |
| `golang.org/x/mod` | **None** | Semver-only; no exploitable surface | — |
| `golang.org/x/sync` | **Medium** | `SetLimit` races on contract violation | `errgroup/errgroup.go:157,159` |
| `BurntSushi/toml` | **Medium** | Unbounded `io.ReadAll`, unvalidated file path | `decode.go:41,162` |
| `dlclark/regexp2` | **Critical** | No backtracking limit; timeout defaults to infinite | `runner.go:946-955`, `regexp.go:15-16` |
| `yuin/goldmark` | **Critical** | XSS via raw HTML when Unsafe enabled; unclosed comments OOM | `renderer/html/html.go:597-611`, `parser/raw_html.go:98-118` |

## Priority Actions

1. **`dlclark/regexp2` (Critical)** — Ensure all callers set a finite `MatchTimeout`. The library provides no backtracking step limit, making any user-supplied regex a ReDoS vector. Consider wrapping with a context-based timeout.

2. **`yuin/goldmark` (Critical)** — Never use `WithUnsafe()` when rendering user-supplied markdown. The `<!-- raw HTML omitted -->` default is the only XSS protection.

3. **`BurntSushi/toml` (Medium)** — Wrap `DecodeFile`/`Decode` with a size-limited reader if accepting untrusted input. Validate file paths when using `DecodeFile`.

4. **`golang.org/x/sys` (Medium)** — If `ComposeCommandLine` is used with attacker-influenced program names, quote stripping can alter argument boundaries. Validate program names independently.
