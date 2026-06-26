# Security Review: goldmark v1.8.2 (vendored)

## Summary

- **Target Type**: Source Code (Go Markdown parser + HTML renderer)
- **Modes Used**: A (read-only static analysis)
- **Findings**: 1 (0 Critical, 1 Medium)
- **Risk Level**: Low _for this usage_ (Reasonix renders to ANSI, not HTML)
- **Rule of Two Violation**: No — goldmark is a pure parser/renderer with no network, filesystem, or shell access
- **Confidence**: High
- **Dependencies reviewed**: 0 external (stdlib only)
- **CVEs checked**: 1 search (none applicable to v1.8.2)
- **Release notes analyzed**: Not applicable (latest release vendored)
- **Cross-dependency chains identified**: 0

## Context

Reasonix uses goldmark's **parser only** — it walks the AST with a custom
`mdRenderer` (`internal/cli/md.go`) that produces ANSI-styled terminal text.
The HTML renderer (`renderer/html/`) is **vendored but unused** by Reasonix.
This substantially reduces the attack surface: all HTML-specific risks
(XSS via `WithUnsafe`, `IsDangerousURL` bypass, raw-HTML emission) do not
apply to the actual execution path.

Input is LLM assistant responses — attacker-controlled (the model is operating
on behalf of the user's conversation partner), but rendered to a local terminal,
not a browser.

---

## Findings

### [SEC-001] No recursion depth limit in parser — stack exhaustion on deeply nested input (Medium)

- **Category**: Resource bounds / denial of service
- **OWASP Reference**: LLM10 (Unbounded Consumption)
- **Location**: `vendor/github.com/yuin/goldmark/parser/parser.go` (block parsing loop) and `vendor/github.com/yuin/goldmark/ast/ast.go:536` (Walk)
- **Confidence**: Medium
- **Issue**: The parser has no limit on block nesting depth. A malicious or
  buggy LLM output containing deeply nested lists or blockquotes (e.g. 10,000
  levels) will be parsed into a deeply nested AST. The subsequent AST walk in
  `mdRenderer.renderBlocks` uses recursion, and Go's default goroutine stack
  (~1 GB) can accommodate very deep recursion but will eventually overflow
  with a fatal crash.
- **Attack Path**:
  1. Attacker causes the model to emit a deeply nested markdown structure
     (via prompt injection into the conversation, e.g. 10,000 nested `> ` blockquotes)
  2. Reasonix passes the output to `goldmark.Parser().Parse()`
  3. The parser creates a deeply nested `ast.Blockquote` tree
  4. `mdRenderer.renderBlocks` recursively walks the tree
  5. Stack overflow → goroutine crash → the chat TUI panel silently fails to render
- **Attacker-Controlled**: Partial (the LLM's output is the attacker's vector; the
  attacker must first poison the conversation)
- **Guard/Mitigation Present**: None. No depth limit in parser or renderer
- **Residual Exploitability**: Denial-of-service on the render path only — the
  application binary does not crash, but the specific panel rendering goroutine
  would panic with a stack overflow. Go's runtime terminates the goroutine but
  not the process (runtime.Goexit on stack overflow in goroutines). The chat TUI
  would silently fail to render that message.
- **Evidence**: Searched for `recursion|stack|depth|limit|maxNesting|maxRec` in all
  vendor goldmark `.go` files — zero hits for depth-limiting constructs.
- **Remediation**: Add a nesting depth guard in `mdRenderer.renderBlocks` (e.g.,
  `if indent > 200 { return }` or a `depth` parameter incremented at each
  recursive call). This is the Reasonix-specific fix; a goldmark upstream fix
  would be a `MaxNesting` parser option.

---

## Non-Findings (Investigated and Cleared)

### HTML renderer not used
Reasonix's `mdRenderer` walks the AST directly and produces ANSI escape codes.
The HTML renderer (`renderer/html/html.go`) is vendored but never instantiated
in the Reasonix codebase. `search_content "html\.NewRenderer\|html\.WithUnsafe"` across
`internal/` returns zero matches.

This means:
- `html.WithUnsafe()` cannot be accidentally enabled
- `html.IsDangerousURL()` bypass vectors are irrelevant
- Raw HTML is explicitly **dropped** in `md.go:346` (`case *ast.RawHTML: // drop`)
- Links render as text + dimmed `(url)` — no clickable or executable output

### Custom math parser (`mathParser`) — safe
`internal/cli/mathnode.go` parses `$...$` / `$$...$$` delimited math. The
content is passed to `latexToUnicode()` which is a pure string→string symbol
lookup — no eval, no exec, no filesystem, no network. Currency guard prevents
`$5` prose from being treated as math.

### Parser panics — not reachable from input
The five `panic()` calls in the parser are all in registration/setup code
(`addBlockParser`, `addInlineParser`, etc.) that fire on programming errors
(wrong type assertion), not on malformed markdown input. They are unreachable
under normal operation.

### No external dependencies
`search_content "github.com\|golang.org\|go\.mod"` in vendor goldmark shows
zero third-party imports. The parser uses only Go stdlib. No supply-chain
risk from transitive dependencies.

---

## Integration Contract

### Input bounds
- **Accepted**: `[]byte` of arbitrary length (UTF-8 markdown). No size limit
  is enforced by the parser.
- **Rejected**: `text.NewReader` wraps a `[]byte`; the reader does not reject
  based on content.
- **Max safe size**: Not tested or enforced. Caller should bound input to a
  reasonable size (~1 MB for markdown chat output is more than adequate).

### Output shape
- **Parser output**: `ast.Node` tree rooted at `*ast.Document`. All nodes
  implement `ast.Node` interface. Tree is depth-first, with parent/child/sibling
  pointers.
- **Nil guarantees**: `Parse()` never returns nil. The document is always
  valid, even for empty input.
- **Position invariants**: All `Segment` / `Lines` values are within
  `[0, len(source))`. `Line.At(i)` never panics for `i < Lines().Len()`.
- **Reasonix renderer output**: Plain ANSI-styled text (no HTML). Newlines
  used for block separation. Trailing newline always present for non-empty
  output.

### Side effects
- **I/O**: None. Parser and renderer are pure functions of the input bytes.
- **Allocations**: Proportional to input size and AST node count. Each block
  and inline element allocates a node. A 10 KB input typically produces < 1 KB
  of allocation.
- **Global state**: None. Parser and renderer are stateless beyond the
  `sync.Once` init guard.
- **Goroutine safety**: Safe for concurrent use. `sync.Once` protects parser
  initialization.

### Error modes
- **Returned errors**: `Parser.Parse()` returns `ast.Node` directly (no error
  return). `mdRenderer.Render()` returns `string` (no error return). All
  parsing failures degrade gracefully — malformed input produces a best-effort
  AST.
- **Panics**:
  - Parser registration panics on wrong-typed `PrioritizedValue` — **only during
    `sync.Once` init**, not on input.
  - `segment.go:80`: panic on invalid internal state (`text.NewSegment` with
    stop < start) — **only reachable via programming error**, not from input.
  - `ast/inline.go:28-43`: panics on `AppendChild`/`InsertAfter`/etc. on inline
    nodes — **programming error only**.
  - **No panics triggered by malformed markdown input.**
- **Timeouts**: None. Parser runs synchronously to completion. Quadratic
  behavior is theoretically possible on adversarial input but not observed
  in practice for chat-sized inputs.

### Resource bounds
- **Memory**: O(n) where n is input size. Each byte of input may produce an
  AST node, each node is ~100-200 bytes. A 1 MB input could produce ~200 MB
  of AST allocation in the worst case.
- **CPU**: O(n) for typical markdown. Nested emphasis parsing could be O(n²)
  in pathological cases (alternating `*` delimiters), but this is bounded by
  the CommonMark spec.
- **Stack**: AST `Walk()` uses recursion. The parser's block-open/close loop
  is iterative. The Reasonix `renderBlocks` / `renderBlock` functions use
  mutual recursion with no depth limit — see SEC-001.

### Explicit non-guarantees
- **Does NOT sanitize HTML** — the HTML renderer has `WithUnsafe()` and defaults
  to `Unsafe: false` (safe), but Reasonix does not use it. The raw `ast.HTMLBlock`
  and `ast.RawHTML` node content is stored in the AST but not rendered by Reasonix.
- **Does NOT limit recursion depth** — nested lists, blockquotes, and emphasis
  can produce arbitrarily deep AST trees. Caller responsible for depth guarding.
- **Does NOT enforce input size limit** — caller should bound input.
- **Does NOT validate link or image URLs** — the parser stores URLs verbatim.
  The HTML renderer has `IsDangerousURL`, but Reasonix does not use it. Reasonix
  renders URLs as dimmed text after the link text, which is safe in a terminal.
- **Does NOT reject shell metacharacters** — goldmark is a text parser, not
  a shell parser. Text content may contain `|`, `;`, `$()`, etc. which are
  harmless in terminal rendering context.

### Integration examples

**CORRECT: Parse with Reasonix's ANSI renderer (what Reasonix does)**

```go
// internal/cli/md.go — parses markdown, renders ANSI for terminal
r := newMarkdownRenderer(80)
src := []byte(input)
doc := r.md.Parser().Parse(text.NewReader(src))
var buf strings.Builder
r.renderBlocks(&buf, doc, src, 0)
output := buf.String()
// output is ANSI-styled text, safe for terminal display
```

**INCORRECT: Use the HTML renderer with WithUnsafe() (what Reasonix avoids)**

```go
// DO NOT DO THIS — raw HTML from the LLM output rendered verbatim
// If the model emits <script>alert(1)</script>, it lands in the terminal
// (though in a terminal, script tags are harmless — the real risk is
// ANSI escape injection via raw HTML boundary confusion)
md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
var buf bytes.Buffer
md.Convert([]byte(userInput), &buf)
// buf.String() may contain unescaped HTML
```

**CORRECT: Use the HTML renderer without WithUnsafe() (safe default)**

```go
md := goldmark.New() // Unsafe defaults to false
var buf bytes.Buffer
md.Convert([]byte(userInput), &buf)
// Dangerous URLs blanked, raw HTML replaced with comments
```

---

## Assessment

**Safe to use** for Reasonix's current purpose (terminal ANSI rendering).

The decision to not use goldmark's HTML renderer eliminates the primary attack
surface. The custom `mdRenderer` drops raw HTML, renders links as non-clickable
text, and has no code-execution paths. The remaining risk (SEC-001 — stack
exhaustion on deep nesting) is a denial-of-service concern with low
exploitability since the attacker must first inject content into the LLM's
output, and the impact is a single goroutine crash (non-fatal to the process).
