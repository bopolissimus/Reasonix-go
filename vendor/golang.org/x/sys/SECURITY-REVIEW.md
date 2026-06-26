## Security Review: golang.org/x/sys v0.46.0 (vendored subset)

### Summary
- **Target Type**: Source Code — Go extended standard library
- **Modules reviewed**: `unix`, `windows`, `plan9`
- **Source**: `golang.org/x/sys@v0.46.0` (BSD-3-Clause, Go Authors)
- **Findings**: 0 Critical, 0 High, 2 Medium
- **Risk Level**: Low
- **Rule of Two Violation**: No
- **Confidence**: High
- **Dependencies reviewed**: 3 modules (unix, windows, plan9); no external deps beyond stdlib `syscall` and `unsafe`
- **Reasonix importers**: `internal/cli/theme_osc_unix.go` (unix), `desktop/devinfo_windows.go` (windows), `desktop/open_workspace_windows.go` (windows), `internal/proc/kill_windows.go` (windows), `internal/sysproxy/system_windows.go` (windows), plus ~30 transitive importers via terminal libraries (bubbletea, lipgloss, ultraviolet, etc.)

### Findings

#### [SEC-SYS-001] Extensive unsafe.Pointer surface area — MEDIUM

- **Category**: Memory Safety
- **OWASP Reference**: LLM03 (Supply Chain)
- **Location**: `unix/*.go` (~1500+ `unsafe.Pointer`/`uintptr` sites), `windows/*.go` (~700+ sites)
- **Confidence**: Medium
- **Issue**: The `x/sys` packages use `unsafe.Pointer` extensively to interface with kernel syscalls and Windows API functions. This is inherent to their purpose — Go cannot make syscalls without converting between Go types and kernel-expected memory layouts. Each `unsafe.Pointer` conversion is a potential memory corruption site if the size, alignment, or lifetime of the pointed-to memory is incorrect.
- **Attack Path**: Exploitation would require either:
  1. A bug in the Go compiler's memory layout that causes a struct to not match kernel expectations (extremely unlikely — covered by Go's compatibility guarantees)
  2. A malformed struct passed from application code that causes the kernel to read/write out of bounds (structural hazard, not injectable)
  3. A kernel bug triggered via valid syscall that the wrapper faithfully exposes
- **Attacker-Controlled**: No — syscall arguments are constructed by application code, not from attacker-controlled wire data in these packages
- **Guard/Mitigation Present**: Generated code (`zsyscall_*.go`) is produced by `mkwinsyscall`/`mksyscall` tools with known-correct struct layouts. Manual syscall wrappers in `syscall_linux.go` etc. follow established patterns reviewed by the Go team.
- **Residual Exploitability**: None from external attacker. The risk is supply-chain (a compromised `x/sys` release could insert malicious syscalls) or application-level (incorrect use of exposed syscall primitives).
- **Remediation**: No code change needed. This is an accepted risk profile for syscall wrapper libraries. Mitigations: (a) pin the vendor hash and review updates; (b) restrict which syscalls application code calls (don't expose `Ptrace`, `Setuid`, `Chroot` to untrusted callers).

#### [SEC-SYS-002] Powerful primitives exposed with no guardrails — MEDIUM

- **Category**: Excessive Capability
- **OWASP Reference**: LLM06 (Excessive Agency), ASI02 (Tool Misuse)
- **Location**: `unix/syscall_linux.go` and per-OS syscall files
- **Confidence**: Medium
- **Issue**: The `unix` package exposes powerful system primitives including `PtraceAttach`, `PtraceDetach`, `PtracePokeData`, `Setuid`, `Setgid`, `Chroot`, `Exec`, `Mount`, `Mknod`, `Kill`, `Reboot`, and hundreds of `ioctl` operations. Any Go code importing `golang.org/x/sys/unix` can call these without additional privilege checks — the only barrier is the kernel's own permission model (CAP_SYS_ADMIN, root, etc.).
- **Attack Path**:
  1. Application code imports `golang.org/x/sys/unix` for benign terminal I/O (e.g., `ioctl` for terminal size)
  2. A vulnerability elsewhere in the application (e.g., command injection in a tool executor) allows calling arbitrary Go functions
  3. The attacker chains to `unix.PtraceAttach()` to inject code into another process, `unix.Reboot()` to crash the system, or `unix.Mount()` to remount filesystems
- **Attacker-Controlled**: Partial — attacker needs code execution within the application first; `x/sys` amplifies the blast radius
- **Guard/Mitigation Present**: Kernel enforces permission checks (CAP_SYS_ADMIN, ownership, etc.). Application code typically only calls a narrow subset of `unix` functions.
- **Residual Exploitability**: Real for applications with powerful tool execution capabilities (like Reasonix's `Bash` tool). The risk is not in `x/sys` itself but in what the application does with the primitives it exposes.
- **Remediation**: For Reasonix specifically: ensure the `Bash` tool and any subprocess execution paths do not grant access to `unix` syscall wrappers on the import path. The terminal I/O usage (theme_osc_unix.go) is benign. Document that `x/sys/unix` imports should be limited to well-defined terminal/OS operations, not general syscall access.

### Needs Verification

#### [SEC-SYS-002-V] Verify Reasonix's tool execution boundary

The `x/sys/unix` package is imported for terminal I/O (`theme_osc_unix.go`) and transitively by terminal libraries. Confirm that:
1. No `Bash`-executed code can import `x/sys/unix` at runtime
2. No LLM-generated code paths can reach `unix` syscall wrappers

If both hold, SEC-SYS-002 is entirely mitigated for Reasonix. Status: needs runtime verification.

### Integration Contract

#### unix

**Input bounds**:
- **Accepted**: OS-specific file descriptors (`int`), paths (`string`), flags (`int`/`uint`), sizes; virtually all POSIX syscall argument types
- **Rejected**: invalid file descriptors → kernel returns `EBADF`; invalid paths → `ENOENT`; wrong types → compile-time rejection (Go type system)
- **Max safe size**: path length limited by OS `PATH_MAX` (typically 4096); buffer sizes caller-controlled

**Output shape**:
- Most syscall wrappers: return `error` (nil = success, non-nil = syscall.Errno)
- Read-like calls: return `(n int, err error)`
- Open-like calls: return `(fd int, err error)`
- Nil guarantees: syscall return values are zero-valued on error
- `BytePtrFromString`, `ByteSliceFromString`: return `(*byte, error)` or `([]byte, error)` — nil pointer on error

**Side effects**:
- **I/O**: Direct kernel syscalls — all side effects are whatever the kernel does (read, write, create, delete, mount, kill, reboot, etc.)
- **Allocations**: Most wrappers allocate only for path/string conversion (`BytePtrFromString`); syscall buffers allocated by caller
- **Global state**: Syscalls like `Setuid`, `Chroot`, `Mount` affect the entire process (not goroutine-local)
- **Goroutine safety**: Syscalls are goroutine-safe at the Go level; kernel-level safety depends on the specific syscall (e.g., `write` is atomic for pipes ≤ PIPE_BUF; `read` + `write` on same fd from multiple goroutines may interleave)

**Error modes**:
- All syscall errors returned as `syscall.Errno` wrapping kernel errno values (`EACCES`, `ENOENT`, `EINVAL`, etc.)
- String conversion errors: `EINVAL` for embedded NUL bytes
- **Panics**: Documented panics exist in edge cases — see individual function docs; the overwhelming majority return errors

**Resource bounds**:
- **Memory**: Caller-controlled — buffers, iovecs, etc. allocated by caller, not by the wrapper
- **CPU**: Kernel-bounded — syscall cost varies by operation; no busy-wait in the Go wrappers
- **Time**: Unbounded for blocking syscalls (read on empty pipe, wait on child, etc.); caller must manage deadlines via goroutine patterns or `SetDeadline`

**Explicit non-guarantees**:
- Does NOT validate path safety — caller is responsible for path traversal prevention
- Does NOT enforce any access control — kernel permission model is the sole barrier
- Does NOT provide safe defaults — unsafe operations (Ptrace, Setuid, Reboot) are exposed with no guard
- Platform-specific: APIs available on one OS may panic or not compile on others; check build tags
- The API is NOT stable across minor versions — symbols may be added, removed, or changed

**Integration examples**:
```go
// CORRECT: constrained usage for terminal I/O
import "golang.org/x/sys/unix"
fd := int(os.Stdout.Fd())
ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
if err != nil {
    return err
}
```

```go
// INCORRECT: passing user-controlled data directly to powerful syscalls
import "golang.org/x/sys/unix"
unix.Chroot(userInput)        // BAD: no path validation
unix.PtraceAttach(userInput)  // BAD: attacker-controlled PID
unix.Mount(src, dst, userFS, flags, data) // BAD: user-controlled mount params
```

#### windows

**Input bounds**:
- **Accepted**: `Handle` (uintptr), `HWND` (uintptr), Windows API string types (`*uint16`), standard Go types coerced to Windows equivalents
- **Rejected**: NUL bytes in `UTF16FromString` → `syscall.EINVAL`; invalid handles → Windows API error
- **Max safe size**: Paths limited by Windows `MAX_PATH` (260) unless using extended-length path prefix; string conversions bound by Go string size

**Output shape**:
- API wrappers: return `(result, error)` where `error` is non-nil on failure; error is `syscall.Errno` wrapping Windows error codes
- `UTF16FromString`: returns `([]uint16, error)` — nil slice on NUL error
- `UTF16ToString`: returns string — silently truncates at NUL
- `EscapeArg`, `ComposeCommandLine`: return `string` — properly quoted for Windows command line parsing

**Side effects**:
- **I/O**: Windows API calls via `syscall.SyscallN` — all side effects from the underlying Windows API
- **Allocations**: String conversion allocates UTF-16 buffers; `ComposeCommandLine` allocates result buffer
- **Global state**: Some Windows APIs affect process-wide or system-wide state
- **Goroutine safety**: Windows API calls are goroutine-safe (each `SyscallN` enters the kernel independently)

**Error modes**:
- Windows API errors returned as `syscall.Errno` with the Windows error code
- NUL-in-string errors returned as `syscall.EINVAL`
- **Panics**: `StringToUTF16` panics on NUL bytes (deprecated — use `UTF16FromString` instead)

**Resource bounds**:
- **Memory**: String conversions O(n); API buffers caller-allocated
- **CPU**: O(1) per API call plus kernel overhead
- **Time**: Unbounded for blocking API calls (e.g., `WaitForSingleObject`)

**Explicit non-guarantees**:
- Does NOT validate Windows API arguments beyond type correctness — caller is responsible
- `CommandLineToArgv` returns a fixed-size array pointer (8192×8192 `uint16`) regardless of actual argument count — use `DecomposeCommandLine` instead for safe parsing
- `EscapeArg` does NOT handle the program name (first argument) specially — use `ComposeCommandLine` for full command lines
- Platform-specific: Windows-only APIs; will not compile on non-Windows targets

**Integration examples**:
```go
// CORRECT: safe command line composition
args := []string{"program.exe", "-flag", "value with spaces"}
cmdLine := windows.ComposeCommandLine(args)
// cmdLine: program.exe -flag "value with spaces"

// CORRECT: safe argument decomposition
args, err := windows.DecomposeCommandLine(cmdLine)
```

```go
// INCORRECT: deprecated StringToUTF16 — panics on NUL
s := windows.StringToUTF16(userInput) // panics if userInput contains \x00

// CORRECT alternative:
s, err := windows.UTF16FromString(userInput)
if err != nil {
    return err
}
```

### Assessment

**Safe to use.** The `x/sys` packages are low-level syscall wrappers — they faithfully expose OS primitives with correct memory layout. The `unsafe.Pointer` usage is inherent and correct. The primary risk is not in the packages themselves but in how they are used: exposing `unix.Ptrace`, `unix.Chroot`, or `unix.Reboot` to untrusted callers is dangerous. Reasonix's usage is confined to benign terminal I/O (`IoctlGetWinsize`, etc.) and Windows path resolution — well within the safe subset. No exploitable vulnerabilities were found in the vendored code.

### Skill Trust Classification

N/A — this is a library, not a skill.
