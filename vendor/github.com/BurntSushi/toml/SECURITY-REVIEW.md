## Security Review: BurntSushi/toml (v1.1.0-compatible, vendored)

### Summary
- **Target Type**: Source Code — vendored TOML parser library
- **Modes Used**: A (read-only static analysis)
- **Findings**: 6 (0 Critical, 0 High, 3 Medium, 3 Low)
- **Risk Level**: Low — safe for its intended use as a local config-file parser
- **Rule of Two Violation**: No — this library processes local configuration files (trusted input); it does not (A) process untrusted user input in the normal Reasonix use case, has no (B) access to sensitive systems beyond what the caller provides, and cannot (C) change state externally beyond populating caller-provided Go values.
- **Confidence**: High
- **Dependencies reviewed**: 1 total (1 direct; internal/tz.go is the only transitive internal package)
- **CVEs checked**: None found for this package in public databases (NVD, GHSA, OSV). No Go vulnerability DB entries.
- **Release notes analyzed**: Library is vendored at a single snapshot; full version-gap analysis not applicable.
- **Cross-dependency chains identified**: 0

### Findings

#### [SEC-001] Uncaught Panics from `p.bug()` Crash the Application (Medium)
- **Category**: Denial of Service
- **OWASP Reference**: ASI05
- **Location**: `parse.go:127-130` (`bug()`), `parse.go:42-52` (recover handler)
- **Confidence**: Medium
- **Issue**: The `parse()` function deploys a `recover()` handler that only catches `ParseError` panics. The `p.bug()` method calls `panic(fmt.Sprintf(...))` — which produces a plain `string`, not a `ParseError` — so `bug()` panics blow straight through the recover handler and crash the calling goroutine. There are 7 `p.bug()` call sites in `parse.go` that serve as defaults in `switch` statements on lexer item types (`topLevel`, `value`, `keyString`, `valueInteger`, `valueFloat`, `valueBool`). If crafted input triggers an unexpected lexer token (e.g., through an edge case in the lexer state machine), the application crashes rather than returning a parse error.
- **Attack Path**:
  1. Attacker provides a malformed TOML file that causes the lexer to emit an unexpected `itemType` (e.g., a token type the parser's switch doesn't handle in a given context).
  2. Parser hits the `default: p.bug(...)` branch.
  3. `bug()` panics with a `string`, which the `recover()` handler does NOT catch (it only catches `ParseError`).
  4. Panic propagates uncaught → application crashes.
- **Attacker-Controlled**: Partial — attacker controls the TOML input content (if the application loads untrusted config).
- **Guard/Mitigation Present**: None. The `recover()` handler explicitly re-panics on non-`ParseError` values.
- **Residual Exploitability**: Crash-only (DoS). No code execution, no data exfiltration. Requires that the input passes lexer validation but produces a token type the parser doesn't expect in its current context.
- **Evidence**:
  ```go
  // parse.go:42-52
  defer func() {
      if r := recover(); r != nil {
          if pErr, ok := r.(ParseError); ok {
              pErr.input = data
              err = pErr
              return
          }
          panic(r)  // <-- bug() panics end up here
      }
  }()

  // parse.go:127-130
  func (p *parser) bug(format string, v ...any) {
      panic(fmt.Sprintf("BUG: "+format+"\n\n", v...))
  }
  ```
- **Remediation**: Either (a) make `bug()` panic with a `ParseError` instead of a string, or (b) extend the recover handler to also catch `string` panics that start with `"BUG: "` and wrap them. In practice this is low-risk for production because Reasonix loads config files from known paths, not untrusted input.

#### [SEC-002] No Input Size or Nesting-Depth Limits — OOM / Stack Exhaustion (Medium)
- **Category**: Resource Exhaustion / Denial of Service
- **OWASP Reference**: LLM10 / ASI05
- **Location**: `decode.go:109` (`io.ReadAll`), `lex.go:69-76` (lexer structure), `lex.go:560-572` (recursive lexer states)
- **Confidence**: Medium
- **Issue**: The decoder reads the entire TOML input into memory via `io.ReadAll(dec.r)` before parsing, and the parser builds a complete `map[string]any` of the document. Additionally, the lexer uses mutually recursive state functions (`lexValue` → `lexArrayValue` → `lexValue`) to handle nested arrays and inline tables, with each nesting level consuming a stack frame. A crafted TOML file with either (a) millions of keys causing OOM, or (b) extremely deep nesting (e.g., `[[[[[[...]]]]]]` repeated thousands of times) causing stack overflow, can crash the process.
- **Attack Path**:
  1. Attacker provides a TOML file with extremely deep array/inline-table nesting (e.g., ~10K+ levels).
  2. Lexer recursively calls `lexValue` → `lexArrayValue` → `lexValue` for each nesting level.
  3. Go runtime stack overflows → process crashes.
- **Attacker-Controlled**: Yes — attacker controls file content.
- **Guard/Mitigation Present**: None. No recursion depth limit in lexer; no input size limit in decoder; no key-count limit in parser.
- **Residual Exploitability**: DoS only. The input size must be at least N bytes for N nesting levels (each level adds ~2 bytes `[[` or `[{`), so a stack overflow at ~10K+ frames requires only ~20KB input — easily achievable.
- **Evidence**:
  ```go
  // decode.go:109 — reads entire input
  data, err := io.ReadAll(dec.r)

  // lex.go:560-572 — recursive state-chain for arrays
  func lexValue(lx *lexer) stateFn {
      ...
      case '[':
          return lexArrayValue  // recurses
  }
  func lexArrayValue(lx *lexer) stateFn {
      ...
      lx.push(lexArrayValueEnd)
      return lexValue  // recurses back
  }
  ```
- **Remediation**: Wrap the reader in `io.LimitReader` before passing to the decoder (caller-side). For deep nesting, the caller should set a reasonable limit on input size; even 1-10MB is adequate for config files. The library itself would benefit from an internal recursion-depth guard in the lexer.

#### [SEC-003] `indirect()` Follows Cyclic Pointer Chains Indefinitely (Medium)
- **Category**: Resource Exhaustion
- **OWASP Reference**: ASI05
- **Location**: `decode.go:380-399`
- **Confidence**: Medium
- **Issue**: The `indirect()` function dereferences pointers recursively, allocating new values for nil pointers along the way. The library's own documentation acknowledges: "This decoder does not handle cyclic types. Decode will not terminate if a cyclic type is passed." If a Go struct type contains a pointer to itself (e.g., `type Node struct { Next *Node }` with a cycle), the function loops forever, allocating until the heap is exhausted.
- **Attack Path**:
  1. Application defines a Go type with a self-referential pointer and passes it as the decode target.
  2. Decoder calls `indirect()` which follows the pointer chain.
  3. Infinite loop → OOM crash.
- **Attacker-Controlled**: No — the Go types are defined at compile time by the application, not by TOML input. An attacker cannot create cyclic types via TOML data.
- **Guard/Mitigation Present**: None. The docs explicitly state this is unsupported.
- **Residual Exploitability**: Near-zero for Reasonix. Would require the application to define and pass a cyclic type, which is unusual in practice.
- **Evidence** (from doc.go and decode.go):
  ```go
  // decode.go:380-399
  func indirect(v reflect.Value) reflect.Value {
      if v.Kind() != reflect.Ptr { ... return v }
      if v.IsNil() { v.Set(reflect.New(v.Type().Elem())) }
      return indirect(reflect.Indirect(v))  // infinite on cycles
  }
  ```
- **Remediation**: Add a visited-pointer set or depth limit (e.g., 100 levels) in `indirect()`. Low priority given the near-zero attack surface.

#### [SEC-004] `ErrorWithPosition()` Exposes Full TOML Input in Error Messages (Low)
- **Category**: Information Disclosure
- **OWASP Reference**: LLM02
- **Location**: `error.go:100-128`
- **Confidence**: Low
- **Issue**: `ParseError.ErrorWithPosition()` and `ErrorWithUsage()` include the entire TOML input file as context in their error strings. If the application logs or displays TOML parse errors to end users (or in a web response), the full config file content — potentially containing secrets, API keys, database credentials — would be leaked.
- **Attacker-Controlled**: No — attacker doesn't control the error path, but the error message content is derived from the (potentially sensitive) config file.
- **Guard/Mitigation Present**: None at the library level. The `Error()` method (which is what `fmt.Errorf`/`%s` typically calls) only includes the message and line number, NOT the full input. `ErrorWithPosition()` must be called explicitly.
- **Residual Exploitability**: Low. The standard `Error()` method is safe. Only code that explicitly calls `ErrorWithPosition()` or `ErrorWithUsage()` would leak. Reasonix should ensure it only uses `.Error()` in user-facing error paths.
- **Remediation**: (Caller-side) Do not call `ErrorWithPosition()` or `ErrorWithUsage()` when surfacing errors to users or external systems; use `.Error()` only. Library-side could redact values in the displayed context, but that's beyond this review's scope.

#### [SEC-005] `DecodeFile()` Has No Path Sanitization (Low)
- **Category**: Path Traversal
- **OWASP Reference**: ASI05
- **Location**: `decode.go:46-52`
- **Confidence**: Low
- **Issue**: `DecodeFile(path string, v any)` calls `os.Open(path)` directly with no path sanitization. If the `path` argument is ever derived from user input, an attacker could read arbitrary files on the filesystem (e.g., `../../etc/passwd`).
- **Attacker-Controlled**: Depends on caller. In Reasonix's normal usage, config file paths are hardcoded constants, so attacker control is "No". Mitigated by the fact that this is a config file parser, not a user-file upload handler.
- **Guard/Mitigation Present**: None at the library level.
- **Residual Exploitability**: Near-zero for Reasonix, assuming config paths are constants.
- **Remediation**: (Caller-side) Always use hardcoded, absolute paths with `DecodeFile()`. Never pass user-supplied paths.

#### [SEC-006] Lexer Channel Buffer Size of 10 May Lose Items on Panic (Low)
- **Category**: Data Integrity (edge case)
- **OWASP Reference**: ASI05
- **Location**: `lex.go:82`
- **Confidence**: Low
- **Issue**: The lexer's `items` channel is created with a buffer of 10 (`make(chan item, 10)`). When the parser triggers a `panicErr()` (which is caught by the recover handler), up to 10 already-emitted-but-unconsumed items remain in the channel buffer. The recover handler returns the `ParseError`, but the channel buffer is leaked. Since the lexer runs synchronously (not as a goroutine), this is not a goroutine leak, but it does mean items emitted during error recovery may be discarded.
- **Attacker-Controlled**: No.
- **Residual Exploitability**: None — this is a correctness issue, not a security one. It only affects error paths.
- **Remediation**: None needed. This is a defensive note; the synchronous lexer design means the buffer is effectively just a queue and the process terminates after error.

---

### Assessment
**Safe to use** for its intended purpose as a local configuration-file parser in Reasonix. The library is well-structured, uses no unsafe deserialization primitives (no `eval`, no `pickle`-equivalent, no shell execution), properly escapes strings in both decode and encode paths, and correctly validates UTF-8 input. The three MEDIUM findings are all denial-of-service-class issues that are mitigated by the fact that Reasonix loads config from trusted local files, not from untrusted network input.

No high-confidence vulnerabilities related to injection, authentication bypass, secret exposure, or unsafe deserialization were found.

### Recommendation
Wrap the reader with `io.LimitReader` when calling `NewDecoder()` to bound input size (e.g., 10 MB for config files). Use `toml.Decode()` (the string variant) or `toml.Unmarshal()` (the `[]byte` variant) for small in-memory configs; only use `NewDecoder(r).Decode(v)` for file-based input, and bound the reader.

---

## Integration Contract

### Input bounds
- **Accepted**: `io.Reader` (via `NewDecoder`), `string` (via `Decode`), `[]byte` (via `Unmarshal`), `os.File` path (via `DecodeFile`), `fs.FS` + path (via `DecodeFS`).
- **Rejected**: Non-UTF-8 input (returned as `errLexUTF8`), NULL bytes, UTF-16 BOM (stripped with warning), control characters other than `\t`, `\r`, `\n`.
- **Max safe size**: **Not bounded by the library.** Caller must limit. For config files, 1–10 MB is recommended. The entire input is read into memory before parsing (`io.ReadAll`), and the parse tree is fully materialized as `map[string]any`.

### Output shape
- **Return type**: `MetaData` + `error` (for `Decode`/`DecodeFile`/`DecodeFS`/`Decoder.Decode`), `error` only (for `Unmarshal`).
- **Decoded value**: Populated into the caller-provided pointer `v`. Supports Go structs (with `toml:"name"` tags), `map[string]T`, `any` (empty interface), and types implementing `Unmarshaler` or `encoding.TextUnmarshaler`.
- **Type mapping**: TOML integers → `int64` (or narrower signed/unsigned ints with range checks). TOML floats → `float64` (or `float32`). TOML booleans → `bool`. TOML strings → `string`. TOML datetimes → `time.Time` (with timezone handling). TOML arrays → Go slices/arrays. TOML tables → Go structs/maps. TOML array-of-tables → `[]struct`/`[]map[string]T`.
- **Nil guarantees**: `MetaData` is always a valid zero-value on error. On success, `MetaData.Keys()`, `MetaData.IsDefined()`, `MetaData.Type()`, and `MetaData.Undecoded()` are populated.
- **Primitive**: TOML values can be stored as `Primitive` for lazy decoding via `MetaData.PrimitiveDecode()`.

### Side effects
- **I/O**: `DecodeFile` opens and reads a file from the local filesystem. `DecodeFS` reads from the provided `fs.FS`. `NewDecoder` reads from the provided `io.Reader`. `Decode`/`Unmarshal` perform no I/O.
- **Allocations**: The full TOML parse tree (`map[string]any` with string/int64/float64/bool/time.Time/[]any leaves) is allocated in memory. The decoded Go value is populated via reflection — additional allocations depend on the target type. `indirect()` allocates new values for nil pointers encountered in the target.
- **Global state**: **None.** The parser and decoder are purely functional with no mutable global state. (`internal/tz.go` reads `time.Now().Zone()` at init time to set package-level `LocalDatetime`/`LocalDate`/`LocalTime` timezone variables, which can be changed by the caller before decoding.)
- **Goroutine safety**: Safe for concurrent use. The decoder struct holds no mutable shared state. The encoder (`Encoder`) writes to a `bufio.Writer` and is safe as long as the writer is not shared without synchronization.

### Error modes
- **Returned errors**: All parse/type errors are returned as `ParseError` (or `*ParseError`). Common errors: duplicate keys, type mismatches, out-of-range integers/floats, invalid datetime formats, invalid escape sequences, control characters in input, invalid UTF-8, newlines in inline tables, newlines in single-line strings.
- **Panics**: The parser uses `panic` internally for error propagation (caught by `recover()` in `parse()`). However, `p.bug()` panics with a plain `string` and is **not caught** — see SEC-001. The encoder uses `encPanic()` which panics with `tomlEncodeError` (caught by `recover()` in `safeEncode()`).
- **Nil pointer decode**: `Decode(nil)` returns an error. Decoding into a nil map/slice is handled (map/slice is initialized). Nil struct fields are skipped.

### Resource bounds
- **Memory**: O(n) where n is input size. The entire input is buffered in memory, and the parse tree (`map[string]any`) is roughly proportional to input size. Key metadata (positions, types) adds additional fixed overhead per key.
- **CPU**: O(n) lexing + O(n) parsing + O(n) reflection-based decoding. Parsing is single-pass; no backtracking beyond individual token lookahead.
- **Stack**: Lexer uses recursive state functions; nesting depth equals TOML structure depth. **Unbounded** — see SEC-002. Parser uses bounded recursion proportional to key depth.
- **Termination**: Terminates for all valid TOML input. Does not terminate for cyclic Go types passed as decode targets (documented limitation). Terminates with error for all invalid TOML input encountered so far.

### Explicit non-guarantees
- **Does NOT limit input size or nesting depth** — caller must wrap `io.Reader` with `io.LimitReader` or impose a config-file size limit.
- **Does NOT handle cyclic Go types** — `indirect()` will loop forever dereferencing pointer cycles. Caller must ensure decode target types are acyclic.
- **Does NOT sanitize file paths** — `DecodeFile()` passes the path directly to `os.Open()`. Caller must not pass user-controlled paths.
- **Does NOT redact sensitive values in error messages** — `ErrorWithPosition()` and `ErrorWithUsage()` include the full TOML input. Caller must use `.Error()` only in user-facing error paths.
- **Does NOT validate TOML key semantics beyond syntax** — duplicate keys are caught, but semantic constraints (e.g., whether a key SHOULD exist) are the caller's responsibility via `MetaData.IsDefined()` and `MetaData.Undecoded()`.
- **Does NOT guarantee key order stability across versions** — `MetaData.Keys()` returns keys in document order, but this is not a contractual guarantee (the internal representation may change).
- **Does NOT prevent integer overflow in all cases** — `int64` is the widest integer type supported by the parser. TOML integers larger than `2^63-1` cause a parse error (`errParseRange`).

### Integration examples

**CORRECT: bounded file read with error handling**
```go
import "github.com/BurntSushi/toml"

type Config struct {
    Port    int    `toml:"port"`
    DBPath  string `toml:"db_path"`
}

func loadConfig(path string) (*Config, error) {
    var cfg Config
    _, err := toml.DecodeFile(path, &cfg) // path is a hardcoded constant
    if err != nil {
        return nil, fmt.Errorf("config: %w", err) // uses .Error(), not .ErrorWithPosition()
    }
    return &cfg, nil
}
```

**CORRECT: bounded reader for untrusted-sized input**
```go
import (
    "io"
    "github.com/BurntSushi/toml"
)

func loadBounded(r io.Reader) (*Config, error) {
    limited := io.LimitReader(r, 10<<20) // 10 MB limit
    var cfg Config
    _, err := toml.NewDecoder(limited).Decode(&cfg)
    return &cfg, err
}
```

**INCORRECT: user-controlled path**
```go
// INCORRECT: path from user input — allows arbitrary file reads
func loadUserConfig(userPath string) (*Config, error) {
    var cfg Config
    _, err := toml.DecodeFile(userPath, &cfg) // DANGER: arbitrary file read
    return &cfg, err
}
```

**INCORRECT: leaking config in error messages**
```go
// INCORRECT: ErrorWithPosition() includes the full config file content
if err != nil {
    if pe, ok := err.(toml.ParseError); ok {
        log.Printf("Config error:\n%s", pe.ErrorWithPosition()) // LEAKS secrets
    }
}
```
