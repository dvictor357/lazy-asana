# Config Reference

The app stores a single JSON config file on disk.

## Location

```text
~/.config/lazy-asana/config.json
```

The app creates the parent directory if needed and writes the file with mode `0600`.

## Example

```json
{
  "asana_token": "pat/...",
  "openrouter_key": "sk-or-...",
  "default_workspace_gid": "1200000000000001",
  "default_project_gid": "1200000000000002",
  "actual_hours_field_gid": "1200000000000003",
  "user_gid": "1200000000000004"
}
```

## Fields

| Field | Required | Meaning |
| --- | --- | --- |
| `asana_token` | Yes | Bearer token used for all Asana API calls. |
| `openrouter_key` | No | API key used for LLM-assisted planning through OpenRouter. |
| `default_workspace_gid` | Yes after bind | Workspace used for search endpoints and project listing. |
| `default_project_gid` | Yes after bind | Primary project used by the hours and planner flows. |
| `actual_hours_field_gid` | No | Numeric custom field used for hours totals and task submission. |
| `user_gid` | Recommended | Current Asana user ID used to filter results to your assignments. |

## Behavior Notes

### `asana_token`

- Required for binding, hours, planning, and moving tasks.
- If it is missing or invalid, the app cannot talk to Asana.

### `openrouter_key`

- Optional.
- When absent, the planner still opens, but LLM-driven planning is unavailable.

### `default_workspace_gid`

- Used for workspace-scoped task search.
- Set during the bind flow after you choose a workspace.

### `default_project_gid`

- Used by:
  - the hours view
  - planner task creation
  - project-specific custom field detection during binding

### `actual_hours_field_gid`

- Set only if the bind flow finds a numeric custom field that looks like an hours field.
- If empty, hours totals read as `0` and planner submissions skip hours custom fields.

### `user_gid`

- Populated from `/users/me` during binding.
- Used by weekly hours and move-task searches to filter to your assignments.
- If missing, those screens warn that results may include all assignees.

## Editing Manually

You can edit the JSON by hand, but re-running `Bind Account` is usually safer because it refreshes:

- `default_workspace_gid`
- `default_project_gid`
- `actual_hours_field_gid`
- `user_gid`

After manual edits, restart the app.
