# Git — Go Migration project

## Never commit or push without asking

Explain what you want to commit (files + message) and ask clearly for permission. Same for push — explain what will be pushed and ask.

## Commit messages

Single-line imperative mood, 50–72 chars, no trailing period. No AI attribution.
Conventional Commits: `type(scope): description`.

## Merging — fetch first

`git merge origin/main` merges the **local** `origin/main` ref (last fetched snapshot). If the remote has moved since the last fetch, the merge will say "Already up to date" while a subsequent push fails with:

```
! [rejected]  main -> main (fetch first)
```

**Correct sequence:**

```sh
git fetch origin      # refresh origin/main from remote
git merge origin/main # merge the fresh ref
```

`git pull --rebase` also works (fetch + rebase in one shot), but when the instruction is to merge specifically, you must `git fetch` right before it.

## Pre-commit checks

Run these **before every commit** to catch the fastest CI failures locally:

```bash
gofmt -w .                          # catches gofmt (saves ~13s CI)
go vet ./...                        # catches vet warnings (saves ~52s CI/lint)
go test ./internal/tool/builtin/ ./internal/boot/  # catches tool/boot test breaks
```

CI runs `golangci-lint` (not locally available), but gofmt + vet already block ~80% of fast-fail scenarios.

## When production changes break tests — fix principles

1. **Run the failing test first.** Don't guess — read the error output.
2. **Understand why it fails.** Trace the new code path. Is the test asserting stale behavior, or did the production change introduce a real regression?
3. **Decide whether the new behavior is correct.** The test may need updating because the contract changed, or the production code may need fixing because the test caught a bug.
4. **Don't edit the test to make it pass without understanding.** Gutting assertions or inverting expected values to match accidental output erases the test's value.
5. **If the new behavior IS correct:** update the test's setup and assertions. Explain in the commit message why the test changed.
6. **If the new behavior is WRONG:** fix the production code, not the test.
7. **Push gate:** `go vet ./...` and `go test ./...` must pass before pushing.

## PR hygiene

- **One force-push per round of review feedback.** Multiple force-pushes destroy review history.
- **Keep the PR diff minimal.** Only files relevant to the PR's purpose — no stray changes.
- **Amend, don't add commits, for review feedback** — keeps the commit history clean.

## Import cycle rule

Before importing a new internal package from a non-test file, verify the target package's **test files** aren't already importing back to you:

```
# BAD: agent(_test.go) → tool/builtin(sessions.go) → agent  → setup failed
```

Use `go test ./path/to/target/` to detect cycles **before** pushing. A `[setup failed]` message means a cycle exists.

## Cache-impact PR metadata

When PR changes touch files under `internal/boot/`, `internal/tool/`, `internal/provider/`, or other cache-sensitive paths, the PR body MUST include:

```
Cache-impact: <none|low|medium|high> — <reason>
Cache-guard: <focused guard test/command or existing guard rationale>
```

If the PR also touches `internal/config/`, `internal/memory/`, `internal/outputstyle/`, `internal/skill/`, or `internal/boot/`, add:

```
System-prompt-review: <reviewer/approval note>
```
