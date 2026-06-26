# Batch Security Review: Vendored Text-Processing Dependencies

**Date**: 2025-07-17
**Scope**: `vendor/github.com/rivo/uniseg/`, `vendor/github.com/clipperhouse/displaywidth/`, `vendor/github.com/clipperhouse/uax29/v2/`, `vendor/github.com/mattn/go-runewidth/`
**Target Type**: Source Code (vendored Go libraries)
**Risk Level**: Clean

---

## Summary

| Package | Version | Direct/Transitive | Findings | Verdict |
|---|---|---|---|---|
| `github.com/rivo/uniseg` | v0.4.7 | indirect | 0 | Clean |
| `github.com/clipperhouse/displaywidth` | v0.11.0 | indirect | 0 | Clean |
| `github.com/clipperhouse/uax29/v2` | v2.7.0 | indirect | 0 | Clean |
| `github.com/mattn/go-runewidth` | v0.0.24 | direct | 0 | Clean |

**No high-confidence vulnerabilities identified.** All four packages are pure-computation Unicode text segmentation and width-measurement libraries with no network I/O, no filesystem writes, no shell execution, no deserialization, no `unsafe` blocks, and no dynamic code evaluation.

---

## 1. `github.com/rivo/uniseg` v0.4.7

### Security Assessment

**Attack surface**: Zero. This is a purely computational library operating on immutable, compile-time-generated lookup tables.

**Key characteristics**:
- No `unsafe` usage across 22 `.go` files — confirmed by search
- No `panic()` calls — confirmed by search
- No network, filesystem, or shell access in runtime code
- No environment variable reads
- `gen_breaktest.go` and `gen_properties.go` are build-tagged `//go:build generate` — excluded from runtime builds entirely
- All exported functions are deterministic, side-effect-free transformations on string/`[]byte` input
- The only mutable global is `EastAsianAmbiguousWidth` (an `int` var that callers may set to 1 or 2), which is a documented configuration point, not a secret or sensitive state

**Data-flow summary**:
```
User string/[]byte → utf8.DecodeRune → compile-time property tables → state machine → grapheme/word/sentence/line boundaries + width int
```

No data leaves the process. No data is evaluated, executed, or deserialized.

### Integration Contract

#### Input bounds
- **Accepted**: `string` or `[]byte` of arbitrary length; rune values up to `0x10FFFF`
- **Rejected**: Malformed UTF-8 is handled gracefully — illegal bytes are treated as individual grapheme clusters with width 0 or 1 depending on position. No errors returned.
- **Max safe size**: No inherent limit; memory use is proportional to input string length (iterator holds a substring reference, not a copy)

#### Output shape
- **Return type**: `(cluster string, rest string, width int, state int)` or struct wrappers around these
- **Width range**: 0–4 per grapheme cluster (0 for combining marks/controls, 1 for standard, 2 for wide/CJK/emoji, 3 for two-em dash, 4 for three-em dash)
- **State encoding**: Packed bitfield combining grapheme (4 bits), word (5 bits), sentence (4 bits), line (8 bits), and property (remaining bits) state. Consumers must use the public `Mask*` and `Shift*` constants — never extract bits manually
- **Boundary guarantees**: Final segment always returns `LineMustBreak` per UAX #14 LB3

#### Side effects
- **I/O**: None
- **Allocations**: `FirstGraphemeCluster` / `FirstGraphemeClusterInString` — zero allocations (returns sub-slices of input). `Graphemes` iterator — allocates the iterator struct itself (~56 bytes). `Step` / `StepString` — zero allocations beyond the state machine on the stack
- **Global state**: Reads `EastAsianAmbiguousWidth` (an exported var, caller-configurable). Does not write it
- **Goroutine safety**: All functions are stateless aside from the caller-provided `state int`. Safe for concurrent use as long as each goroutine passes its own state

#### Error modes
- **Returned errors**: None — all functions handle malformed input silently
- **Panics**: None
- **Timeouts**: No blocking operations

#### Resource bounds
- **Memory**: O(1) scratch; iterator holds a string reference (no copy). Lookup tables are ~200–400 KB of compile-time constants in `.rodata`
- **CPU**: O(n) where n = input length in bytes. One `utf8.DecodeRune` per code point plus table lookups per transition. Bounded — state machine cannot loop indefinitely; each iteration advances the byte position
- **Stack**: ~200 bytes of local state per call frame. No recursion

#### Explicit non-guarantees
- Does NOT validate that grapheme cluster boundaries are semantically meaningful — follows Unicode TR29 mechanically
- Does NOT guarantee that width values match any specific terminal emulator's rendering
- Does NOT handle ANSI escape sequences — treats them as individual characters with their Unicode widths
- Does NOT limit input size — caller must bound input if processing untrusted sources

#### Integration examples

```go
// CORRECT: iterate with state tracking
state := -1
str := "Hello, 世界! 🏳️‍🌈"
for len(str) > 0 {
    var cluster string
    var width int
    cluster, str, width, state = uniseg.FirstGraphemeClusterInString(str, state)
    fmt.Printf("%q → width %d\n", cluster, width)
}

// CORRECT: simple width count
w := uniseg.StringWidth("café") // returns 4
```

```go
// INCORRECT: trying to extract bits from state without using Mask constants
lineBreak := state & 0x3 // WRONG — may overlap with other bitfields; use uniseg.MaskLine
```

---

## 2. `github.com/clipperhouse/displaywidth` v0.11.0

### Security Assessment

**Attack surface**: Zero. Pure computation building on `clipperhouse/uax29/v2/graphemes` for segmentation and a generated trie for character property lookup.

**Key characteristics**:
- No `os`, `exec`, `http`, or `net` imports — confirmed by search
- No `unsafe` usage
- No `panic()` calls (defensive `pos == start` guard silently advances by 1 byte)
- Generated trie (`trie.go`, ~98 KiB, code-generated from Unicode data) uses only array indexing — no code execution
- The defensive infinite-loop guard (`if pos == start { pos++ }`) is a safety net against a misbehaving grapheme parser; it silently skips a byte rather than looping forever
- `Options.ControlSequences` and `Options.ControlSequences8Bit` are configuration flags, not attacker-controlled. They change measurement behavior but do not introduce injection vectors — the ANSI escape sequence parser never evaluates content, only measures byte lengths

**Data-flow summary**:
```
User string/[]byte → printable ASCII fast path OR trie lookup → property enum → width lookup table → int
```

The ANSI escape parsing path: `ESC byte → sequence type switch → consume parameter/intermediate/final bytes → return byte length`. No interpretation of escape sequence content.

### Integration Contract

#### Input bounds
- **Accepted**: `string` or `[]byte` of arbitrary length; `rune` values up to `0x10FFFF`
- **Rejected**: None — all input handled. Surrogate halves (U+D800–U+DFFF) return width 0. Illegal UTF-8 bytes in the trie return width 0
- **Max safe size**: No inherent limit. The trie lookup is O(1) per code point. The grapheme iterator uses a windowed view of the input

#### Output shape
- **Return type**: `int` (display width in monospace cells) or `string`/`[]byte` (for truncation functions)
- **Width range**: 0–2 per grapheme cluster (0 for controls/combining marks, 1 for standard, 2 for wide/CJK/emoji). East Asian Ambiguous characters default to 1 but become 2 when `EastAsianWidth` is true
- **Truncation output**: When `ControlSequences` is true and truncation occurs, trailing 7-bit ANSI escape sequences are preserved in the output to prevent color bleed. These are appended as-is — never interpreted or modified

#### Side effects
- **I/O**: None
- **Allocations**: `String()`/`Bytes()` — the grapheme iterator allocates a `graphemes.Iterator` struct (~80 bytes) per non-ASCII segment. `TruncateString()` — allocates a `strings.Builder` when preserving escape sequences. No allocations on the ASCII fast path
- **Global state**: `DefaultOptions` is an exported, immutable `Options` struct. Callers typically pass their own `Options` values
- **Goroutine safety**: All functions are stateless. Safe for concurrent use

#### Error modes
- **Returned errors**: None
- **Panics**: None — the defensive `pos == start` guard prevents infinite loops silently
- **Timeouts**: No blocking operations

#### Resource bounds
- **Memory**: O(1) scratch beyond the iterator. Trie tables are ~98 KB of compile-time constants
- **CPU**: O(n) where n = input length. ASCII fast path processes printable ASCII at ~1 byte per iteration. Non-ASCII falls through to the trie+grapheme path which is O(1) per code point
- **Stack**: ~200 bytes per call frame. No recursion

#### Explicit non-guarantees
- Does NOT validate that escape sequences are well-formed beyond ECMA-48 structural rules — passes them through by byte length
- Does NOT interpret ANSI SGR parameters (colors, attributes) — only measures their display width
- Does NOT guarantee that width values match any specific terminal emulator (e.g., kitty vs iTerm2 vs Windows Terminal)
- 8-bit control sequences (0x80–0x9F) are NOT preserved during truncation even when `ControlSequences8Bit` is true — documented limitation to prevent UTF-8 boundary corruption

#### Integration examples

```go
// CORRECT: measure display width with default options
w := displaywidth.String("Hello, 世界!") // returns 10

// CORRECT: truncate with ANSI reset preservation
opts := displaywidth.Options{ControlSequences: true}
result := opts.TruncateString("\x1b[31mVery long text\x1b[0m", 10, "…")
// result preserves the trailing \x1b[0m reset
```

```go
// INCORRECT: measuring width by iterating runes
for _, r := range s {
    w += displaywidth.Rune(r) // WRONG — combining marks and ZWJ sequences need grapheme context
}
// CORRECT: use displaywidth.String(s) instead
```

---

## 3. `github.com/clipperhouse/uax29/v2` v2.7.0

### Security Assessment

**Attack surface**: Minimal. Pure grapheme cluster segmentation per UAX #29. The only non-pure-computation surface is `FromReader(r io.Reader)` which wraps `bufio.Scanner`.

**Key characteristics**:
- No `os`, `exec`, `http`, or `net` imports — confirmed by search
- No `unsafe` usage
- Three `panic()` calls in `iterator.go:92,95,99` — all defensive assertions on internal invariants:
  - `panic(err)` if `splitFunc` returns an error (line 92)
  - `panic("splitFunc returned a zero or negative advance")` (line 95)
  - `panic("splitFunc advanced beyond end of data")` (line 99)
- These panics fire only on logic bugs in the internal `splitFunc`, not on user input. The `splitFunc` implementation never returns errors on valid or invalid UTF-8 — it handles all byte sequences gracefully
- The ANSI escape sequence recognizers (`ansi.go`, `ansi8.go`) are pure byte-scanning functions with no side effects
- `FromReader(r io.Reader)` delegates to `bufio.Scanner` with a custom `SplitFunc` — standard Go pattern, no security concern

**Data-flow summary**:
```
User string/[]byte/io.Reader → byte-by-byte scan → Unicode property lookup via generated trie → UAX #29 state machine → grapheme cluster boundaries → (advance int, token []byte)
```

The ANSI escape path: `ESC/0x9B byte → switch on sequence type → consume bytes until terminator → return total byte length as single grapheme`

### Integration Contract

#### Input bounds
- **Accepted**: `string`, `[]byte`, or `io.Reader` of arbitrary length
- **Rejected**: None — all input handled. `bufio.Scanner` has a default 64 KB token limit which applies to `FromReader`; oversized tokens cause `bufio.ErrTooLong`. This is a `bufio.Scanner` default, not a library choice, and can be increased via `sc.Buffer()`
- **Max safe size**: No inherent limit for `FromString`/`FromBytes`. `FromReader` inherits `bufio.Scanner`'s 64 KB default token limit

#### Output shape
- **Return type**: `Iterator[T]` with `Value() T`, `Start() int`, `End() int` methods
- **Value type**: Same as input type — `string` for `FromString`, `[]byte` for `FromBytes`
- **Token boundaries**: Byte positions into the original data. `Value()` returns `data[Start():End()]`
- **Empty iterator**: `Next()` returns `false` immediately on empty input; `Value()` returns zero value

#### Side effects
- **I/O**: Only via `FromReader` which reads from the provided `io.Reader`
- **Allocations**: `FromString`/`FromBytes` — allocates the `Iterator` struct (~100 bytes) plus a function pointer. `FromReader` — allocates a `bufio.Scanner` (4 KB initial buffer) plus the iterator. `Next()` — no allocations; `Value()` returns a sub-slice of the underlying data
- **Global state**: None. Two package-level variables (`splitFuncString`, `splitFuncBytes`) are immutable function pointers
- **Goroutine safety**: Each `Iterator` is independent. `splitFunc` is stateless. Safe for concurrent use with separate iterators. NOT safe to share a single `Iterator` across goroutines

#### Error modes
- **Returned errors**: `FromReader` errors surfaced via `Scanner.Err()` after `Scan()` returns false
- **Panics**: Three defensive panics in `Next()` — all guard internal invariants that should never be violated by any input. If triggered, indicates a bug in `splitFunc`, not malicious input
- **Timeouts**: Only via `io.Reader` blocking. No internal timeouts

#### Resource bounds
- **Memory**: Iterator struct is ~100 bytes. `bufio.Scanner` (when using `FromReader`) allocates a 4 KB initial buffer that grows as needed. `splitFunc` is zero-allocation — processes in-place
- **CPU**: O(n) where n = input length. ASCII hot path: 1 branch + 1 increment per byte. Non-ASCII: property lookup (~3 array accesses) + state machine transition per code point. ANSI escape scanning: sequential byte walk until terminator
- **Stack**: ~500 bytes per `Next()` call. `splitFunc` is non-recursive, iterative

#### Explicit non-guarantees
- Does NOT validate that escape sequences are complete or semantically valid — recognizes them structurally only
- Does NOT handle 8-bit C1 controls (0x80–0x9F) unless `AnsiEscapeSequences8Bit` is explicitly set to true
- Does NOT limit token size in `FromString`/`FromBytes` — a single grapheme cluster can be arbitrarily long (e.g., ZWJ sequences). `FromReader` inherits `bufio.Scanner`'s 64 KB default buffer
- The `SplitFunc` (exported `bufio.SplitFunc`) is not safe for concurrent use by multiple scanners — `bufio.Scanner` serializes access

#### Integration examples

```go
// CORRECT: iterate grapheme clusters from a string
g := graphemes.FromString("Hello, 🏳️‍🌈!")
for g.Next() {
    fmt.Printf("%q at [%d:%d]\n", g.Value(), g.Start(), g.End())
}

// CORRECT: from an io.Reader with increased buffer
sc := graphemes.FromReader(r)
buf := make([]byte, 0, 1024*1024) // 1 MB
sc.Buffer(buf, 1024*1024)
for sc.Scan() {
    fmt.Println(sc.Text())
}
```

```go
// INCORRECT: passing mutable byte slice to FromBytes then modifying it
data := []byte("hello")
g := graphemes.FromBytes(data)
data[0] = 'H' // RACE — iterator holds a reference, not a copy
for g.Next() { /* may see corrupted data */ }
```

---

## 4. `github.com/mattn/go-runewidth` v0.0.24

### Security Assessment

**Attack surface**: Very low. Pure computation with one notable characteristic: environment variable reads at package initialization.

**Key characteristics**:
- Reads `os.Getenv("RUNEWIDTH_EASTASIAN")` at init time — this is a documented configuration mechanism, not a secret leak
- On POSIX: reads `LC_ALL`, `LC_CTYPE`, `LANG` to auto-detect East Asian locale — standard locale detection pattern
- On Windows: reads `WT_SESSION` to detect Windows Terminal
- No network I/O, no filesystem writes, no shell execution, no `unsafe`
- `CreateLUT()` allocates a 557,056-byte lookup table on demand — callers explicitly opt in
- Uses `sort.Slice` during init on `makeWidthTable` — this runs once at startup, bounded by table size (~2000 entries)
- Uses `graphemes.FromString` from `clipperhouse/uax29/v2` for grapheme-aware width measurement in `StringWidth` and `Truncate`

**Environment variable evaluation**: The `handleEnv()` function is called from `init()`. It reads `RUNEWIDTH_EASTASIAN` and, if empty, calls `IsEastAsian()` which reads `LC_ALL`/`LC_CTYPE`/`LANG`. These are all read-only operations. No secrets are exposed. The values only influence whether East Asian Ambiguous characters are width 1 or 2 — a cosmetic rendering choice.

### Integration Contract

#### Input bounds
- **Accepted**: `string` of arbitrary length; `rune` values up to `0x10FFFF`
- **Rejected**: `rune < 0` or `rune > 0x10FFFF` returns width 0. Nil or empty strings return width 0
- **Max safe size**: No inherent limit. `CreateLUT()` allocates a fixed ~557 KB regardless of input size

#### Output shape
- **Return type**: `int` (display width in cells) or `string` (for truncation/fill/wrap functions)
- **Width range**: 0–2 per character/grapheme. 0 for controls/combining marks/non-printable, 1 for standard, 2 for wide/CJK/emoji
- **Truncation suffix**: `Truncate(s, w, tail)` guarantees `StringWidth(result) <= w` (accounting for tail)

#### Side effects
- **I/O**: None
- **Allocations**: `StringWidth` — zero-allocation on the ASCII fast path; grapheme iterator allocation (~100 bytes) on the non-ASCII path. `Truncate` — allocates result string. `Wrap` — allocates via `strings.Builder`. `CreateLUT` — single 557 KB allocation, not freed
- **Global state**: `EastAsianWidth` and `StrictEmojiNeutral` are exported package-level vars set at init. `DefaultCondition` holds the current condition. `CreateLUT()` mutates `DefaultCondition.combinedLut`
- **Goroutine safety**: Read-only after init — safe. `CreateLUT()` is explicitly documented as NOT safe for concurrent calls with other operations

#### Error modes
- **Returned errors**: None — all input handled silently
- **Panics**: None
- **Timeouts**: No blocking operations

#### Resource bounds
- **Memory**: Without LUT: ~10 KB of compile-time tables (interval lists). With LUT: additional 557 KB. `StringWidth` — O(1) scratch beyond the grapheme iterator
- **CPU**: Without LUT: binary search over interval tables — O(log n) where n ≈ 2000 entries, effectively constant. With LUT: direct array index — O(1). Grapheme path: O(input length) per `clipperhouse/uax29`
- **Stack**: ~200 bytes per call frame. No recursion

#### Explicit non-guarantees
- Does NOT handle ANSI escape sequences — treats them as individual characters with their Unicode widths
- Does NOT guarantee width values match any specific terminal emulator
- `StringWidth` of a single-byte string does NOT call the grapheme iterator — it uses a fast path that may differ from `graphemes`-based width for edge cases like isolated combining marks
- `Truncate` uses the first non-zero-width rune in a grapheme cluster as the cluster's width — this is an approximation that may differ from `displaywidth`'s more precise per-grapheme measurement

#### Integration examples

```go
// CORRECT: measure display width
w := runewidth.StringWidth("Hello, 世界!") // returns 10

// CORRECT: truncate with ellipsis
result := runewidth.Truncate("Very long text here", 10, "…")

// CORRECT: enable LUT for high-throughput scenarios (not concurrency-safe during creation)
runewidth.CreateLUT()
// ... many StringWidth calls ...
```

```go
// INCORRECT: relying on RuneWidth for grapheme-cluster-aware width
for _, r := range "é" { // 'é' may be 1 or 2 runes (e + combining acute)
    w += runewidth.RuneWidth(r) // WRONG for combining sequences
}
// CORRECT: use StringWidth which handles grapheme clusters
```

---

## Cross-Dependency Chain Analysis

The dependency chain is linear with no circular references:

```
mattn/go-runewidth ──→ clipperhouse/uax29/v2 (graphemes)
clipperhouse/displaywidth ──→ clipperhouse/uax29/v2 (graphemes)
rivo/uniseg (standalone, no dependencies beyond stdlib)
```

### Chain: `go-runewidth` + `uax29/graphemes`

`go-runewidth.StringWidth()` delegates to `graphemes.FromString()` for non-ASCII input. The grapheme iterator returns byte slices; `go-runewidth` then iterates runes within each grapheme and takes the width of the first non-zero-width rune as the cluster's width.

**Trust boundary**: `go-runewidth` receives grapheme boundaries from `uax29/graphemes` and width values from its own internal tables. The composition is read-only — no data flows from one dependency through a dangerous sink in the other. No trust assumptions are violated.

**Verdict**: Safe — no exploitable mismatch.

### Chain: `displaywidth` + `uax29/graphemes`

`displaywidth.String()` delegates to `graphemes.FromString()` for non-ASCII input, then measures width with its own generated trie (`trie.go`). The ANSI escape options are passed through to the grapheme iterator.

**Trust boundary**: `displaywidth` sets `g.AnsiEscapeSequences` and `g.AnsiEscapeSequences8Bit` on the iterator before iteration. When these flags are true, the grapheme iterator treats escape sequences as single tokens. `displaywidth` then measures the width of those tokens via `graphemeWidth()`, which correctly returns 0 for sequences starting with ESC (0x1B) or C1 controls (0x80–0x9F).

**Verdict**: Safe — the width measurement correctly accounts for ANSI escapes as zero-width. No trust boundary violation.

### Cross-dependency attack chains: None identified

No data flows between `rivo/uniseg` and any other package. No package's output feeds into another package's dangerous sink. All inter-package data flow is string/`[]byte` → grapheme segmentation → width integer, with no mutable shared state.

---

## Overall Verdict

**All four packages are safe to use.** They are pure-computation Unicode text processing libraries with no I/O, no code execution, no deserialization, and no mutable shared state beyond documented configuration variables. The one environment-variable read (`go-runewidth` reading locale variables at init) is a standard pattern for locale detection and does not expose secrets or introduce injection vectors.
