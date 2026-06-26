## Security Review: github.com/danieljoos/wincred v1.2.3

- **Target Type**: Vendored Go dependency (Windows Credential Manager API wrapper)
- **Modes Used**: A (Read, Grep, Glob only)
- **Findings**: 0 Critical, 0 High, 2 Medium, 3 Low
- **Risk Level**: Low
- **Rule of Two Violation**: No — this is a pure API wrapper; it has no (A) untrusted-input processing of its own
- **Confidence**: High
- **Dependencies reviewed**: 1 total (1 direct: `golang.org/x/sys` v0.46.0, 0 transitive beyond that)
- **CVEs checked**: NVD, GitHub Advisory DB, OSV — 0 CVEs found for this package
- **Release notes analyzed**: Latest upstream release is v1.2.3 (same as installed); no newer versions
- **Cross-dependency chains identified**: 0 (single dependency, no internal chain)

---

### Summary

wincred is a thin Go wrapper around the Windows Credential Management API
(`advapi32.dll` — `CredReadW`, `CredWriteW`, `CredDeleteW`, `CredEnumerateW`,
`CredFree`). It exposes credential read/write/delete/list operations and
converts between Go structs and the native Windows `CREDENTIALW` struct via
`unsafe.Pointer` and `reflect.SliceHeader`. The library itself does **no**
network I/O, filesystem access, command execution, or `init()` side effects.
All risk is in the `unsafe`/syscall layer and in how consuming code handles the
returned credential data.

---

### Findings

#### [SEC-001] Unsafe pointer conversion with unbounded length copy (Medium)

- **Category**: Memory Safety
- **OWASP Reference**: N/A (systems-level)
- **Location**: `conversion.go:39-51`
- **Confidence**: Medium
- **Issue**: `goBytes(src *byte, len uint32)` constructs a Go `[]byte` slice
  backed by the raw C pointer `src` using `reflect.SliceHeader` + `unsafe.Pointer`,
  then copies it into a Go-managed slice. If the `len` parameter exceeds the
  actual allocated buffer, `copy()` reads out-of-bounds memory.
- **Attack Path**:
  1. Windows `CredReadW` / `CredEnumerateW` return a `CREDENTIALW` struct with
     a `CredentialBlobSize` or `ValueSize` field.
  2. `sysToCredential()` calls `goBytes(cred.CredentialBlob, cred.CredentialBlobSize)`.
  3. If Windows returns a malformed struct (corrupted heap, bad driver, privilege
     escalation via another vulnerability), the copy could read beyond the buffer.
- **Attacker-Controlled**: No (sizes come from the Windows API, not user input).
- **Guard/Mitigation Present**: The size values originate from the Windows OS
  itself, which is trusted. No path from untrusted input to these parameters.
- **Residual Exploitability**: None in normal operation. Would require a
  pre-existing Windows kernel or `advapi32` vulnerability to feed bad sizes.
- **Evidence**:
  ```go
  // conversion.go:39-51
  func goBytes(src *byte, len uint32) []byte {
      if src == nil || len == 0 {
          return []byte{}
      }
      rv := make([]byte, len)
      copy(rv, *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
          Data: uintptr(unsafe.Pointer(src)),
          Len:  int(len),
          Cap:  int(len),
      })))
      return rv
  }
  ```
- **Remediation**: Low priority. This is a standard Go syscall pattern. The
  risk is bounded by the trustworthiness of the Windows OS. No action needed
  unless the library is used in a sandboxed/AppContainer context where the
  Windows API itself could be compromised.

#### [SEC-002] Unsafe slice construction from API-returned pointer + count (Medium)

- **Category**: Memory Safety
- **OWASP Reference**: N/A (systems-level)
- **Location**: `sys.go:131-136`, `conversion.go:67-80`
- **Confidence**: Medium
- **Issue**: Both `sysCredEnumerate()` and `sysToCredential()` construct Go
  slices from raw pointers returned by the Windows API using
  `reflect.SliceHeader` + `unsafe.Pointer`:
  - `sysCredEnumerate()`: `*(*[]*sysCREDENTIAL)(unsafe.Pointer(&reflect.SliceHeader{...}))`
    with `count` from `CredEnumerateW`.
  - `sysToCredential()`: same pattern for `Attributes` array with
    `cred.AttributeCount`.
  - If the returned count is inconsistent with the actual allocated memory, the
    resulting Go slice header points to memory that Go does not own.
- **Attack Path**: Same as SEC-001 — requires Windows API to return corrupt data.
- **Attacker-Controlled**: No.
- **Guard/Mitigation Present**: Count values come from Windows API. The slices
  are read (not written) and immediately converted to safe Go structs. The
  `CredFree` call in `defer` handles cleanup.
- **Residual Exploitability**: None in normal operation.
- **Evidence**:
  ```go
  // sys.go:131-136
  credsSlice := *(*[]*sysCREDENTIAL)(unsafe.Pointer(&reflect.SliceHeader{
      Data: pcreds,
      Len:  count,
      Cap:  count,
  }))
  ```
  ```go
  // conversion.go:72-77
  attrSlice := *(*[]sysCREDENTIAL_ATTRIBUTE)(unsafe.Pointer(&reflect.SliceHeader{
      Data: uintptr(unsafe.Pointer(cred.Attributes)),
      Len:  int(cred.AttributeCount),
      Cap:  int(cred.AttributeCount),
  }))
  ```
- **Remediation**: Same as SEC-001. This is idiomatic Go syscall code. The
  `reflect.SliceHeader` pattern is deprecated in Go 1.21+ in favor of
  `unsafe.Slice`, but the risk profile is identical. Consider updating to
  `unsafe.Slice` for forward compatibility with Go toolchain checks.

---

#### [SEC-003] Credential data returned as plain []byte with no zeroing (Low)

- **Category**: Sensitive Information Disclosure
- **OWASP Reference**: LLM02
- **Location**: `types.go:34` (`CredentialBlob []byte`), `wincred.go:35`
- **Confidence**: High
- **Issue**: The `CredentialBlob` field is a plain Go `[]byte`. After the
  caller is done with the credential, the underlying memory is not zeroed.
  Go's garbage collector may reuse the memory, but the old secret data remains
  until overwritten. This is a defense-in-depth concern — the library does not
  expose credentials itself, but it provides no mechanism for secure cleanup.
- **Attack Path**: A separate vulnerability (e.g., memory dump, core dump,
  `/proc/pid/mem` on WSL) could read residual credential data from the Go heap.
- **Attacker-Controlled**: No.
- **Guard/Mitigation Present**: None at library level. Go's GC does not zero
  freed memory by default. The caller (go-keyring) copies the blob to a `string`
  and returns it — strings are immutable and also not zeroed.
- **Residual Exploitability**: Requires local access to process memory. Low
  practical risk for a desktop CLI tool.
- **Remediation**: Callers should copy the credential blob and zero the original
  slice after use (e.g., `bytes.Clone` + `clear`). The library could expose a
  `ZeroBlob()` helper, but this is a caller responsibility in idiomatic Go.

---

#### [SEC-004] `List()` enumerates all stored Windows credentials (Low)

- **Category**: Excessive Data Access
- **OWASP Reference**: LLM06
- **Location**: `wincred.go:126-136`
- **Confidence**: High
- **Issue**: `List()` calls `sysCredEnumerate("", true)` which returns **all**
  credentials in the Windows Credential Manager for the current user. A caller
  with access to this function can enumerate credentials belonging to other
  applications.
- **Attack Path**: If an attacker controls the calling code (e.g., through a
  compromised go-keyring consumer), they could call `List()` to discover what
  other applications store credentials and then call `GetGenericCredential()`
  with each target name to read them.
- **Attacker-Controlled**: Indirectly — the caller decides whether to call `List()`.
- **Guard/Mitigation Present**: go-keyring uses `List()` only in `DeleteAll()`,
  which filters by a `service:` prefix before operating. It does not expose raw
  `List()` results to callers.
- **Residual Exploitability**: Bounded by go-keyring's usage pattern.
- **Evidence**:
  ```go
  // wincred.go:126-136
  func List() ([]*Credential, error) {
      creds, err := sysCredEnumerate("", true)
      if err != nil && errors.Is(err, ErrElementNotFound) {
          creds = []*Credential{}
          err = nil
      }
      return creds, err
  }
  ```
- **Remediation**: The library's API is a direct mirror of the Windows API.
  Restricting `List()` would break compatibility with legitimate use cases.
  Mitigation belongs in the caller: never expose raw `List()` output to
  untrusted channels.

---

#### [SEC-005] No credential data in error returns (Low — positive finding)

- **Category**: Defense-in-Depth
- **Location**: `wincred.go:20-23`, `sys.go:76-142`
- **Confidence**: High
- **Issue**: The library returns only Windows error codes (`syscall.Errno`) on
  failure — never credential data. Error constants (`ErrElementNotFound`,
  `ErrInvalidParameter`, `ErrBadUsername`) are predefined and contain no dynamic
  data. This is the correct pattern: errors are safe to log.
- **Evidence**:
  ```go
  // wincred.go:20-23
  ErrElementNotFound   = sysERROR_NOT_FOUND      // syscall.Errno(1168)
  ErrInvalidParameter  = sysERROR_INVALID_PARAMETER // syscall.Errno(87)
  ErrBadUsername       = sysERROR_BAD_USERNAME     // syscall.Errno(2202)
  ```
- **Remediation**: None needed. This is the correct design.

---

### Needs Verification

No MEDIUM-confidence items requiring further investigation.

---

### Integration Contract

#### Input bounds
- **Accepted**: `targetName string` — any valid UTF-8 string. The Windows API
  validates the format; excessively long strings (≥32K characters) may cause
  `sysERROR_INVALID_PARAMETER`. No documented maximum in this library.
- **Rejected**: The Windows API rejects malformed target names at the OS level.
  The library does not add its own validation.
- **Max safe size**: `CredentialBlob` is limited to **2560 bytes** by the
  Windows API (`CRED_MAX_CREDENTIAL_BLOB_SIZE`). Exceeding this in `Write()`
  returns a Windows error.
- **Filter string** (for `FilteredList`): an empty filter causes
  `sysERROR_INVALID_PARAMETER` or `sysERROR_NOT_FOUND`. The filter is passed
  as-is to `CredEnumerateW`.

#### Output shape
- **Return type**: `*GenericCredential`, `*DomainPassword`, `*Credential`, or
  `[]*Credential` for list operations. All credential structs have
  `CredentialBlob []byte` containing the raw secret.
- **Nil guarantees**: Returns `nil, error` when a credential is not found.
  `List()` returns `[]*Credential{}, nil` (empty slice, not nil) when no
  credentials exist. `FilteredList()` returns the same for no matches.
- **Encoding guarantees**: `CredentialBlob` is raw bytes. The library performs
  **no encoding or decoding** — it passes bytes directly to/from the Windows
  API. Callers must apply their own encoding (UTF-16 LE is documented as the
  common Windows convention).
- **String fields**: `TargetName`, `Comment`, `TargetAlias`, `UserName` are
  decoded from UTF-16 via `windows.UTF16PtrToString()`. These are guaranteed
  valid UTF-8 after conversion.

#### Side effects
- **I/O**: Calls Windows `advapi32.dll` functions only. No filesystem access,
  no network access, no subprocess execution.
- **Allocations**: Each `Get*` and `List` call allocates new Go structs and
  copies credential data from Windows-managed memory. The Windows memory is
  freed via `CredFree` in a `defer`.
- **Global state**: None. No package-level mutable state. No `init()` functions
  with side effects on Windows builds.
- **Goroutine safety**: Safe for concurrent use. Each call creates its own
  Windows API invocation with no shared state. The underlying `proc` variables
  (`procCredRead`, etc.) are initialized once via `NewLazySystemDLL` which is
  concurrency-safe.

#### Error modes
- **Returned errors**:
  | Error | Trigger |
  |-------|---------|
  | `ErrElementNotFound` (`syscall.Errno(1168)`) | Credential with given target name does not exist; or `List()`/`FilteredList()` returns no results |
  | `ErrInvalidParameter` (`syscall.Errno(87)`) | Empty target name; blob exceeds 2560 bytes; invalid credential type |
  | `ErrBadUsername` (`syscall.Errno(2202)`) | Invalid username format in credential |
  | `"Operation not supported"` (non-Windows) | Any operation on non-Windows platforms |
  | Other `syscall.Errno` | Any other Windows API failure |
- **Panics**: None. All errors are returned.
- **Timeouts**: The Windows API calls are synchronous and do not have configurable
  timeouts. They return immediately unless the Credential Manager service is
  unresponsive (rare — would manifest as a hang, not a timeout error).

#### Resource bounds
- **Memory**: `List()` allocates one `*Credential` per stored credential and
  copies each blob. A user with thousands of credentials could allocate
  significant memory. Bounded by the user's own credential store, not by
  attacker input.
- **CPU**: O(n) in number of credentials for `List()`; O(1) for single-credential
  operations.
- **Stack**: No recursion; stack usage is bounded by syscall frames.

#### Explicit non-guarantees
- Does **NOT** zero credential data after use. Callers must zero their own
  copies if secure cleanup is required.
- Does **NOT** encrypt or decrypt credential data — the Windows Credential
  Manager handles encryption transparently at the OS level.
- Does **NOT** validate that `targetName` is safe for the caller's use case.
  Callers must ensure they don't pass attacker-controlled target names to
  `GetGenericCredential()` or `Write()`.
- Does **NOT** limit which credentials `List()` returns — it enumerates all
  credentials accessible to the current user, including those stored by other
  applications.
- Does **NOT** provide atomic compare-and-swap or transactional write+delete.
  Each operation is an independent Windows API call.
- **Non-Windows builds**: All functions return `"Operation not supported"`.
  There is no fallback to file-based or in-memory storage.

#### Integration examples

**CORRECT: Store and retrieve a credential with go-keyring as intermediary**
```go
// go-keyring's windowsKeychain.Set() handles validation:
// - password ≤ 2560 bytes
// - service < 512 bytes (or < 30KB for DeleteAll compatibility)
// - constructs credName as "service:username"
err := keyring.Set("my-app", "user@example.com", "secret-token")
secret, err := keyring.Get("my-app", "user@example.com")
```

**CORRECT: Direct usage with explicit error handling**
```go
cred, err := wincred.GetGenericCredential("myAppTarget")
if errors.Is(err, wincred.ErrElementNotFound) {
    // credential doesn't exist — handle gracefully
}
if err != nil {
    // some other Windows API error — log the error code, never the credential
    return fmt.Errorf("credential read failed: %w", err)
}
// Use cred.CredentialBlob, then zero it after
token := string(cred.CredentialBlob)
clear(cred.CredentialBlob) // zero the original
```

**INCORRECT: Logging credential data in errors**
```go
cred, err := wincred.GetGenericCredential(targetName)
if err != nil {
    // DANGEROUS: if err is nil but cred is somehow empty, or if the
    // pattern is used elsewhere — never log credential blobs.
    log.Printf("failed to read credential %s: %v", string(cred.CredentialBlob), err)
}
```

**INCORRECT: Calling List() and exposing results**
```go
// DANGEROUS: exposes all stored credentials to the caller
allCreds, _ := wincred.List()
return allCreds // leaks other applications' credentials
```

---

### Assessment

**Safe to use.** wincred is a thin, focused Windows API wrapper with a clean
design: no network, no filesystem, no command execution, no `init()` side
effects, and no credential data in error paths. The `unsafe.Pointer` usage is
idiomatic Go syscall code — the sizes come from the trusted Windows OS, not
from user input. The primary risk is in how the consuming code (go-keyring,
then the Reasonix credential layer) handles retrieved credential data.
Existing go-keyring callers do not log or forward credentials; this invariant
should be preserved in future changes.

---

### Previous Reviews

| Date | Review | Key Findings |
|------|--------|-------------|
| 2025-01-16 | `docs/security/security-review-batch4.md` | Medium risk: heavy unsafe/syscall usage, credential enumeration via `List()` |
| 2026-06-25 | `docs/security/dependency-review-2026-06.md` | SEC-010: Credential Manager Access (Medium). Audit go-keyring → wincred path |
| 2026-07-17 | This review | 2 Medium (memory safety), 3 Low; clean design; Integration Contract added |
