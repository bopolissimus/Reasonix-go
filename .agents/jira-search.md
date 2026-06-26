# Jira search guidance

When searching Jira via the atlassian-proxy MCP tools, include these instructions in any sub-agent's task argument.

## Essential JQL patterns

```
# All unresolved issues in REX, newest first
project = REX AND resolution = Unresolved ORDER BY created DESC
```

## Critical defaults to override

| Param | Default | What to use | Why |
|-------|---------|-------------|-----|
| `maxResults` | 25 | **100** | The default returns only a handful; you'll miss issues |
| `fields` | many | `["summary","status","created","priority"]` | Full descriptions bloat output and cause truncation |

## Cloud ID

```
090b7a45-a02b-4ddf-9409-5f347b5992b8
```

## Why sub-agents miss results

The MCP tool `searchJiraIssuesUsingJql` defaults `maxResults` to 25. Without explicit override, a query returns only the first 25 issues and the pagination token is silently lost in the truncated result. Always pass `maxResults: 100` (or higher if you genuinely need more).

The `isLast` field tells you whether results are complete. If `isLast: false`, paginate with `nextPageToken`.

## Related tools

| Tool | Use |
|------|-----|
| `searchJiraIssuesUsingJql` | Search by JQL |
| `getJiraIssue` | Get single issue details |
| `getVisibleJiraProjects` | List projects + issue types |
| `getTransitionsForJiraIssue` | Get available status transitions |
| `transitionJiraIssue` | Change issue status |
| `addCommentToJiraIssue` | Add a comment |
| `createJiraIssue` | Create new issues |
| `editJiraIssue` | Update fields |

## Project key

The Reasonix project key is `REX` (project ID 10001). Issue types: Task, Sub-task, Epic.
