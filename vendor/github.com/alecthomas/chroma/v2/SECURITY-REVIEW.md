# Security Review: `github.com/alecthomas/chroma/v2`

- **Target Type**: Vendored dependency (source code — syntax highlighting library)
- **Review Date**: 2026-07-02
- **Version**: v2.27.0 (vendored)
- **Modes Used**: A (read-only static analysis)
- **Findings**: 0 Critical, 0 High, 3 Medium
- **Risk Level**: Low
- **Rule of Two Violation**: No — chroma is a pure data-transformation library. It processes untrusted input (code text) and produces formatted output, but has no system access, no network access, no file writes, and no credential access. The blast radius is bounded to the output stream and memory.
- **Confidence**: High
- **Dependencies reviewed**: 1 direct transitive (`github.com/dlclark/regexp2/v2`)
- **CVEs checked**: 1 database queried (GitHub Advisory DB); 0 CVEs found applicable
- **Release notes analyzed**: 0 (single vendored version)
- **Cross-dependency chains identified**: 0

---

## Architecture Overview

Chroma is a syntax highlighting engine ported from Pygments. It takes untrusted source code text, tokenizes it via a regex-based state machine, and formats tokens into HTML, ANSI, SVG, or JSON output.

```
Untrusted Input (code text)
    ↓
RegexLexer.Tokenise(text)  →  Iterator of Tokens
    ↓
Formatter.Format(w, style, iterator)  →  formatted output to io.Writer
```

**Attack surface entry points:**
1. `Tokenise(text)` — arbitrary text (code to highlight) from LLM outputs
2. `Unmarshal(xml)` — XML lexer definitions (trusted: only called with embedded FS files or developer-authored config)
3. `NewXMLStyle(r)` — XML style definitions (trusted: only called with bundled style files)

The critical data path is `Tokenise(text)` → regex matching → token emission → formatter output.

---

## Findings

### [SEC-001] Unbounded Token Production — Memory Exhaustion (Medium)

- **Category**: Resource Bounds / Unbounded Consumption (OWASP LLM10)
- **Location**: `regexp.go:194-228` (`LexerState.Iterator`), `iterator.go:16-21` (`Iterator.Tokens`)
- **Confidence**: Medium
- **Issue**: There is no limit on the size of input text passed to `Tokenise()`, nor on the number of tokens produced. The `LexerState.Iterator()` method iterates character-by-character until the entire `[]rune(text)` is consumed. For extremely large inputs or inputs that trigger many short token matches, this can produce an unbounded number of `Token` structs, each carrying a `Value string`. Callers that collect all tokens via `Iterator.Tokens()` or `chroma.Tokenise()` (which builds a `[]Token` slice) will allocate memory proportional to the token count.

- **Attack Path**:
  1. Attacker provides a very large code block (e.g., 100 MB of repetitive source code).
  2. Chroma's lexer tokenizes the entire input, producing millions of tokens.
  3. Consumer collects all tokens into a slice → OOM.

- **Attacker-Controlled**: Yes — the text to highlight is controlled by the LLM output or diff content.
- **Guard/Mitigation Present**: The Reasonix consumer applies `clampPlain()` and line-level truncation in `diffview.go`, but these operate on already-tokenized output lines, not on the input to `Tokenise()`. No upstream input size limit exists in chroma itself.
- **Residual Exploitability**: Low for Reasonix — code blocks in chat TUI are naturally bounded by context window limits and `clampPlain` per-line truncation. Higher for other consumers (e.g., batch processing of arbitrary files).
- **Evidence**: In `regexp.go:194-228`, the loop `for l.Pos < end && len(l.Stack) > 0` runs until all input is consumed, emitting a token for every match (including 1-character Error tokens for unmatched characters).
- **Remediation**: Add an optional `MaxTokens` or `MaxInputSize` to `TokeniseOptions` and short-circuit the iterator when exceeded. In Reasonix specifically, consider wrapping input with a size check before calling `Tokenise()`.

---

### [SEC-002] Panic-Based Error Propagation — Crash Risk for Custom Formatters (Medium)

- **Category**: Error Handling / Crash
- **Location**: `regexp.go:199` (`panic("unknown state " + l.State)`), `regexp.go:219` (`panic(err)` on mutator error)
- **Confidence**: Medium
- **Issue**: The `LexerState.Iterator()` method uses `panic()` for error propagation rather than returning errors. The design comment in `iterator.go:6` states: "If an error occurs within an Iterator, it may propagate this in a panic. Formatters should recover." The `FormatterFunc` wrapper (`formatter.go:18-23`) does recover from panics, and the `RecoveringFormatter` wrapper (`formatter.go:27-35`) provides the same. However, a custom formatter that does not implement panic recovery will crash the entire process if the lexer encounters an unexpected state or a mutator returns an error.

- **Attack Path**:
  1. A lexer rule definition (from embedded XML) contains a state transition error or a mutator returns an error.
  2. Lexer panics during tokenization.
  3. If the formatter does not recover, the process crashes.
  4. This is NOT attacker-controlled (the lexer rules are trusted), but represents a latent crash risk.

- **Attacker-Controlled**: No — cannot be triggered by input text alone. Requires a bug in lexer rule definitions.
- **Guard/Mitigation Present**: All built-in formatters use `FormatterFunc` (which recovers). Reasonix uses `formatters.Get("terminal256")` which wraps with `FormatterFunc`. The vendored `FormatterFunc.Format()` at `formatter.go:18-23` recovers panics and converts them to returned errors.
- **Residual Exploitability**: None for Reasonix — the formatter in use recovers. Medium for consumers writing custom formatters without reading the panic-recovery requirement documented at `iterator.go:6`.
- **Evidence**: `regexp.go:199`: `panic("unknown state " + l.State)` — if a lexer state machine references a non-existent state, the process panics. `regexp.go:219`: `panic(err)` — if a mutator returns an error during tokenization.
- **Remediation**: Already mitigated for Reasonix via `FormatterFunc` recovery. For the upstream library: consider replacing panics with an error-returning iterator pattern (breaking API change) or documenting the recovery requirement more prominently.

---

### [SEC-003] Debug Tracing Writes Source Code to stderr (Medium)

- **Category**: Information Disclosure
- **Location**: `regexp.go:161-172` (tracing block)
- **Confidence**: Medium
- **Issue**: When `SetTracing(true)` is called on a `RegexLexer`, every tokenization emits a JSON trace line to `os.Stderr` containing the lexer name, state, rule index, regex pattern, byte position, match length, and elapsed time. While this does not include the matched text directly, the position + length + lexer state can be correlated to reconstruct parts of the source code being highlighted. If tracing is accidentally enabled in production, LLM-generated code content could leak to stderr (and from there to container logs, log aggregators, etc.).

- **Attack Path**:
  1. Tracing is enabled on a lexer (via `SetTracing(true)`, `Trace(true)`, or the `Trace` method).
  2. Each tokenization produces a JSON trace line written to `os.Stderr`.
  3. Stderr is captured by the process supervisor, container runtime, or log aggregator.
  4. Source code content (from LLM output) is inferable from trace positions and lengths.

- **Attacker-Controlled**: No — tracing is opt-in and must be explicitly enabled by the developer.
- **Guard/Mitigation Present**: Tracing defaults to `false`. Reasonix does not enable tracing. The trace struct encodes position and length but not the actual matched text.
- **Residual Exploitability**: None for Reasonix (tracing not enabled). Low risk even if accidentally enabled — only positions are leaked, not full text.
- **Evidence**: `regexp.go:161-172`: `_ = trace.Encode(Trace{Lexer: l.Lexer.config.Name, State: l.State, Rule: ruleIndex, Pattern: rule.Pattern, Pos: l.Pos, Length: length, Elapsed: ...})` — writes to `os.Stderr` via `json.NewEncoder(os.Stderr)` at `regexp.go:153`.
- **Remediation**: If tracing is ever needed for debugging, ensure it is gated behind a build tag or runtime flag that cannot be accidentally enabled in production.

---

## Defenses Confirmed

| Defense | Status | Evidence |
|---------|--------|----------|
| ReDoS timeout per regex | ✅ Present | `regexp.go:283`: `rule.Regexp.MatchTimeout = time.Millisecond * 250` |
| HTML output escaping | ✅ Present | `formatters/html/html.go:323`: `html.EscapeString(token.String())` |
| SVG output escaping | ✅ Present | Uses `html.EscapeString()` on attribute values |
| TTY output (ANSI) | ✅ Non-web | Terminal escape sequences; no HTML/JS injection surface |
| XXE protection | ✅ Go stdlib | Go's `encoding/xml` does not process external entities |
| No eval/exec paths | ✅ Confirmed | No `os/exec`, `eval`, `plugin.Open`, or similar found |
| No hardcoded secrets | ✅ Confirmed | `search_content` for common secret patterns returned 0 hits |
| No network I/O | ✅ Confirmed | Library has no network calls |
| Formatter panic recovery | ✅ Present | `formatter.go:18-23` (`FormatterFunc`), `formatter.go:33` (`RecoveringFormatter`) |
| Regex compilation errors caught at init | ✅ Present | `maybeCompile()` returns errors; `MustNewXMLLexer` panics at init for bad patterns |

---

## Dependency: `github.com/dlclark/regexp2/v2`

The single transitive dependency provides the regex engine with backtracking support and timeout capability. Static review confirmed:

- No `os/exec`, `net`, or filesystem access
- `unsafe` usage confined to internal byte/string conversion helpers (`helpers/indexof.go`)
- `reflect` usage in `replace.go` for `MatchEvaluator` callbacks — confined to user-provided function types, not arbitrary reflection
- No hardcoded secrets or network calls
- `MatchTimeout` defaults to `math.MaxInt64` ("forever"), but chroma explicitly sets it to 250ms

**Verdict**: Safe for this use case. The timeout mechanism is the primary security feature relied upon by chroma.

---

## Integration Contract

### Input bounds
- **Accepted**: `string` of arbitrary length (the code text to highlight)
- **Rejected**: None — empty string returns empty token stream
- **Max safe size**: No built-in limit. Callers should bound input to prevent memory exhaustion (see SEC-001). In Reasonix, code blocks are naturally bounded by LLM context window and per-line `clampPlain()`.

### Output shape
- **Return type**: `chroma.Iterator` (a `func() Token`) or `[]Token` via `chroma.Tokenise()`
- **Token struct**: `{Type: TokenType, Value: string}` where `Type` is one of the predefined `chroma.TokenType` constants
- **EOF sentinel**: `chroma.EOF` (`Token{}`) marks end of stream
- **Error token**: Unmatched characters emit `Token{Error, "…"}` — the lexer never fails on bad input, it emits Error tokens for unrecognized sequences
- **Empty tokens**: The `Coalesce` wrapper filters zero-length tokens; direct `Tokenise` may emit them
- **Nil guarantees**: `Tokenise()` returns `nil, error` on failure (e.g., regex compilation error). Successful tokenization always returns a valid iterator.

### Side effects
- **I/O**: None from the library itself. The `trace` feature writes JSON to `os.Stderr` when explicitly enabled (off by default).
- **Allocations**: Proportional to input size and token count. Each token allocates a `Token` struct and a `string` for its value. Regex matching allocates match group slices.
- **Global state**: Lexer registry (`LexerRegistry`) and formatter registry (`formatters.Register`) are mutable global maps. `RegexLexer` uses a `sync.Once` for deferred regex compilation.
- **Goroutine safety**: `RegexLexer.Tokenise()` is safe for concurrent use (each call creates its own `LexerState`). The deferred compilation uses `sync.Once`. The style/formatter caches use `sync.Mutex`.

### Error modes
- **Returned errors**: `Tokenise()` returns errors from regex compilation failures and I/O errors from formatter `Write()`. Formatter errors are returned, not panicked (due to `FormatterFunc` recovery).
- **Panics**: Two panic sites in `LexerState.Iterator()`:
  - `regexp.go:199`: `panic("unknown state " + l.State)` — if lexer rules reference a non-existent state
  - `regexp.go:219`: `panic(err)` — if a mutator returns an error
  - Both are caught by `FormatterFunc`/`RecoveringFormatter` and converted to returned errors
- **Timeouts**: Regex matching times out after 250ms per pattern. A timeout causes the match to fail (treated as no match), and the lexer emits the character as an `Error` token.

### Resource bounds
- **Memory**: O(tokens) — each token allocates. No explicit limit. The `Coalesce` wrapper caps single-token concatenation at 8192 bytes.
- **CPU**: O(input) in normal cases. Worst-case: O(input × rules) if every character fails to match and falls through to Error token. Regex timeouts (250ms) bound per-pattern worst case but don't bound total CPU time across patterns.
- **Stack**: Regex matching uses the regexp2 VM, which has its own stack. No unbounded Go stack recursion observed.

### Explicit non-guarantees
- **Does NOT limit input size** — caller must bound input to avoid memory exhaustion.
- **Does NOT guarantee token count is proportional to input length** — pathological inputs could produce many small tokens.
- **Does NOT sanitize ANSI escape sequences in input** — if source code contains literal escape sequences, they pass through to TTY output. This is by design for syntax highlighting but means "malicious-looking" escape sequences in source code will be rendered as-is in terminal output.
- **Does NOT guarantee stable output across versions** — lexer rule changes between chroma versions may produce different token sequences for identical input.
- **Does NOT validate that regex patterns are safe** — patterns from embedded XML are trusted. The `Unmarshal()` function accepts arbitrary XML and will compile whatever regex patterns it contains.
- **The ClassPrefix option is NOT sanitized for CSS injection** — `formatters/html/html.go`'s `class()` function at line 311 concatenates `f.prefix + cls` directly into a CSS class attribute. If the caller passes a user-controlled prefix, CSS injection is possible. Callers must ensure `ClassPrefix` is developer-controlled, not user-controlled.

### Integration examples (for Reasonix consumers)

```go
// CORRECT: bounded input, known-safe formatter, no tracing
lexer := lexers.Match(path)
if lexer == nil {
    lexer = lexers.Fallback
}
it, err := lexer.Tokenise(nil, code) // code is a single line, already clamped
if err != nil {
    return code // fall back to plain text
}
var b strings.Builder
if err := formatters.Get("terminal256").Format(&b, styles.Get("github-dark"), it); err != nil {
    return code
}
return b.String()
```

```go
// INCORRECT: no input size bound, collects all tokens into memory
it, _ := lexer.Tokenise(nil, hugeFileContents) // hugeFileContents could be 500MB
tokens := it.Tokens() // allocates millions of Token structs → OOM
```

```go
// INCORRECT: user-controlled ClassPrefix enables CSS injection
prefix := r.URL.Query().Get("css_prefix") // attacker controls this
f := html.New(html.ClassPrefix(prefix))    // prefix concatenated into class="..."
f.Format(w, style, it)
// Attacker sets css_prefix to: "x\" onclick=\"alert(1)\" id=\""
```

---

## Assessment

**Safe to use** for the Reasonix chat TUI code-block rendering use case, with the noted caveats:

1. Input text is naturally bounded (LLM context window, per-line `clampPlain()`).
2. The `terminal256` formatter is used (no HTML injection surface).
3. Tracing is not enabled.
4. The formatter in use recovers from lexer panics.
5. `ClassPrefix` is not user-controlled.
