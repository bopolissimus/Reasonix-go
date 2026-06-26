# Agent instructions — Go Migration project

> Go rewrite of Reasonix. Migrating features from the DR1 TypeScript fork.

## Tone

Don't reply sycophantically. Don't say "you're right" or "good catch." Just state what is better and move on. Brief with cause — not "Fixed." but not a dissertation.

## Adversarial thinking

Before executing a direction, think about edge cases, why it might be a mistake, or why a different approach might be better. Surface that reasoning concisely:

- **Low risk**: Show the push-back, then proceed.
- **Serious risk** (data loss, irreversible changes, security regression): Pause and ask.

## Autopilot gating

Don't eagerly perform actions. Only act without confirmation when explicitly told to ("keep going", "auto-apply", "run the loop"). Otherwise: report findings, propose next steps, wait.

This applies to commits, code changes, and any mutating operation.

## Sub-agent vs inline

**Use sub-agents** for: broad surveys across multiple files, exploring unfamiliar subsystems, parallelizable reads (fan out N agents concurrently), and security scanning. Context preservation in the main agent takes priority over spawn cost.

**Use inline** for: single-file edits or reads, following up on a sub-agent's file:line finding, focused debugging with 2–3 known files, writing or editing.

## MCP tools — never delegate to sub-agents

Always run MCP tool calls (Jira, Confluence, Atlassian proxy, etc.) in the main harness — never inside a sub-agent. MCP tools can trigger permission prompts that are invisible when run in a sub-agent's isolated loop. The user must be able to see and respond to these prompts.

Web search, code exploration, security scanning, and other non-MCP tasks can still be delegated to sub-agents normally.

## Go conventions

- **Before committing:** `gofmt -w .`, `go vet ./...`, `go test ./internal/tool/builtin/ ./internal/boot/`
- **Build:** `make build` or `CGO_ENABLED=0 go build -o bin/reasonix ./cmd/reasonix`
- **Package comments:** One sentence per package, matching surrounding density and idiom.
- **Import cycle rule:** Before importing a new internal package, verify the target's tests aren't already importing back. Use `go test ./path/to/target/` to detect cycles.
- **PR hygiene:** One force-push per review round, minimal diff, amend don't add commits.

## Dependency security (MANDATORY)

**Every change to `go.mod` or `go.sum` requires a security-reviewer audit.** This includes:

- Direct dependencies you add
- Transitive dependencies pulled in by upgrades
- New packages from `go mod tidy` after toolchain changes

**Process:**

1. `go mod tidy && go mod vendor && go build -mod=vendor ./...`
2. `git diff --stat vendor/` to identify new packages
3. Run the security-reviewer skill on all new packages (delegate to v4-pro sub-agents — the skill's frontmatter specifies `model: deepseek-v4-pro`)
4. File findings as Jira tickets tagged `security` + `dependency`. Critical findings block the PR.
5. Document the review in `docs/security/dependency-review-YYYY-MM.md`

Full policy and audit history: [.agents/dependency-security.md](dependency-security.md)

## Topic files (read on demand)

Before modifying a subsystem, check whether a topic file exists. Read it first — don't wait until you hit a problem.

| File | Covers | Read when… |
|---|---|---|
| [docs/survey/GO-CODEBASE-SURVEY.md](../docs/survey/GO-CODEBASE-SURVEY.md) | 20-category Go codebase survey with cross-references to DR1 | Navigating the Go codebase — finding where code lives |
| [docs/survey/gap-analysis.md](../docs/survey/gap-analysis.md) | 100+ feature cross-reference: DR1 → Go implementation status | Deciding what to migrate or checking if a feature exists |
| [docs/migration/plan.md](../docs/migration/plan.md) | 7-phase migration plan with open questions | Planning migration work |
| [docs/migration/todo.md](../docs/migration/todo.md) | 40+ actionable checklist across all phases | Tracking migration progress |
| [git.md](git.md) | Commit format, merges, pre-commit rules, push guidance | Git operations — committing, pushing, merging |
| [jira-search.md](jira-search.md) | Jira query patterns that avoid truncated results | Searching Jira for issues |
| [writing-agent-docs.md](writing-agent-docs.md) | How to write topic-specific .md files and route from AGENTS.md | Adding or updating `.agents/` documentation |
| [dependency-security.md](dependency-security.md) | **MANDATORY** — vendoring policy, security review process, full audit history. Read before adding or upgrading ANY dependency. | Any change to go.mod or go.sum |

Call `read_file(".agents/<name>.md")` before touching the relevant code.

## DR1 reference (read-only)

The DR1 TypeScript fork lives at `../DR1/`. Its survey is at `../DR1/.agents/survey/CODEBASE-SURVEY.md`. These files are authoritative for DR1 features — never edit them. When implementing a DR1 feature in Go, read the DR1 source first, then design the Go-idiomatic equivalent.

## Jira

Before performing Jira searches, read [.agents/jira-search.md](jira-search.md) for query patterns that avoid truncated results. Run Jira MCP calls directly in the main harness — do not delegate them to sub-agents.

The Reasonix project key is `REX`. Cloud ID: `090b7a45-a02b-4ddf-9409-5f347b5992b8`.

## Git

Never commit or push without asking. Explain what you want to commit (files + message) and ask clearly for permission. Same for push — explain what will be pushed and ask.

See [.agents/git.md](git.md) for commit format, merges, and pre-commit rules.

## Confluence

Migration documentation lives at: [Go Migration — Reasonix TypeScript → Go](https://bopolissimus.atlassian.net/wiki/spaces/SD/pages/10027009) (page ID 10027009, space SD).
