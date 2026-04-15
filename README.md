# lazy-asana

`lazy-asana` is a terminal UI for planning, reviewing, and moving Asana tasks with optional OpenRouter-assisted week planning.

It is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), talks directly to the Asana REST API, and stores local state in a small config file under `~/.config/lazy-asana/`.

## What The App Does

- Bind your Asana account and choose a default workspace and project.
- Review task hours for an ISO week or a custom date range.
- Build a local task list and submit tasks and subtasks into Asana.
- Generate structured week plans with OpenRouter and submit them to Asana.
- Move one or more tasks from a weekly task picker into another project and optional section.

## Quickstart

### Prerequisites

- Go `1.26.2` or newer
- An Asana Personal Access Token
- An OpenRouter API key if you want LLM-assisted planning

### Build And Run

```bash
go build -o lazy-asana .
./lazy-asana
```

You can also run it without a build step:

```bash
go run .
```

### First Run

On first launch the app opens the bind flow:

1. Enter your Asana Personal Access Token.
2. Optionally enter your OpenRouter API key.
3. Select a workspace.
4. Select the default project you want the app to work against.

The app then writes config to `~/.config/lazy-asana/config.json`.

## Core Workflows

### Bind Account

- Validates the Asana token against `/users/me`.
- Lists available workspaces.
- Lets you search and pick a default project.
- Detects a numeric "actual hours" field for that project when possible.

### Show Week Hours

- Fetches tasks in the default project for a selected ISO week or explicit date range.
- Sums the detected numeric hours field across returned tasks.
- Filters to your user when `user_gid` is available from the bind flow.

### Plan Day/Week

- Maintains a local list of tasks and subtasks before submission.
- Submits tasks into the default project and creates subtasks under each parent task.
- Can ask OpenRouter for a structured week plan and parse the returned JSON into tasks.

### Move Tasks

- Fetches tasks for a selected week.
- Lets you multi-select tasks from the results.
- Lets you search for a target project, optionally choose a section, and optionally remove tasks from one source project.

## Documentation

- [Setup](docs/setup.md)
- [Workflows](docs/workflows.md)
- [Config Reference](docs/config.md)
- [Architecture Notes](docs/architecture.md)
- [Contributing](CONTRIBUTING.md)

## Configuration

Config lives at `~/.config/lazy-asana/config.json`.

Example:

```json
{
  "asana_token": "pat/...",
  "openrouter_key": "sk-or-...",
  "default_workspace_gid": "...",
  "default_project_gid": "...",
  "actual_hours_field_gid": "...",
  "user_gid": "..."
}
```

See [docs/config.md](docs/config.md) for field-by-field details.

## Known Limitations

- The hours and move flows rely on Asana task search by due date, so tasks without a due date are excluded from those screens.
- Hours totals depend on the app finding a numeric custom field that looks like an "actual hours" field in the selected project. If no matching field is found, totals display as `0` and planner submissions skip that custom field.
- OpenRouter support is currently focused on full-plan generation. The separate chat screen exists in source but is not exposed from the main menu.
- The manual planner form needs more polish than the LLM-generated planning path. The richer multi-field flow is not as complete as the week-plan generator yet.
- The repository currently has no automated tests beyond a compile-level `go test ./...`.

## Development

Useful commands:

```bash
go build ./...
go test ./...
go run .
```

Contribution guidance lives in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
