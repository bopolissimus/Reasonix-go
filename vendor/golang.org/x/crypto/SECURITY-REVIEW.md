## Security Review: golang.org/x/crypto v0.53.0 (vendored subset)

### Summary
- **Target Type**: Source Code — Go extended standard library
- **Modules reviewed**: `bcrypt`, `blowfish`
- **Source**: `golang.org/x/crypto@v0.53.0` (BSD-3-Clause, Go Authors)
- **Findings**: 0 Critical, 0 High, 1 Medium
- **Risk Level**: Low
- **Rule of Two Violation**: No
- **Confidence**: High
- **Dependencies reviewed**: 2 direct (bcrypt, blowfish) — no external deps beyond stdlib
- **Reasonix importers**: `internal/serve/auth.go` (bcrypt)

### Findings

#### [SEC-CRYPTO-001] Blowfish: 64-bit block cipher (Sweet32) — MEDIUM

- **Category**: Cryptographic Weakness
- **OWASP Reference**: LLM03 (Supply Chain)
- **Location**: `blowfish/cipher.go:1` (package-level; deprecated in doc comment)
- **Confidence**: High
- **Issue**: The blowfish package implements a 64-bit block cipher. After ~2³² blocks of data (~32GB), ciphertext collisions become probable due to the birthday bound (Sweet32 attack). The package itself documents this: "Blowfish is a legacy cipher and its short block size makes it vulnerable to birthday bound attacks (see https://sweet32.info)."
- **Attack Path**:
  1. Attacker must observe large volumes of blowfish-encrypted traffic (~32GB of ciphertext under the same key)
  2. Birthday-bound collisions leak plaintext XOR differences, enabling plaintext recovery
- **Attacker-Controlled**: No — data volume threshold makes this impractical in normal operation
- **Guard/Mitigation Present**: The package explicitly documents the deprecation and recommends AES-GCM or XChaCha20-Poly1305. In Reasonix, blowfish is only used indirectly via bcrypt (which uses blowfish internally for key schedule, not for bulk encryption).
- **Residual Exploitability**: None in practice. Bcrypt encrypts a fixed 24-byte string 64 times with Blowfish — well below the 32GB birthday bound. No bulk-data encryption path exists.
- **Evidence**: `blowfish/cipher.go:12-15`: `// Deprecated: any new system should use AES (from crypto/aes, if necessary in an AEAD mode like crypto/cipher.NewGCM) or XChaCha20-Poly1305`
- **Remediation**: No action required. The blowfish package is only present as a dependency of bcrypt, which uses it correctly for its key schedule, not for bulk encryption. Do not introduce direct blowfish usage into new code.

### Needs Verification

No MEDIUM-confidence items requiring verification.

### Integration Contract

#### bcrypt

**Input bounds**:
- **Accepted**: `password []byte` of length 1–72 bytes; `cost int` in range [4, 31]; `hashedPassword []byte` ≥ 59 bytes with `$2a$` or `$2y$` prefix
- **Rejected**: passwords > 72 bytes (`ErrPasswordTooLong`); cost < 4 or > 31 (`InvalidCostError`); hash < 59 bytes (`ErrHashTooShort`); invalid hash prefix (`InvalidHashPrefixError`); future bcrypt versions (`HashVersionTooNewError`)
- **Max safe size**: password capped at 72 bytes; hash output always 60 bytes; salt always 16 bytes (22 encoded)

**Output shape**:
- `GenerateFromPassword`: returns `[]byte` of exactly 60 bytes (the bcrypt hash in modular crypt format `$2a$<cost>$<22-char-salt><31-char-hash>`), or nil + error
- `CompareHashAndPassword`: returns nil on match, non-nil error on mismatch; uses constant-time comparison
- `Cost`: returns cost int from 4–31, or 0 + error
- Nil guarantees: hash output is never nil on success

**Side effects**:
- **I/O**: Reads from `crypto/rand.Reader` (entropy source) — blocks if system entropy exhausted
- **Allocations**: O(1) per call; allocates 60-byte result buffer, 16-byte salt, temporary Blowfish cipher (~4KB state)
- **Global state**: None
- **Goroutine safety**: Safe for concurrent use; each call is self-contained

**Error modes**:
- See Input bounds above for all returned errors
- **Panics**: None — all errors returned

**Resource bounds**:
- **Memory**: Fixed ~4.2KB per call (Blowfish cipher state + tempporaries)
- **CPU**: O(2^cost) — cost=10 means 1024 rounds × 2 Blowfish expansions ≈ 2M operations; cost=31 means ~2B operations (acceptable upper bound given cost validation)
- **Time**: ~100ms at cost=10; ~200s at cost=31 (DoS concern if attacker can pick cost — but cost is validated ≤31)

**Explicit non-guarantees**:
- Does NOT validate password strength or complexity — caller is responsible for password policy
- Does NOT zero memory after use (hash/salt/password copies may persist in Go heap)
- `CompareHashAndPassword` does NOT return the hash — only nil/error; timing-safe comparison is used
- Does NOT support bcrypt versions newer than `$2a$` (returns `HashVersionTooNewError`)

**Integration examples**:
```go
// CORRECT: hash with explicit cost, constant-time compare
hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
if err != nil {
    return err
}
// ... store hash ...
if err := bcrypt.CompareHashAndPassword(storedHash, []byte(attempt)); err != nil {
    return ErrInvalidCredentials
}
```

```go
// INCORRECT: comparing hashes with bytes.Equal (timing attack)
hash1, _ := bcrypt.GenerateFromPassword(pw1, 10)
hash2, _ := bcrypt.GenerateFromPassword(pw2, 10)
if bytes.Equal(hash1, hash2) { // BAD — timing leak
```

#### blowfish

**Input bounds**:
- **Accepted**: `key []byte` of length 1–56 bytes (NewCipher) or 1+ bytes (NewSaltedCipher); `salt []byte` of any length (empty salt falls through to NewCipher)
- **Rejected**: key length < 1 or > 56 (`KeySizeError` for NewCipher); empty salt is silently accepted (treated as no salt)
- **Max safe size**: block size is fixed 8 bytes; key up to 56 bytes

**Output shape**:
- `NewCipher`/`NewSaltedCipher`: returns `*Cipher` or nil + error
- `Encrypt(dst, src []byte)`: writes exactly 8 bytes to dst from 8 bytes of src
- `Decrypt(dst, src []byte)`: writes exactly 8 bytes to dst from 8 bytes of src
- Nil: `*Cipher` is nil on error, never nil on success

**Side effects**:
- **I/O**: None
- **Allocations**: Cipher struct is ~4KB (18×uint32 + 4×256×uint32); key expansion mutates cipher state
- **Global state**: None (uses embedded Pi/S-box tables that are read-only constants)
- **Goroutine safety**: Cipher is not safe for concurrent use — Encrypt/Decrypt mutate internal state via ExpandKey

**Error modes**:
- `KeySizeError` returned for invalid key length; `NewSaltedCipher` returns the error from `NewCipher` when salt is empty with invalid key
- **Panics**: None — all errors returned

**Resource bounds**:
- **Memory**: ~4.1KB per Cipher instance
- **CPU**: O(key_length) for key schedule; O(1) for Encrypt/Decrypt (fixed 16 Feistel rounds)
- **Stack**: Fixed depth, no recursion

**Explicit non-guarantees**:
- Does NOT provide authenticated encryption — raw ECB-like block cipher; caller must use CBC/CTR mode from `crypto/cipher`
- Does NOT provide any semantic security for multi-block data — 64-bit block size vulnerable to Sweet32
- This package is DEPRECATED — not suitable for new systems

**Integration examples**:
```go
// CORRECT: only use blowfish for bcrypt compatibility, never directly
import "golang.org/x/crypto/bcrypt" // uses blowfish internally

// INCORRECT: using blowfish directly for application encryption
c, _ := blowfish.NewCipher(key)
c.Encrypt(dst, src) // 64-bit block — vulnerable to Sweet32
```

### Assessment

**Safe to use.** The vendored subset (bcrypt + blowfish) has no exploitable vulnerabilities. Blowfish is deprecated but used correctly by bcrypt for key scheduling, not bulk encryption. Bcrypt correctly uses `crypto/rand` for salt and `crypto/subtle.ConstantTimeCompare` for hash comparison. The 72-byte password limit is documented and returns an error rather than silently truncating.

### Skill Trust Classification

N/A — this is a library, not a skill.
