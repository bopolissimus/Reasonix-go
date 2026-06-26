# Security Review: Batch 1 — Vendored Go Dependencies

**Date:** 2025-07-15  
**Scope:** 7 vendored packages under `/home/dev/reasonix/go1/vendor/`  
**Skip policy:** Test files excluded unless they reveal production config.  
**Severity scale:** Critical → High → Medium → Low / Informational.

---

## 1. `golang.org/x/net` (network, HTTP, proxy)

**Files examined:** 21 (http/, idna/, internal/, proxy/, websocket/)

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| No `InsecureSkipVerify` usage | — | — | Standard Go HTTPS/TLS handling; no bypass. |
| No hardcoded credentials | — | — | Only vendored copy of upstream `golang.org/x/net`. |
| No command execution | — | — | No `os/exec` or `syscall.Exec` found. |
| No path traversal | — | — | No file operations outside proxy env-var reads. |
| No `init()` with network calls or file writes | — | — | Only package-level variable initializations. |
| No crypto weaknesses | — | — | Standard library crypto; no MD5/SHA1 usage. |

**Verdict:** ✅ No findings. This is a standard vendored copy of the upstream `golang.org/x/net` submodule with no modifications that introduce risk.

---

## 2. `golang.org/x/crypto` (cryptography)

**Files examined:** 7 (bcrypt/, blowfish/)

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| No `InsecureSkipVerify` | — | — | Not applicable (no TLS code in these sub-packages). |
| No hardcoded credentials | — | — | Pure algorithm implementations. |
| No command execution | — | — | No `os/exec` or similar. |
| No path traversal | — | — | No file operations at all. |
| No `init()` with I/O | — | — | Only static data tables. |
| No crypto weaknesses | — | — | bcrypt uses Blowfish with adaptive cost; no MD5/SHA1. |

**Verdict:** ✅ No findings. Only `bcrypt` and `blowfish` sub-packages are vendored; these are well-audited implementations.

---

## 3. `github.com/gorilla/websocket` (WebSocket)

**Files examined:** 19

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| **Weak masking-key RNG** — uses `math/rand.Uint32()` instead of `crypto/rand` | **Medium** | `conn.go:13` (import `math/rand`), `conn.go:184-188` (`newMaskKey()` calls `rand.Uint32()`) | WebSocket masking keys are generated with `math/rand`, which is predictable if the seed can be guessed. For client→server masking (RFC 6455 §5.3), this could theoretically allow an attacker to reconstruct the masking key and manipulate frame payloads. **Practical impact is low** because `math/rand` is auto-seeded on Go ≥1.20 and the key is 32 bits only; still, the WebSocket spec recommends a strong source. |
| **SHA1 in WebSocket handshake** — used per RFC 6455 §4.2.2 | **Low (by-design)** | `util.go:9,20-24` (`computeAcceptKey` uses `crypto/sha1`) | SHA1 is used as required by the WebSocket protocol for the `Sec-WebSocket-Accept` challenge computation. This is not a security vulnerability; it's mandated by the RFC. Noted for completeness. |
| **TLS verification enforced** | ✅ (positive) | `tls_handshake.go:15`, `tls_handshake_116.go:15` | Both build-tagged variants explicitly enforce hostname verification after handshake when `InsecureSkipVerify` is false. |
| **Proxy auth credentials passed in cleartext** to upstream proxy | **Low** | `proxy.go:50-53` | HTTP proxy `Proxy-Authorization: Basic` credentials are base64-encoded (not encrypted). This is inherent to the HTTP Basic scheme, not a code defect. Only affects connections through an HTTP proxy. |
| No hardcoded credentials | — | — | Credentials come from caller-provided config/headers. |
| No command execution | — | — | No `os/exec` or `syscall` usage. |
| No path traversal | — | — | No file I/O operations. |
| `init()` — safe | — | `proxy.go:23` | Registers the HTTP proxy dialer; no network calls, no file writes. |

---

## 4. `github.com/larksuite/oapi-sdk-go/v3` (Lark/Feishu API SDK)

**Files examined:** 312 (core/, service/*, ws/, card/, event/)

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| **SHA1 used for card event signature verification** | **Low** | `card/card.go:276` (`Signature` uses `crypto/sha1`) | Card event `Signature()` function uses SHA1 for HMAC-style verification. SHA1 is considered weak for collision resistance. However, this is used as a symmetric verification token (not for digital signatures), so practical exploitation requires knowing the secret token. Still, SHA1 should be upgraded to HMAC-SHA256. |
| **`math/rand` seeded with `time.Now()` for reconnect jitter** | **Low** | `ws/client.go:285-286` (`rand.Seed(time.Now().UnixNano())` + `rand.Intn()`) | Reconnect backoff jitter uses predictable `math/rand`. This is for timing only (not cryptography), so impact is negligible — an attacker cannot gain advantage from predicting reconnect timing. |
| **`http.DefaultClient` used as bootstrap HTTP client** | **Low** | `ws/client.go:55` | The WebSocket client bootstrap uses `http.DefaultClient` instead of a configured client. This means no custom TLS config, timeouts, or transport settings apply during the initial handshake. Users of this SDK may not realize the bootstrap step lacks their configured transport. |
| **`http.DefaultClient` used as fallback** | **Informational** | `core/httptransport.go:30,192`, `core/utils.go:287` | When no `HttpClient` is configured, the SDK falls back to `http.DefaultClient`. This is documented behavior but means TLS config depends on Go's defaults (proper cert verification). Users should provide a custom client with timeouts. |
| No `InsecureSkipVerify` | ✅ | — | No code disables TLS verification. |
| No hardcoded credentials | ✅ | — | `AppSecret` and tokens are configuration fields, not hardcoded. |
| No command execution | ✅ | — | No `os/exec`, `syscall`, or shell invocations. |
| No path traversal | ✅ | — | No file I/O that accepts user-controlled paths. |
| `init()` — none exist | ✅ | — | No `init()` functions in the package. |
| **Large attack surface** (model files) | **Informational** | `service/*/v1/model.go` (hundreds of files) | The SDK bundles auto-generated model files for every Lark API endpoint. Each model file is large (many thousands of lines). While this is normal for generated SDKs, it creates a large review surface. |

---

## 5. `github.com/godbus/dbus/v5` (D-Bus IPC)

**Files examined:** 47

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| **Potential path traversal in SHA1 auth cookie lookup** | **Medium** | `auth_sha1_windows.go:66` (`os.Open(a.home + "/.dbus-keyrings/" + string(context))`) | The `AuthCookieSha1` mechanism opens a file at `~/.dbus-keyrings/<context>`, where `context` comes from the D-Bus server's challenge. If a malicious D-Bus server sends a `context` containing `../`, the path could escape the keyrings directory. **Mitigation**: This only applies when using DBUS_COOKIE_SHA1 authentication against an attacker-controlled D-Bus server — a rare scenario since D-Bus is typically used on trusted local sockets. |
| **SHA1 used in DBUS_COOKIE_SHA1 auth mechanism** | **Low (by-design)** | `auth_sha1_windows.go:7,50` (`crypto/sha1`) | SHA1 is used as specified in the D-Bus specification for cookie-based authentication. This is a protocol requirement, not a code defect. |
| **Command execution: `launchctl` on macOS** | **Low** | `conn_darwin.go:13` (`exec.Command("launchctl", "getenv", "DBUS_LAUNCHD_SESSION_BUS_SOCKET")`) | Executes `launchctl` to discover the D-Bus session bus socket path. Arguments are hardcoded constants — no user input involved. |
| **Command execution: `dbus-launch` on other Unix** | **Medium** | `conn_other.go:16,20` (`execCommand("dbus-launch")` — exported variable `execCommand = exec.Command`) | Runs `dbus-launch` to get the session bus address. The command name is hardcoded (`"dbus-launch"`), but `execCommand` is an exported package-level variable that **could be overridden** by another package at init time, opening a code-injection path if an attacker can influence Go package init ordering. |
| **`init()` functions — all safe** | ✅ | `transport_generic.go:20`, `transport_nonce_tcp.go:11`, `transport_tcp.go:8`, `transport_unix.go:91` | All four `init()` functions register transport constructors into a map. No network calls, no file writes. |
| No hardcoded credentials | ✅ | — | Credentials are caller-provided. |
| No temp file issues | ✅ | — | No temporary file creation. |

---

## 6. `github.com/zalando/go-keyring` (credential storage)

**Files examined:** 17 (core, secret_service/, internal/)

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| **Command execution on macOS: `security` CLI** | **High** | `keyring_darwin.go:44,76,105,125` | The macOS backend calls `/usr/bin/security` via `exec.Command()` for all operations (Get, Set, Delete, DeleteAll). **The `Set` function (line 76-96) pipes the command via stdin, constructing a command string with `fmt.Sprintf` that includes user-controlled `service`, `username`, and `password` values.** While `shellescape.Quote()` is used, shell injection via argument-passing in exec.Command is not the concern — but the password value (base64 encoded) is written to stdin of `security -i`, which is safer than command-line arguments. |
| **Password length limit bypass via encoding** | **Low** | `keyring_darwin.go:74` | Passwords are base64-encoded before storage, which inflates size by ~33%. The `Set` function checks `len(command) > 4096` (line 85), but the encoding happens before this check. Very long passwords could silently exceed OS limits after encoding. |
| **Base64 encoding of secrets (not encryption)** | **Low** | `keyring_darwin.go:74` | On macOS, secrets are base64-encoded with a well-known prefix (`go-keyring-base64:`) before storage. This is not encryption — it's a workaround for multi-line/non-ASCII values. The actual security depends on the system keychain (which encrypts at rest). |
| **`init()` — sets provider** | ✅ | `keyring_darwin.go:138`, `keyring_unix.go:180`, `keyring_windows.go:101` | All three OS-specific `init()` functions set the provider variable. No network calls or file writes. |
| No hardcoded credentials | ✅ | — | Credentials are caller-provided at runtime. |
| No path traversal | ✅ | — | File paths are constructed from OS APIs, not user input. |
| No TLS issues | ✅ | — | No network connections made by this package. |
| **Unix backend uses D-Bus (secret_service)** | **Informational** | `secret_service/secret_service.go`, `keyring_unix.go:9-181` | The Linux/BSD backend connects to the D-Bus session bus to talk to the Secret Service (gnome-keyring / kwallet). This inherits the security properties of the D-Bus session (typically `AF_UNIX` only, local user). |

**Key observation for macOS backend:** The `Set` function writes the command to stdin of `security -i`, which is safer than passing secrets as argv, but still means the password traverses an inter-process pipe. On a compromised host, another process with the same uid could ptrace or read /proc. This is inherent to the macOS Keychain CLI interface.

---

## 7. `github.com/sabhiram/go-gitignore` (filesystem globbing)

**Files examined:** 5

| Finding | Severity | File:Line | Detail |
|---|---|---|---|
| **Reads arbitrary files via `CompileIgnoreFile` / `CompileIgnoreFileAndLines`** | **Low** | `ignore.go:183,196` (`ioutil.ReadFile(fpath)`) | These functions accept a file path and read it. If the caller passes a user-controlled path, this could read arbitrary files. **However, this is a library function design choice, not a vulnerability in the library itself** — the caller controls the path argument. |
| No hardcoded credentials | ✅ | — | No credential handling at all. |
| No command execution | ✅ | — | Pure pattern-matching library. |
| No TLS issues | ✅ | — | No network code. |
| No `init()` | ✅ | — | No init functions. |
| No crypto weaknesses | ✅ | — | No crypto usage. |
| No temp files | ✅ | — | Read-only operations. |

**Verdict:** ⚠️ The library itself is safe. The only risk is if a calling application passes user-controlled file paths to `CompileIgnoreFile()` — but that's a caller-side concern, not a library vulnerability.

---

## Summary

| Package | Critical | High | Medium | Low / Info |
|---|---|---|---|---|
| `golang.org/x/net` | — | — | — | — |
| `golang.org/x/crypto` | — | — | — | — |
| `gorilla/websocket` | — | — | 1 (weak RNG for masking) | 1 (SHA1 per RFC) |
| `larksuite/oapi-sdk-go/v3` | — | — | — | 3 (SHA1 signature, `math/rand` jitter, `http.DefaultClient` fallback) |
| `godbus/dbus/v5` | — | — | 1 (path traversal in cookie auth) | 2 (SHA1 per spec, `execCommand` var) |
| `zalando/go-keyring` | — | 1 (macOS `exec.Command` with piped input) | — | 2 (base64 not encryption, length limit) |
| `sabhiram/go-gitignore` | — | — | — | — |

**Top actionable items:**
1. **High - zalando/go-keyring:** macOS backend uses `/usr/bin/security` CLI via `exec.Command` with user-controlled values piped via stdin (`keyring_darwin.go:76-96`). While shellescape quoting is applied, consider using the macOS Keychain native API via CGo instead.
2. **Medium - gorilla/websocket:** Masking keys generated with `math/rand` instead of `crypto/rand` (`conn.go:184-188`). Replace with `crypto/rand`.
3. **Medium - godbus/dbus/v5:** Path traversal potential in cookie auth file lookup (`auth_sha1_windows.go:66`). Sanitize the `context` parameter to reject `../` sequences.
4. **Low - larksuite/oapi-sdk-go/v3:** Card signature uses SHA1 (`card/card.go:276`); upgrade to HMAC-SHA256.
