# How to configure the REX project workflow in Jira

## You can't do this via the API — it's admin-only in the Jira UI.

### Steps

1. Go to **Project Settings** → **Workflows** for the REX project
   - URL: `https://bopolissimus.atlassian.net/jira/settings/projects/REX/workflows`

2. Click **Add workflow** → **Create from scratch** or copy the existing one.

3. Add these statuses (or rename existing ones):

   | Status | Category | Description |
   |--------|----------|-------------|
   | **New** | To Do (blue-gray) | Ticket filed, not yet scoped |
   | **Design** | In Progress (yellow) | Approach decided, plan written |
   | **Develop** | In Progress (yellow) | Actively implementing |
   | **Test** | In Progress (yellow) | Code written, tests passing, ready for validation |
   | **Documentation** | In Progress (yellow) | Feature works, docs updated |
   | **Done** | Done (green) | Shipped, verified in production |
   | **Blocked** | To Do (blue-gray) | Waiting on dependency or decision |
   | **Deferred** | To Do (blue-gray) | Intentionally postponed |

4. Set up transitions:

   ```
   New → Design → Develop → Test → Documentation → Done
   Any → Blocked
   Blocked → (previous state)
   New → Deferred
   Any → Deferred
   Done → New (reopen)
   ```

5. After creating the workflow, go to **Workflow Schemes** and assign it to the REX project.

6. Then go to **Board Settings** → **Columns** and map the statuses to board columns:

   | Column | Statuses |
   |--------|----------|
   | Backlog | New, Deferred |
   | Design | Design |
   | In Progress | Develop, Blocked |
   | Testing | Test, Documentation |
   | Done | Done |

### Temporary workaround (while workflow isn't set up)

Use labels to simulate the states:

```
/label design
/label develop
/label testing
/label documentation
```

And move tickets between the default statuses:
- `To Do` = New, Design, Deferred, Blocked
- `In Progress` = Develop, Test, Documentation
- `Done` = Done
