# Tool comparison: TypeScript Reasonix vs Go Reasonix

| Tool | TS | Go | Comments |
|------|----|----|----------|
| `ask` / `ask_choice` / `ask_user` | `ask_user` (free-text), `ask_choice` (picker) | `ask` | Go combines both modes; TS splits into three tools. Go's `ask` supports multi-question batches. |
| `bash` / `run_command` | `run_command` | `bash` | Both run shell commands. Go adds deep safety: quoting-aware parsing, argument-level gating (blocks `&&`, `|`, `$()`, `find -delete` even when quoted), sandbox confinement via `confine`. |
| `bgjobs` / `run_background` | `run_background`, `job_output`, `wait_for_job`, `stop_job`, `list_jobs` | `bash` (background), `bash_output`, `kill_shell`, `wait` | TS splits background jobs across 5 tools; Go bundles into 4 (`bash` with `run_in_background`, `bash_output`, `kill_shell`, `wait`). |
| `code_index` / `get_symbols` / `find_in_code` | `get_symbols`, `find_in_code` | `code_index` | TS uses tree-sitter (multi-language); Go uses Go AST only. TS splits into symbol-outline + single-file search; Go combines. |
| `complete_step` / `mark_step_complete` | `mark_step_complete` | `complete_step` | Both mark one plan step done. Go adds evidence-backed completion with verification struct; TS is simpler (result string). |
| `connect_tool_source` | — | `connect_tool_source` | Token-economy connector to enable optional tool sources. No TS equivalent. |
| `confine` | — | `confine` | Workspace/sandbox confinement toggle for bash, web_fetch, writers. No TS equivalent. |
| `copy_file` | `copy_file` | — | Go uses `bash cp` instead. No dedicated TS tool for the Go side. |
| `create_directory` | `create_directory` | — | Go uses `bash mkdir -p` instead. |
| `create_skill` | `create_skill` | — | Scaffolds a SKILL.md with frontmatter. Go's `install_skill` can install from external source but not scaffold from scratch. |
| `delete_directory` | `delete_directory` | — | Go uses `bash rm -rf` instead. |
| `delete_file` | `delete_file` | — | Go uses `bash rm` instead. |
| `delete_range` | — | `delete_range` | Delete text range by start/end anchor text. No TS equivalent (TS uses `edit_file` for this). |
| `delete_symbol` | — | `delete_symbol` | Delete Go symbol via AST parsing. No TS equivalent (TS uses `edit_file`). |
| `directory_tree` | `directory_tree` | — | Recursive tree view with depth/dependency controls. Go uses `ls` + `glob` instead. |
| `edit_file` | `edit_file` | `edit_file` | Same. Both use SEARCH/REPLACE with exact text anchoring. |
| `explore` | `explore` | `explore` | Same — both are read-only codebase investigation subagent skills. |
| `forget` | `forget` | `forget` | Same — delete a memory by name. |
| `get_file_info` | `get_file_info` | — | Stat a file (size, mtime, type). Go gets some of this via `ls` but no dedicated stat tool. |
| `glob` | `glob` | `glob` | Same — file pattern matching (e.g. `**/*.go`). |
| `install_skill` / `install_source` | `install_skill` | `install_skill`, `install_source` | Both install skills from URL/source. TS's `install_skill` also scaffolds new skills; Go's `install_source` handles MCP servers too. |
| `isolate_workspace` | `isolate_workspace` | — | Create isolated Git worktree for safe refactors. No Go equivalent. |
| `list_directory` / `ls` | `list_directory` | `ls` | Same — directory listing. Different name. |
| `list_sessions` | — | `list_sessions` | List saved conversation sessions. No TS equivalent. |
| `lsp_definition` | — | `lsp_definition` | LSP go-to-definition (gopls, rust-analyzer, typescript-language-server). No TS equivalent. |
| `lsp_diagnostics` | — | `lsp_diagnostics` | LSP compiler/linter diagnostics for a file. No TS equivalent. |
| `lsp_hover` | — | `lsp_hover` | LSP type signature and documentation on hover. No TS equivalent. |
| `lsp_references` | — | `lsp_references` | LSP find-all-references across workspace. No TS equivalent. |
| `mcp add` / `add_mcp_server` | `add_mcp_server` (tool) | `reasonix mcp add` (CLI) | TS exposes MCP registration as a model-callable tool; Go keeps it as a CLI command only. |
| `merge_worktree` | `merge_worktree` | — | Merge completed isolated worktree back. No Go equivalent. |
| `move_file` | `move_file` | `move_file` | Same — rename/move. |
| `multi_edit` | `multi_edit` | `multi_edit` | Same — atomic multi-edit across files. |
| `notebook_edit` | — | `notebook_edit` | Edit a Jupyter notebook cell. No TS equivalent. |
| `parallel_tasks` | — | `parallel_tasks` | Spawn multiple sub-agents concurrently. No TS equivalent (TS can only invoke one subagent tool per turn). |
| `read_file` | `read_file` | `read_file` | Same — read file with line offset/limit. |
| `read_session` | — | `read_session` | Read a saved conversation session by file name. No TS equivalent. |
| `read_skill` | — | `read_skill` | Load a skill body without executing it (read-only, works in plan mode). No TS equivalent. |
| `read_only_skill` | — | `read_only_skill` | Run a skill in read-only mode (plan-mode-safe). No TS equivalent. |
| `read_only_task` | — | `read_only_task` | Read-only sub-agent (general-purpose, not skill-named). TS's `explore`/`research` are equivalent but named and scoped. |
| `remember` | `remember` | `remember` | Same — save durable fact to project memory. |
| `recall_memory` / `memory` | `recall_memory` | `memory` | Both read memories. TS: read single memory by name. Go: `memory` combines search, list, and read into one tool. |
| `research` | `research` | `research` | Same — web search + code reading subagent. |
| `review` | `review` | `review` | Same — code review on branch diff, isolated subagent. |
| `revise_plan` | `revise_plan` | — | Replace remaining plan steps mid-flight. Go stays in plan mode and re-presents via `todo_write` instead. |
| `run_skill` | `run_skill` | `run_skill` | Same — invoke a playbook from the Skills index. |
| `search_content` / `grep` | `search_content` | `grep` | Same — regex content search. Different name. |
| `search_files` | `search_files` | — | Find files by NAME substring/regex. Go uses `glob` + piping results; no dedicated filename-only search tool. |
| `security_review` | `security_review` | `security_review` | Same — security-focused review on branch diff, isolated subagent. |
| `slash_command` | — | `slash_command` | Invoke project slash command by name and return expanded prompt. No TS equivalent (TS slash commands are user-facing only). |
| `submit_plan` | `submit_plan` | — | Submit structured plan with approve/refine/cancel gate. Go uses the turn's final response text, approved via `exit_plan_mode`. |
| `todo_write` | `todo_write` | `todo_write` | Same — structured task list. Go adds a `level` field (0=phase, 1=sub-step) for hierarchical plan display. |
| `web_fetch` | `web_fetch` | `web_fetch` | Same — fetch URL and return text content. |
| `web_search` | `web_search` | — | Multi-engine web search (Exa/Tavily/DuckDuckGo). Go has no search engine API integration; `web_fetch` only. |
| `workspace` | — | `workspace` | Return workspace root path. TS knows its cwd implicitly; no dedicated tool. |
| `write_file` | `write_file` | `write_file` | Same — write/overwrite file content. |

## Summary

| | TS | Go |
|---|---|---|
| **Total unique tool surfaces** | 44 | 43 |
| **TS-only (no Go equivalent)** | 11 | — |
| **Go-only (no TS equivalent)** | — | 17 |
| **Shared (equivalent or near-equivalent)** | 25 | 25 |
| **Different name, same function** | 3 (`search_content`→`grep`, `list_directory`→`ls`, `run_command`→`bash`) | — |
