# lazy-asana

A terminal-based Asana task planner with LLM assistance, inspired by [lazygit](https://github.com/jesseduffield/lazygit).

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and powered by [OpenRouter](https://openrouter.ai/) for AI features.

## Features

- **Account Binding** - Configure your Asana Personal Access Token and OpenRouter API key via an interactive TUI
- **Week Hours View** - Query and display actual hours logged across tasks for a given week or date range
- **Task Planner** - Manually create tasks and subtasks, or let an LLM auto-generate a structured day/week plan
- **LLM Chat** - Chat with an AI assistant (via OpenRouter) for ad-hoc planning help
- **Asana Integration** - Full task CRUD: create tasks and subtasks directly in your Asana projects with custom fields (actual hours, due dates)

## Installation

```bash
go build -o lazy-asana .
```

## Usage

```bash
./lazy-asana
```

On first launch you'll be prompted to bind your account:

1. Enter your **Asana Personal Access Token** (required)
2. Enter your **OpenRouter API key** (optional, enables LLM features)
3. Select your **workspace** and **default project**

### Navigation

| Key | Action |
|-----|--------|
| `↑/k` | Move up |
| `↓/j` | Move down |
| `enter` | Select / confirm |
| `tab` | Switch fields / cycle tasks |
| `esc` | Go back |
| `q` | Quit |

## Configuration

Config is stored at `~/.config/lazy-asana/config.json`:

```json
{
  "asana_token": "pat/...",
  "openrouter_key": "sk-or-...",
  "default_workspace_gid": "...",
  "default_project_gid": "...",
  "actual_hours_field_gid": "..."
}
```

## Tech Stack

- **Go** 1.26
- **Bubble Tea** - TUI framework
- **Lip Gloss** - Terminal styling
- **Bubbles** - TUI components (text inputs, viewports)
- **OpenRouter** - LLM API gateway (default model: `deepseek/deepseek-chat-v3-0324`)
- **Asana API** v1.0

## License

MIT
