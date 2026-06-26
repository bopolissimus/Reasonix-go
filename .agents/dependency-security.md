# Dependency Security — Go Vendoring + Security Review

## Why vendored

Every Go dependency is vendored into `vendor/` (`go mod vendor`) and the project builds with `-mod=vendor`. This means:

- **All source is local and auditable.** No network fetch during build. No trust in proxy.golang.org at build time beyond the initial `go mod download`.
- **`go.sum` still verified.** `go mod vendor` copies only packages whose checksums match `go.sum`, so tampered dependencies are rejected before vendoring.
- **Zero runtime network dependency resolution.** The build is hermetic — identical on every machine with the same vendor tree.
- **45 MB of source, 38 packages.** Every line of third-party code is in the repo and can be reviewed.

## When to re-vendor

After any change to `go.mod` (add, remove, or upgrade a dependency):

```bash
go mod tidy
go mod vendor
go build -mod=vendor ./...
```

If `go mod tidy` pulls in a NEW dependency (direct or transitive), you MUST run the security review below before committing.

## Mandatory security review

**Every new dependency, at every depth, must pass the security-reviewer skill audit.** This applies to:

- Direct dependencies you add explicitly
- Transitive dependencies pulled in by upgrades
- Dependencies added by `go mod tidy` when Go toolchain or stdlib changes

### Review steps

1. **Vendor first:**

   ```bash
   go mod tidy
   go mod vendor
   go build -mod=vendor ./...
   ```

2. **Identify new packages** (compare `vendor/` before and after, or check `go.sum` diff):

   ```bash
   git diff --stat vendor/
   ```

3. **Run the security-reviewer skill** on all new packages:

   ```
   /security-reviewer vendor/github.com/new/package
   ```

   For batch reviews (multiple new packages), delegate to v4-pro sub-agents. The skill's frontmatter specifies `model: deepseek-v4-pro` for this reason.

4. **File findings as Jira tickets** in the REX project. Tag them `security` and `dependency`. Critical findings block the PR.

5. **Document the review** in `docs/security/dependency-review-YYYY-MM.md`. Follow the format of `docs/security/dependency-review-2026-06.md`:

   - Summary table: package, version, review date, findings, disposition
   - For each finding: severity, location (file:line), attack path, remediation
   - Clean packages: note that they passed review with no findings

### What to look for

| Category | Check |
|----------|-------|
| Credentials/secrets | Reads env vars, files, or keychains that might leak keys |
| Network safety | SSRF, insecure TLS, plaintext HTTP, hardcoded URLs |
| Command execution | `os/exec`, `syscall`, subprocess spawning |
| File system safety | Path traversal, symlink following, unsafe temp files |
| Input validation | Missing validation on external input |
| Supply chain | Unmaintained packages, suspicious patterns, unexpected network calls in `init()` |
| Cryptography | Weak algorithms, broken RNG, hardcoded keys |
| Deprecated packages | Archived repos, known CVEs, replacement available |

### Skip review for

- Packages that are purely algorithmic/data-structure with no I/O surface (no `os`, `net`, `os/exec`, `syscall`, `crypto`, file system access)
- Test-only dependencies (`*_test.go` imports) that don't ship in the binary
- Packages already reviewed in a previous audit (check `docs/security/` for existing reviews)

### Disposition

| Finding | Action |
|---------|--------|
| Critical / High | Block the PR — must be fixed or mitigated before merge |
| Medium | File a Jira ticket, fix in a follow-up PR |
| Low | Note in the review document, no action required |

## Full audit

A comprehensive audit of all 38 vendored packages was completed on 2026-06-25. See:

- `docs/security/dependency-review-2026-06.md` — consolidated findings
- `docs/security/security-review-batch1.md` through `batch4.md` — per-package details

Findings: 0 Critical, 1 High, 8 Medium, 12 Low. Action items tracked in REX-92 through REX-96.
