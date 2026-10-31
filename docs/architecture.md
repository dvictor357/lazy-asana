# Architecture Notes

This repository is small and centered around a single Bubble Tea application.

## Runtime Flow

1. `main.go` loads config from disk.
2. `tui.NewApp` creates API clients and the initial screen.
3. Bubble Tea runs the TUI in the terminal alternate screen.
4. Each screen performs its own async API calls and state transitions.

## Package Layout

```text
.
├── main.go
├── internal/asana
├── internal/config
├── internal/llm
└── internal/tui
```

## Package Responsibilities

### `main.go`

- Entry point
- Loads config
- Starts the Bubble Tea program

### `internal/config`

- Loads and saves `~/.config/lazy-asana/config.json`
- Defines the persisted `Config` struct

### `internal/asana`

- Thin HTTP client over the Asana REST API
- Handles auth headers, pagination, and JSON decoding
- Exposes the operations the TUI currently needs:
  - `GetMe`
  - `ListWorkspaces`
  - `ListProjects`
  - `GetCustomFieldSettings`
  - `SearchTasks`
  - `CreateTask`
  - `CreateSubtask`
  - `ListSections`
  - `AddTaskToProject`
  - `RemoveTaskFromProject`
  - `UpdateTask`

### `internal/llm`

- Thin HTTP client over OpenRouter chat completions
- Uses `deepseek/deepseek-chat-v3-0324` by default
- Provides helper methods for:
  - freeform chat calls
  - structured week-plan generation
  - plan JSON parsing

### `internal/tui`

- App shell and all screen state machines
- Main files:
  - `app.go`: top-level screen router
  - `menu.go`: main menu
  - `bind.go`: account binding and project selection
  - `hours.go`: weekly hours view
  - `planner*.go`: planner UI, LLM plan parsing, and submission
  - `migrate.go`: move-task flow
  - `chat.go`: chat screen implementation

## Screen Model

The app is organized as several screen-specific Bubble Tea models:

- `screenBind`
- `screenHours`
- `screenPlanner`
- `screenChat`
- `screenMigrate`

The menu currently exposes:

- `Bind Account`
- `Show Week Hours`
- `Task Composer`
- `Move Tasks`

`screenChat` exists in the app router but does not currently have a menu entry.

## Important Behavior

### Weekly Search Logic

The hours and move-task flows both rely on Asana task search with `due_on.after` and `due_on.before`.

Because Asana treats those filters as exclusive, the code shifts the requested range by one day on each side to simulate an inclusive range.

### Default Scope

The saved default workspace and project matter a lot:

- hours are fetched from the default project
- planner submissions create tasks in the default project
- hours-field detection reads custom fields from the default project during bind

### Assignee Filtering

`user_gid` is captured during binding and later used to filter task search results to the current user.

If `user_gid` is missing, the app still works but hours and move screens can show tasks for all assignees.

### LLM Plan Contract

The planner expects OpenRouter to return a JSON array of tasks. If the model returns anything else, parsing fails and the planner surfaces the raw response in the error.

## Current Gaps

These are useful to know before extending the app:

- `chat.go` is implemented but not wired into the menu.
- The manual planner flow is less complete than the LLM week-plan path.
- Planner-side description generation code exists, but that interaction is not yet rounded out as a documented user workflow.
- The repository does not yet have real unit or integration tests.
