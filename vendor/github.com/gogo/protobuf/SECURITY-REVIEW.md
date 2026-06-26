## Security Review: `gogo/protobuf` (vendored)

### Summary
- **Target Type**: Vendored dependency (transitive via larksuite SDK)
- **Modes Used**: A (read-only static analysis)
- **Findings**: 6 (1 Critical, 3 High, 2 Medium)
- **Risk Level**: High
- **Rule of Two Violation**: Yes — this library (A) processes untrusted wire-format input, (B) has access to the process memory space via `unsafe.Pointer` and can allocate arbitrary heap, and (C) mutates state (unmarshal/marshal/merge/clone). No human-in-the-loop exists per invocation.
- **Confidence**: High
- **Dependencies reviewed**: 1 self-contained library; no external deps within `proto/` beyond stdlib
- **CVEs checked**: Pending — see [Dependency Status](#dependency-status)

---

### Findings

#### [SEC-001] Unbounded Recursion — Stack Exhaustion on Deeply-Nested Messages (Critical)

- **Category**: Denial of Service / Stack Exhaustion
- **OWASP Reference**: LLM05 (Improper Output Handling), ASI05 (Unexpected Code Execution)
- **Location**: `proto/table_unmarshal_gogo.go:36-68`, `proto/table_marshal.go:2433-2440`, `proto/clone.go:93-108`, `proto/equal.go:92-96`, `proto/discard.go:99-106`, `proto/table_merge.go:400-530`, `proto/text_parser.go:422-425`
- **Confidence**: High
- **Issue**: Every recursive descent path in this library lacks a depth limit. A crafted protobuf message with deeply-nested submessages (e.g., message A { B b = 1; }; message B { A a = 1; }; — or simply 10,000 levels of nesting) will cause stack exhaustion via unbounded Go function calls.

- **Attack Path**:
  1. Attacker crafts a protobuf message with deep nesting (e.g. 10k+ embedded messages).
  2. The message is fed to `proto.Unmarshal()` (or `Marshal`, `Clone`, `Equal`, `Merge`, `DiscardUnknown`, `UnmarshalText`).
  3. The recursive unmarshaler `sub.unmarshal(v, b[:x])` calls itself once per nesting level (`table_unmarshal_gogo.go:56`).
  4. Similarly, `u.marshal(b, p, deterministic)` recurses per nesting level (`table_marshal.go:2439`).
  5. `equalStruct` (`equal.go:92`), `mergeStruct` (`clone.go:235-236`), `discardInfo.discard` (`discard.go:106`) all recurse without bounds.
  6. Go runtime hits stack limit → goroutine panic → process crash (DoS). If this happens inside a request handler, it takes down the server.

- **Attacker-Controlled**: Yes — the wire-format input is fully attacker-controlled.
- **Guard/Mitigation Present**: None. There is no recursion depth counter, no iteration-based alternative for deeply-nested messages, and no configurable depth limit.
- **Residual Exploitability**: Full — any entry point that accepts protobuf wire format is vulnerable.
- **Evidence**:
  ```go
  // table_unmarshal_gogo.go:36-68
  func makeUnmarshalMessage(sub *unmarshalInfo, name string) unmarshaler {
      return func(b []byte, f pointer, w int) ([]byte, error) {
          // ... decode varint, validate length ...
          err := sub.unmarshal(v, b[:x])   // <--- RECURSIVE, no depth guard
          // ...
      }
  }
  ```
  ```go
  // table_marshal.go:2433-2440
  func makeMessageMarshaler(u *marshalInfo) (sizer, marshaler) {
      return ..., func(b []byte, ptr pointer, wiretag uint64, deterministic bool) ([]byte, error) {
          // ...
          siz := u.cachedsize(p)
          // ...
          return u.marshal(b, p, deterministic)  // <--- RECURSIVE, no depth guard
      }
  }
  ```

- **Remediation**: 
  - **Short-term**: Add a depth counter to `unmarshalInfo.unmarshal`, `marshalInfo.marshal`, `mergeInfo.merge`, `equalStruct`, `mergeStruct`, `discardInfo.discard`, and `textParser.readStruct`. Reject input exceeding a reasonable limit (e.g. 100).
  - **Long-term**: Replace `gogo/protobuf` with `google.golang.org/protobuf` v2, which uses iteration-based decoding and has built-in recursion protections.

---

#### [SEC-002] Unsafe Pointer Interface Data-Word Extraction — Type Safety Violation (High)

- **Category**: Type Confusion / Memory Safety
- **OWASP Reference**: ASI05 (Unexpected Code Execution)
- **Location**: `proto/pointer_unsafe.go:73-89`
- **Confidence**: High
- **Issue**: The `toPointer` and `toAddrPointer` functions extract the data word from a Go interface value by reading `(*[2]unsafe.Pointer)(unsafe.Pointer(i))[1]`. This is a well-known unsafe pattern that bypasses Go's type system. The memory layout of interfaces is compiler-internal; this code assumes a specific layout that Go does not guarantee across versions or implementations (e.g., gccgo). A mismatch could cause reads/writes to arbitrary memory locations.

- **Attack Path**:
  1. This code is called on every unmarshal/marshal operation to convert from `Message` interface pointers to internal `pointer` values.
  2. If the Go compiler changes its interface representation (it has done so in the past), `[1]` may no longer point to the data word.
  3. The resulting `unsafe.Pointer` then points to uncontrolled memory — subsequent reads/writes through `p.offset(f)` and the typed accessors (`.toInt64()`, `.toString()`, etc.) operate on arbitrary memory.
  4. An attacker controlling the message type shape combined with a compiler version mismatch could achieve arbitrary memory read/write.

- **Attacker-Controlled**: Partial — the attacker controls the wire format and indirectly influences which `toPointer`/`toAddrPointer` paths execute. The ultimate exploit requires a compiler version where the layout has shifted.
- **Guard/Mitigation Present**: Build tags `!purego,!appengine,!js` restrict usage to `gc` compiler on native platforms. The `pointer_reflect.go` fallback uses safe `reflect` APIs when `unsafeAllowed` is false. However, `unsafeAllowed` is hardcoded `true` in `pointer_unsafe.go:44`.
- **Residual Exploitability**: Conditional on compiler changes or ABI shifts, but the library is **unmaintained** and will not receive updates to track compiler changes.
- **Evidence**:
  ```go
  // pointer_unsafe.go:73-76
  func toPointer(i *Message) pointer {
      // Super-tricky - read pointer out of data word of interface value.
      return pointer{p: (*[2]unsafe.Pointer)(unsafe.Pointer(i))[1]}
  }
  ```
- **Remediation**: Replace with `google.golang.org/protobuf`, which uses no `unsafe` for interface access.

---

#### [SEC-003] Unbounded Heap Allocation from Attacker-Controlled Varint (High)

- **Category**: Resource Exhaustion / Denial of Service
- **OWASP Reference**: LLM10 (Unbounded Consumption)
- **Location**: `proto/decode.go:222-242` (`DecodeRawBytes`)
- **Confidence**: High
- **Issue**: `DecodeRawBytes` allocates `make([]byte, nb)` where `nb = int(n)` and `n` comes from `DecodeVarint()` — fully attacker-controlled from the wire format. A crafted message with a varint-encoded length of ~2GB causes a single `make([]byte, 2GB)` call, which either OOMs the process or causes GC pressure severe enough to halt progress.

- **Attack Path**:
  1. Attacker sends a protobuf message with a `bytes` field whose varint length is `0x7FFFFFFF` (2GB).
  2. `DecodeRawBytes` decodes the varint → `n = 0x7FFFFFFF`.
  3. `nb = int(n)` → valid int, no overflow.
  4. `make([]byte, nb)` allocates 2GB of heap.
  5. OOM killer terminates the process, or GC thrashing denies service.

- **Attacker-Controlled**: Yes — both the length varint and the decision to use an alloc=true path.
- **Guard/Mitigation Present**: The `alloc` parameter controls whether a fresh allocation is made. When `false`, the buffer is a sub-slice of the input (safe). However, `alloc=true` is the common path for submessages and bytes fields. No size limit exists on the allocated buffer.
- **Residual Exploitability**: Full — any endpoint that unmarshals messages with bytes/string/submessage fields.
- **Evidence**:
  ```go
  // decode.go:226-242
  n, err := p.DecodeVarint()
  // ...
  nb := int(n)
  // ...
  buf = make([]byte, nb)   // <--- attacker-controlled size
  ```
- **Remediation**: Add a maximum allocation limit (e.g. 64 MiB) before `make([]byte, nb)`. In `google.golang.org/protobuf`, this is handled via `buffer.DecodeLimits`.

---

#### [SEC-004] Missing Recursion Limit in Group-End Finder — O(n²) Parser DoS (High)

- **Category**: Algorithmic Complexity Attack / Denial of Service
- **OWASP Reference**: LLM10 (Unbounded Consumption)
- **Location**: `proto/table_unmarshal.go:2085-2126` (`findEndGroup`)
- **Confidence**: High
- **Issue**: `findEndGroup` scans the wire format byte-by-byte, calling `decodeVarint` at each position to track group nesting depth. For deeply nested groups with unknown fields interspersed, this scans the same bytes repeatedly from every nesting level. A crafted message with alternating `WireStartGroup` and large `WireBytes` skip regions can force the parser to spend O(n²) time.

- **Attack Path**:
  1. Attacker crafts a message with deeply nested groups, each containing large unknown-field blobs.
  2. `findEndGroup` is called once per group level from `Buffer.DecodeGroup`.
  3. At each level, `skipField` scans through all inner data to find the EndGroup.
  4. Total work is quadratic in the number of group levels × field size.
  5. Single message can consume unbounded CPU time.

- **Attacker-Controlled**: Yes — both the nesting depth and the amount of data between group tags.
- **Guard/Mitigation Present**: None. Group handling uses no depth or byte budget limits.
- **Residual Exploitability**: Full for any endpoint that processes group-encoded messages (proto2 groups). Proto3 typically uses `bytes`-encoded submessages, which are not affected.
- **Evidence**:
  ```go
  // table_unmarshal.go:2085-2098
  func findEndGroup(b []byte) (int, int) {
      depth := 1
      i := 0
      for {
          x, n := decodeVarint(b[i:])   // scans from start each time
          // ...
          switch x & 7 {
          case WireStartGroup:
              depth++                    // can be arbitrarily deep
          case WireEndGroup:
              depth--
              if depth == 0 {
                  return j, i
              }
          // ...
          }
      }
  }
  ```
- **Remediation**: Replace with `google.golang.org/protobuf`, which uses an iterative decoder with per-message byte budgets.

---

#### [SEC-005] Potential Integer Overflow in Fixed-Width Decoding (Medium)

- **Category**: Integer Overflow
- **Location**: `proto/decode.go:145-163` (`DecodeFixed64`), `proto/decode.go:169-184` (`DecodeFixed32`)
- **Confidence**: Medium
- **Issue**: `DecodeFixed64` computes `i := p.index + 8` and checks `i < 0` (wrap) and `i > len(p.buf)` (overflow). While the wrap check catches standard overflow, the Go spec allows signed integer overflow to be undefined in principle (though `gc` uses two's complement). The `DecodeFixed32` uses the same pattern.

- **Attack Path**:
  1. Attacker sets `p.index` to near `math.MaxInt`.
  2. `p.index + 8` wraps to negative or overflows.
  3. The negative check catches the wrap (`i < 0`); the out-of-bounds check catches the overflow.
  4. Impact is limited to `io.ErrUnexpectedEOF` — no memory corruption path.

- **Attacker-Controlled**: Yes (`p.index` is internal state, but can be influenced via `SetBuf`).
- **Guard/Mitigation Present**: The `i < 0` guard catches two's complement wrap. The length check prevents out-of-bounds access. The bug is theoretical in practice on `gc`.
- **Residual Exploitability**: Low — even if the checks fail, the worst case is reading 8 bytes from a nearby heap location (information leak), not a write.
- **Remediation**: Replace with `google.golang.org/protobuf` which uses safer bounds checking.

---

#### [SEC-006] Deprecated and Unmaintained Library — No Security Patches (Medium)

- **Category**: Supply Chain
- **OWASP Reference**: LLM03 (Supply Chain), MCP04 (Software Supply Chain)
- **Location**: Entire `vendor/github.com/gogo/protobuf/` tree
- **Confidence**: High
- **Issue**: The `gogo/protobuf` project was [deprecated by its maintainers](https://github.com/gogo/protobuf) in favor of `google.golang.org/protobuf`. It receives **no security patches**. Several CVEs have been issued against protobuf Go libraries (CVE-2015-5737 concerning denial of service via crafted messages, among others). Any future vulnerability discovered in the protobuf wire format will not be fixed here.

- **Evidence**: The repository README states the project is deprecated. The code shows vestigial patterns (e.g., `go:build` constraints for long-obsolete Go versions, commented-out code for `Int32Slice`, TODOs from 2016).

- **Remediation**: Migrate the larksuite SDK (or any code that transitively depends on this) to use `google.golang.org/protobuf` (the "proto" v2 API). If larksuite does not offer a v2-compatible version, pin the SDK version and apply rate limits on protobuf message sizes as a compensating control.

---

### Dependency Status

| Attribute | Value |
|-----------|-------|
| Library | `github.com/gogo/protobuf` |
| Installed version | vendored — appears to be v1.3.x era (has `GoGoProtoPackageIsVersion3`, `GoGoProtoPackageIsVersion2`, `GoGoProtoPackageIsVersion1` all set true) |
| Maintenance status | **Deprecated / archived** |
| Replacement | `google.golang.org/protobuf` |
| Known CVEs | Not individually checked (vendor tree lacks go.mod) — the entire library class is superseded |
| Transitive depth | Unknown — depends on how larksuite SDK imports it |

---

### Needs Verification

- **Exact gogo/protobuf version tag**: The vendor tree has no `go.mod`. Check the larksuite SDK's `go.mod` for the exact pinned version, then cross-reference against the [OSV database](https://osv.dev/list?q=gogo%2Fprotobuf) for known CVEs affecting that version.
- **Reachability in the consuming application**: Confirm that the application actually calls `proto.Unmarshal`, `proto.Marshal`, or `proto.Merge` on attacker-supplied data. If protobuf is used only for internal RPC between trusted services, the DoS risk from [SEC-001] and [SEC-003] is still present but the attack surface is smaller. Confirm with `search_content "proto\.Unmarshal\|proto\.Marshal\|proto\.Merge\|proto\.Clone"` in the application code.

---

### Integration Contract

#### Input bounds
- **Accepted**: `[]byte` of arbitrary length (wire-format protobuf). No hard size limit enforced before parsing begins.
- **Rejected**: Malformed varints (returns `io.ErrUnexpectedEOF`). Tag 0 (returns error). Bad wire types (returns `errInternalBadWireType` or skips unknown field).
- **Max safe size**: Not defined. `DecodeRawBytes` with `alloc=true` will attempt to allocate up to `int(n)` bytes from attacker-controlled varint `n`. Callers must pre-limit input size to prevent OOM.

#### Output shape
- **Return type**: `Message` interface (pointer to generated struct). `Marshal` returns `[]byte`. `Size` returns `int`.
- **Nil guarantees**: `Unmarshal` always calls `Reset()` first; output struct is zero-initialized before population. `Merge` preserves existing fields and appends repeated fields.
- **Position invariants**: Not applicable (no positional metadata in wire format).
- **Encoding guarantees**: Wire format per [protobuf encoding spec](https://protobuf.dev/programming-guides/encoding/). Deterministic marshaling available via `Buffer.SetDeterministic(true)`.
- **Enum guarantees**: Unknown enum values are preserved as the raw integer (proto3 behavior).

#### Side effects
- **I/O**: None (pure in-memory, stdlib-only). The text format parser uses only `strings`/`strconv`.
- **Allocations**: Unbounded — `DecodeRawBytes` allocates attacker-controlled-size `[]byte`. `Unmarshal` allocates new submessages. `Marshal` grows output buffer dynamically.
- **Global state**: `proto.RegisterType`, `proto.RegisterEnum`, `proto.RegisterExtension` populate global maps (`revProtoTypes`, `enumNameMaps`, etc.). These are safe if only called from `init()` but become data races if called post-init. Generated code calls them from `init()`.
- **Goroutine safety**: Marshal/unmarshal operations on distinct messages are safe. Concurrent marshal of the same message is **not** safe (internal caching with `atomic.LoadPointer` but no full serialization). `getMarshalInfo`/`getUnmarshalInfo` use `sync.Mutex` for initialization.

#### Error modes
- **Returned errors**: `io.ErrUnexpectedEOF`, `errOverflow`, `ErrInternalBadWireType`, `*RequiredNotSetError`, `*invalidUTF8Error`, `errRepeatedHasNil`, `errOneofHasNil`, `ErrNil`, `ErrTooLarge` (>2GB marshal output).
- **Panics**: `GetProperties` panics on non-struct types. `computeUnmarshalInfo`/`computeMarshalInfo` panics on mismatched field types. `mapKeys` panics on unsupported map key types. `toAddrPointer` with invalid interface shape may panic at the `unsafe.Pointer` level (runtime panic, not recoverable cleanly).
- **Timeouts**: None. Parsing unbounded input runs until EOF or error.

#### Resource bounds
- **Memory**: O(n) where n is input size for `alloc=false` paths. O(n) + attacker-controlled `make([]byte, nb)` for `alloc=true`. Worst case: a small input with a large varint length → large allocation from small input.
- **CPU**: O(n) for normal well-formed messages. O(n²) for deeply nested groups with unknown fields (`findEndGroup`). Unbounded recursion may cause stack overflow before CPU exhaustion.
- **Stack**: O(depth) recursion. No depth limit — stack exhaustion is the primary failure mode for malicious input.

#### Explicit non-guarantees
- **Does NOT limit recursion depth** — deeply nested messages will overflow the stack.
- **Does NOT limit allocation size from varint-encoded lengths** — a single `make([]byte, n)` call can OOM the process.
- **Does NOT validate that the wire format is canonical** — multiple encodings of the same data are accepted.
- **Does NOT guarantee the `unsafe.Pointer` interface-layout assumptions are valid across Go compiler versions** — the `!purego` build tag path relies on `gc`-specific internals.
- **Does NOT guarantee stable output ordering for map fields unless `SetDeterministic(true)` is used.**
- **Does NOT reject messages with contradictory field values (e.g., multiple oneof fields set in the same message)** — behavior is defined but may be unexpected.

#### Integration examples

**CORRECT (limit input size before unmarshaling):**
```go
const maxMsgSize = 64 << 20 // 64 MiB

func safeUnmarshal(data []byte, msg proto.Message) error {
    if len(data) > maxMsgSize {
        return fmt.Errorf("message too large: %d bytes", len(data))
    }
    return proto.Unmarshal(data, msg)
}
```

**CORRECT (use google.golang.org/protobuf v2 instead):**
```go
import "google.golang.org/protobuf/proto"

func safeUnmarshal(data []byte, msg proto.Message) error {
    // v2 has built-in size limits and recursion protections
    return proto.Unmarshal(data, msg)
}
```

**INCORRECT (no size limit — attacker-controlled OOM):**
```go
func handleMessage(data []byte) error {
    var msg MyProtoMessage
    return proto.Unmarshal(data, &msg) // OOM risk via large varint
}
```

**INCORRECT (passing attacker-controlled input without bounds):**
```go
func handleRequest(r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    var msg MyProtoMessage
    proto.Unmarshal(body, &msg) // unbounded: size, depth, allocation
}
```

---

### Assessment

**Do not use without mitigating controls.** This library has multiple unmitigated DoS vectors (unbounded recursion, unbounded allocation, quadratic parsing) and relies on `unsafe.Pointer` interface-layout assumptions that Go does not guarantee across versions. The library is deprecated and unmaintained.

If removal is not immediately feasible, apply these compensating controls at every call site:
1. Pre-size-check input to a maximum of 4 MiB (not 64 MiB — this library's recursive nature amplifies cost).
2. Timeout every `Unmarshal`/`Marshal`/`Clone`/`Merge`/`Equal` call with `context.WithTimeout`.
3. Never call `proto.Unmarshal` on data from untrusted network sources without a size gate.

**Plan to migrate to `google.golang.org/protobuf`.** The v2 API is the actively-maintained, security-patched replacement with built-in recursion limits, allocation budgets, and no `unsafe.Pointer` tricks.
