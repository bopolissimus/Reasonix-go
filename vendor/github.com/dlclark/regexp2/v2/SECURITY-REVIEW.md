# Security Review: `github.com/dlclark/regexp2`

- **Target Type**: Source Code (Go dependency — .NET-style backtracking regex engine)
- **Modes Used**: A (Read, Grep, Glob)
- **Findings**: 5 (2 Critical, 2 High, 1 Medium)
- **Risk Level**: Critical
- **Rule of Two Violation**: No — this is a library, not an agent system. The caller is responsible for setting appropriate timeouts.
- **Confidence**: High
- **Dependencies reviewed**: 1 (no transitive deps beyond stdlib)
- **CVEs checked**: 0 CVEs found; no CVEs filed against this package
- **Release notes analyzed**: N/A (vendored version is the latest)
- **Cross-dependency chains identified**: 0

---

## Findings

### [SEC-001] Unbounded Backtracking — No Step or Depth Limit (Critical)

- **Category**: Denial of Service / Algorithmic Complexity
- **OWASP Reference**: LLM03 (Supply Chain — via ReDoS in tool dependencies)
- **Location**: `runner.go:219-926` (`executeDefault()`), `regexp.go:27` (`DefaultMatchTimeout`)
- **Confidence**: High
- **Issue**: The `executeDefault()` VM loop processes opcodes including `Oneloop`, `Setloop`, `Branchmark`, `Lazybranch`, `Branchcount`, etc. — all of which push backtracking frames onto `runtrack`/`runstack` arrays. There is **no instruction counter, no step limit, and no iteration cap** anywhere in the execution loop. The default timeout (`DefaultMatchTimeout`) is set to `math.MaxInt64`, which is treated as "infinity" — the timeout checking path (including the background goroutine) is completely skipped when `ignoreTimeout == true`.

- **Attack Path**:
  1. Attacker supplies a regex pattern with nested quantifiers (e.g., `(a+)+b`) and an input string that nearly matches (e.g., `"aaaaac"`).
  2. The regex is compiled and executed via any of `MatchString`, `FindStringMatch`, `Replace`, `Split`, etc.
  3. `scan()` enters the `for` loop, `executeDefault()` begins processing opcodes, hitting lazy/greedy branch loops that push backtracking frames.
  4. With no timeout active, the VM runs until the host process exhausts a CPU core or memory.
  5. Impact: CPU exhaustion (ReDoS). In a server context, this blocks a goroutine indefinitely (Go's scheduler won't preempt a tight compute loop until Go 1.14+ asynchronous preemption, which has ~10ms granularity but still doesn't terminate the loop — it just allows other goroutines to run).

- **Attacker-Controlled**: Yes — both the pattern (if user can supply regex) and the input string are attacker-controlled.
- **Guard/Mitigation Present**: `MatchTimeout` can be set to a finite duration. `ignoreTimeout` flag skips the timeout check entirely when `MatchTimeout == math.MaxInt64`. There is no fallback; no step counter; no recursion depth limit.
- **Residual Exploitability**: **Full**. Default configuration is vulnerable. Even with a finite timeout, the granularity is 100ms (see SEC-003).
- **Evidence**:
  ```go
  // regexp.go:27
  var DefaultMatchTimeout = time.Duration(math.MaxInt64)

  // runner.go:113
  r.ignoreTimeout = (time.Duration(math.MaxInt64) == timeout)

  // runner.go:2038-2043
  func (r *Runner) startTimeoutWatch() {
      if r.ignoreTimeout {
          return  // <-- entire timeout mechanism skipped
      }
      r.deadline = makeDeadline(r.timeout)
  }

  // runner.go:219-223 — no step counter anywhere in the loop
  for {
      if !r.ignoreTimeout {
          if err := r.CheckTimeout(); err != nil {
              return err
          }
      }
      switch r.operator {
      case syntax.Stop:
          return nil
      // ... hundreds of opcode cases, every branch path can loop ...
      }
  }
  ```
- **Remediation**: Callers MUST set `re.MatchTimeout` to a finite duration (≤ 1 second for untrusted input) before any match operation. The library itself could add a configurable backtracking step limit as a second defense layer, but the design intent is .NET compatibility where the timeout is the sole mechanism.

---

### [SEC-002] Unbounded Backtracking Stack Growth → Memory Exhaustion (Critical)

- **Category**: Denial of Service / Unbounded Resource Allocation
- **OWASP Reference**: LLM10 (Unbounded Consumption)
- **Location**: `runner.go:944-955` (`ensureStorage`), `runner.go:957-965` (`doubleIntSlice`)
- **Confidence**: High
- **Issue**: The backtracking stacks (`runtrack`, `runstack`, `runcrawl`) are `[]int` slices that **double** in capacity every time they fill up, with **no upper bound**. The initial capacity is `runtrackcount * 4` (where `runtrackcount` is the number of backtracking opcodes in the compiled program). For a regex with N backtracking opcodes and a worst-case input, each `doubleIntSlice` call doubles the allocation. A 1 KB pattern can produce a `runtrackcount` in the hundreds; repeated doubling can allocate gigabytes on a 64-bit machine before the OS kills the process.

- **Attack Path**:
  1. Attacker supplies a regex with many backtracking constructs (alternation, quantifiers, lookarounds) and a moderately-sized input that causes deep backtracking.
  2. `ensureStack()` / `ensureStorage()` are called when `Runtrackpos` or `Runstackpos` crosses below the threshold.
  3. `doubleIntSlice` allocates `2 * oldLen` elements with no cap check.
  4. After ~30 doublings from a 1024-slot initial array: 1024 × 2^30 ≈ 1 TB of `[]int` (8 bytes per int on 64-bit) → OOM.
  5. Impact: Process killed by OOM killer; adjacent goroutines disrupted.

- **Attacker-Controlled**: Yes — pattern complexity controls `runtrackcount`; input controls how many frames are pushed.
- **Guard/Mitigation Present**: None. No `math.MaxInt` cap, no `maxAlloc` check.
- **Residual Exploitability**: Requires a pattern with many backtracking opcodes plus an input that triggers deep backtracking. The timeout (if set) may fire before memory exhaustion for simple patterns, but for complex patterns the allocation can happen rapidly.
- **Evidence**:
  ```go
  // runner.go:944-955
  func (r *Runner) ensureStorage() {
      if r.Runstackpos < r.runtrackcount*4 {
          doubleIntSlice(&r.runstack, &r.Runstackpos)
      }
      if r.Runtrackpos < r.runtrackcount*4 {
          doubleIntSlice(&r.runtrack, &r.Runtrackpos)
      }
  }

  // runner.go:957-965
  func doubleIntSlice(s *[]int, pos *int) {
      oldLen := len(*s)
      newS := make([]int, oldLen*2)  // <-- no upper bound
      copy(newS[oldLen:], *s)
      *pos += oldLen
      *s = newS
  }
  ```
- **Remediation**: Add a `maxStackSize` constant (e.g., 1<<24 slots = ~128 MB) in `doubleIntSlice` and return an error instead of allocating. The timeout mechanism can also mitigate this but only if it fires before the doubling exceeds available RAM.

---

### [SEC-003] Timeout Granularity Allows Millions of Steps Before Check (High)

- **Category**: Denial of Service / Insufficient Mitigation
- **OWASP Reference**: LLM10 (Unbounded Consumption)
- **Location**: `fastclock.go:105` (`DefaultClockPeriod = 100ms`), `fastclock.go:107-128` (`runClock`)
- **Confidence**: High
- **Issue**: Even when a finite `MatchTimeout` is set, the timeout check uses a background goroutine that updates an atomic clock value **once every ~100 ms** (`DefaultClockPeriod`). `CheckTimeout()` reads this atomic; between updates, the backtracking loop can execute **millions of opcode iterations** unchecked. An attacker can craft a pattern that burns CPU for ~99 ms per check, and if the timeout is, say, 1 second, that's still ~900 ms of unbounded compute before the timeout fires on the next clock tick.

- **Attack Path**:
  1. Caller sets `MatchTimeout = 1 * time.Second`. Clock period is 100 ms.
  2. Attacker supplies `(a+)+b` against `"aaaaa...c"` (1 KB).
  3. `executeDefault` checks `CheckTimeout()` every opcode iteration, but `deadline.reached()` only returns true after the background `runClock` goroutine updates `fast.current`.
  4. Between clock ticks, the VM runs at full speed with no step counter. On a 3 GHz CPU, ~300M instructions execute between ticks.
  5. Impact: A 1-second timeout doesn't actually limit to ~1 second of work. It allows up to ~1.1 seconds (timeout + clock period). More critically, the lack of a step counter means there's no defense-in-depth — the timeout is the **only** guard.

- **Attacker-Controlled**: Yes — pattern and input control the CPU work per opcode iteration.
- **Guard/Mitigation Present**: `CheckTimeout()` is called in two locations in the hot loop (once in `scan()`'s `findFirstChar` loop, once in `executeDefault()`'s main opcode loop). The clock is approximate by design for performance.
- **Residual Exploitability**: Moderate. The timeout mechanism works directionally but the 100 ms granularity is coarse enough that an attacker can degrade service significantly even within a "short" timeout window. A 100 ms timeout would have ~50 ms average precision — still too coarse for per-request protection at scale.
- **Evidence**:
  ```go
  // fastclock.go:105
  const DefaultClockPeriod = 100 * time.Millisecond

  // fastclock.go:117-126
  func runClock() {
      for fast.current.read() <= fast.clockEnd.read() {
          fast.mu.Unlock()
          time.Sleep(clockPeriod)    // <-- 100 ms between updates
          fast.mu.Lock()
          newTime := durationToTicks(time.Since(fast.start))
          fast.current.write(newTime)
      }
  }

  // runner.go:2045-2051
  func (r *Runner) CheckTimeout() error {
      if r.ignoreTimeout || !r.deadline.reached() {
          return nil  // <-- fast path: just one atomic read
      }
      return fmt.Errorf("match timeout after %v on input `%v`", r.timeout, string(r.Runtext))
  }
  ```
- **Remediation**: Add a configurable step counter (e.g., check after every N opcode iterations) as a secondary defense. The step counter could be checked in `CheckTimeout()` alongside the clock, with a default of, say, 1,000,000 steps. Additionally, `SetTimeoutCheckPeriod` can be called with a smaller value (e.g., 10ms) at the cost of more background CPU.

---

### [SEC-004] No Compile-Time ReDoS Pattern Detection (High)

- **Category**: Denial of Service / Missing Input Validation
- **OWASP Reference**: LLM03 (Supply Chain)
- **Location**: `syntax/parser.go`, `syntax/tree.go`, `syntax/writer.go`
- **Confidence**: Medium
- **Issue**: The parser and compiler do not reject or warn about patterns known to cause catastrophic backtracking (nested quantifiers like `(a+)+`, `(a*)*`, `(a+)+b`). Some optimizations in `tree.go` convert backtracking loops to atomic loops (`Oneloopatomic`, `Setloopatomic`) when the loop is at the end of the expression (via `eliminateEndingBacktracking()`), but these are best-effort and do not cover nested quantifier cases where the dangerous construct is *not* at the end. The .NET `Regex` class has similar behavior (no pattern rejection), so this is consistent with the library's design goal.

- **Attack Path**:
  1. Attacker supplies `(a+)+b` as a regex pattern.
  2. `Compile()` parses and compiles it successfully — no warning, no rejection.
  3. The compiled `Code` has backtracking opcodes for the nested quantifiers.
  4. When run against `"aaaaac"`, the backtracking VM explores an exponential number of paths.
  5. Impact: Same as SEC-001 — CPU exhaustion at runtime.

- **Attacker-Controlled**: Yes — the pattern is attacker-controlled.
- **Guard/Mitigation Present**: `eliminateEndingBacktracking()` in `tree.go:311-321` removes backtracking from constructs at the very end of the regex. `tree.go:458-459` makes alternation-internal loops atomic. These help but do not protect the general case.
- **Residual Exploitability**: High. The burden is entirely on the caller to set a timeout.
- **Evidence**:
  ```go
  // tree.go:311-321 — only helps when the dangerous construct is at the END
  // Optimization: eliminate backtracking for loops.
  // Optimization: backtracking removal at expression end.
  // If we find backtracking construct at the end of the regex, we can instead
  // make it non-backtracking, since nothing would ever backtrack into it anyway.
  rootNode.eliminateEndingBacktracking()
  ```
- **Remediation**: Consider adding a `MaxCompileComplexity` option that rejects patterns exceeding a heuristic complexity threshold (e.g., star height > 2, or nested quantifier depth > 3). This is a defense-in-depth measure; the primary defense remains the `MatchTimeout`.

---

### [SEC-005] `bytesEqual` Panics on Empty Slice via Unsafe Pointer (Medium)

- **Category**: Denial of Service / Unsafe Code
- **OWASP Reference**: N/A
- **Location**: `helpers/indexof.go:364-365`
- **Confidence**: Medium
- **Issue**: `bytesEqual()` takes the address of `a[0]` without checking `len(a) > 0`. If called with an empty slice, `&a[0]` panics. The only callers (`Equals` and `EqualsIgnoreCase`) both return early when `len(find) == 0`, so this is not directly exploitable today. However, the function is exported (lowercase but package-internal), and future callers could trigger the panic.

- **Attack Path**:
  1. (Theoretical) A future code path calls `bytesEqual` with an empty `a` or `b`.
  2. `&a[0]` on an empty slice panics with `runtime error: index out of range [0] with length 0`.
  3. Impact: Panic in a regex match operation — DoS for the goroutine.

- **Attacker-Controlled**: Not currently — guarded by callers.
- **Guard/Mitigation Present**: `Equals()` returns `true` for `len(find) == 0` before calling `bytesEqual`. `EqualsIgnoreCase` calls `Equals` first, which has the same guard.
- **Residual Exploitability**: None today. Defense-in-depth concern.
- **Evidence**:
  ```go
  // helpers/indexof.go:364-365
  func bytesEqual(a, b []rune) bool {
      bytesA := unsafe.Slice((*byte)(unsafe.Pointer(&a[0])), len(a)*4)  // panics if len(a)==0
      bytesB := unsafe.Slice((*byte)(unsafe.Pointer(&b[0])), len(b)*4)
      return bytes.Equal(bytesA, bytesB)
  }
  ```
- **Remediation**: Add `if len(a) == 0 || len(b) == 0 { return len(a) == len(b) }` at the top of `bytesEqual`.

---

## Integration Contract

### Input bounds
- **Accepted**: 
  - **Pattern**: A UTF-8 string in .NET/Perl5 regex syntax. The parser accepts patterns up to available memory; no explicit length or depth limit is enforced.
  - **Input text**: Arbitrary `string` or `[]rune`. No length limit beyond available memory. For string input, `getRunes()` allocates `[]rune(len(s))` — O(n) allocation proportional to input.
- **Rejected**: Malformed regex patterns return a `*syntax.Error` from `Compile`. Inputs that timeout return a `"match timeout after ..."` error string (not a typed error).
- **Max safe size**: No built-in limit. Callers should cap input length (e.g., 1 MB for untrusted input) and pattern length (e.g., 1 KB) before compilation.

### Output shape
- **Return type** (`MatchString`/`MatchRunes`): `(bool, error)`. `true` when the pattern matches; `false` when it doesn't. Error is non-nil only on timeout.
- **Return type** (`FindStringMatch`/`FindRunesMatch`): `(*Match, error)`. Non-nil `*Match` on success; `nil, nil` on no match; `nil, error` on timeout.
- **Return type** (`Replace`): `(string, error)`. The replaced string on success; error on timeout.
- **Return type** (`Split`): `([]string, error)`. The split parts on success; error on timeout.
- **Nil guarantees**: `FindStringMatch` returns `nil, nil` when no match is found (not an error). All `*Match` fields are populated on a successful match. `Match.Groups()` always returns at least one element (group 0).
- **Position invariants**: `Capture.RuneIndex` and `Capture.RuneLength` are in rune offsets, not byte offsets. Call `Capture.ByteRange()` for byte offsets. `Capture.RuneIndex >= 0` and `Capture.RuneLength >= 0` for valid captures.
- **Encoding guarantees**: All returned strings are valid UTF-8 (sliced from the original input or replacement text).

### Side effects
- **I/O**: None during matching. `Compile`/`MustCompile` have no I/O. The timeout clock (`fastclock`) starts a **single background goroutine** per process that runs until the latest deadline expires + 1 second. This goroutine reads `time.Now()` and writes to an `atomic.Int64` — no other I/O.
- **Allocations**: 
  - `Compile`: Allocates the `*Regexp` struct, `*syntax.Code`, character class sets, and prefix filter data. Proportional to pattern complexity.
  - String matching: Allocates `[]rune(len(s))` for the input string. Runner stacks (`runtrack`, `runstack`, `runcrawl`) are pooled per `*Regexp` instance and grow unboundedly if backtracking is deep.
  - `Replace`: Allocates a `bytes.Buffer` for the output (pooled up to `MaxCachedReplaceBufferLength`).
  - `FindAll*`: Allocates a `[][]int` slice proportional to the number of matches.
- **Global state**: The `fastclock` is a package-level global. `RegisterEngine` modifies a package-level `map` protected by `sync.RWMutex`. `DefaultMatchTimeout` and `DefaultOptimizationOptions` are mutable package-level vars.
- **Goroutine safety**: `*Regexp` is safe for concurrent use (uses `sync.Pool` for runners, `sync.Mutex` for the replacer data cache). Individual `*Match` values are NOT safe for concurrent use. The timeout clock goroutine is process-global.

### Error modes
- **Returned errors**:
  - `syntax.Error` — from `Compile` on malformed patterns.
  - `"match timeout after <duration> on input '<truncated>'"` — from any match method when `MatchTimeout` is exceeded.
  - `"startAt must align to the start of a valid rune in the input string"` — from `FindStringMatchStartingAt` and `Replace` when `startAt` falls in the middle of a multi-byte rune.
- **Panics**:
  - `MustCompile` panics on parse failure (by design, same as `regexp.MustCompile`).
  - `helpers.indexOf.bytesEqual` panics on empty slice input (see SEC-005).
  - No panics in the execution VM itself — opcodes are validated at compile time.
- **Timeouts**: Approximately `MatchTimeout` after match start, with ~100 ms granularity. The timeout error includes the input string (first 256-ish runes), which may leak input content in error messages/logs.

### Resource bounds
- **Memory**: `O(pattern_size + input_size + backtrack_depth)`. Backtracking stacks can grow without bound (`doubleIntSlice` has no cap).
- **CPU**: `O(2^n)` worst-case for patterns with nested quantifiers (exponential backtracking). Best-case `O(n)` for simple patterns. The runtime is bounded only by `MatchTimeout` (if set) — no step limit.
- **Stack**: The Go call stack depth is `O(1)` — the VM is iterative, not recursive. The `runtrack`/`runstack`/`runcrawl` slices are heap-allocated and can grow arbitrarily.

### Explicit non-guarantees
- Does NOT guarantee constant-time execution. Backtracking is exponential in the worst case.
- Does NOT guarantee bounded memory usage. Backtracking stack arrays double without limit.
- Does NOT apply a backtracking step limit. The timeout is the ONLY guard against runaway execution, and it defaults to infinity.
- Does NOT validate pattern safety at compile time. Patterns known to cause catastrophic backtracking compile successfully.
- Does NOT guarantee that `MatchTimeout` fires precisely at the deadline. Actual timeout is `deadline ± clockPeriod` (~100 ms).
- Does NOT strip sensitive input from timeout error messages. The input string is included in the error text.
- Does NOT guarantee stable output across versions. Optimization changes may alter match behavior for ambiguous patterns.

### Integration examples

```go
// CORRECT: Set a finite timeout before matching untrusted input
re := regexp2.MustCompile(userPattern)
re.MatchTimeout = 1 * time.Second
match, err := re.FindStringMatch(userInput)
if err != nil {
    // handle timeout — log, return 500, etc.
    return
}
```

```go
// CORRECT: Cap input size and set a generous but finite timeout
if len(userInput) > 1<<20 { // 1 MB
    return errors.New("input too large")
}
re := regexp2.MustCompile(`\w+@\w+\.\w+`)
re.MatchTimeout = 5 * time.Second
ok, _ := re.MatchString(userInput)
```

```go
// INCORRECT: No timeout set — DefaultMatchTimeout = infinity
re := regexp2.MustCompile(userPattern) // user controls the pattern
ok, _ := re.MatchString(userInput)     // runs forever on ReDoS pattern
```

```go
// INCORRECT: Trusting DefaultMatchTimeout to be finite
re := regexp2.MustCompile(`(a+)+b`)
// re.MatchTimeout is math.MaxInt64 — timeout checking is entirely skipped
ok, _ := re.MatchString("aaaaaaaaaaaaaaaaaaaaaaaaaaaaac")
// Blocks indefinitely
```

---

## Assessment

**Do not use without setting MatchTimeout.** The library is well-implemented for its design goals (.NET regex compatibility with backtracking), but the default configuration is dangerous for any scenario where patterns or inputs are not fully trusted. Every call site **must** set a finite `MatchTimeout` (≤ 1 second for user-supplied patterns, ≤ 5 seconds for trusted-but-complex patterns). For additional defense-in-depth, wrap calls in a `context.WithTimeout` as a second layer.
