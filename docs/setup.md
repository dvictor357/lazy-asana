# Setup

This project needs an Asana Personal Access Token to do anything useful. OpenRouter is optional and only needed for LLM-assisted planning.

## Requirements

- Go `1.26.2` or newer
- Access to an Asana workspace and project
- An Asana Personal Access Token
- An OpenRouter API key if you want AI planning

## Get Your Tokens

### Asana Personal Access Token

Create a PAT from the Asana developer console:

- Official docs: <https://developers.asana.com/docs/personal-access-token>

Treat the token like a password. It gives this app access to your real Asana data.

### OpenRouter API Key

Create an API key from OpenRouter:

- Official docs: <https://openrouter.ai/docs/api-keys>

The app uses OpenRouter only for planning features. Core Asana flows still work without it.

## First Launch

Start the app:

```bash
go run .
```

Or build and run it:

```bash
go build -o lazy-asana .
./lazy-asana
```

The bind screen asks for:

1. `Asana Personal Access Token`
2. `OpenRouter API Key (optional, for LLM features)`

After you continue, the app:

1. Validates the Asana token with `/users/me`
2. Fetches available workspaces
3. Lets you pick a workspace
4. Lets you search and pick a default project
5. Reads that project's custom field settings
6. Saves local config

## What Gets Saved

The app stores config at:

```text
~/.config/lazy-asana/config.json
```

Current fields:

- `asana_token`
- `openrouter_key`
- `default_workspace_gid`
- `default_project_gid`
- `actual_hours_field_gid`
- `user_gid`

The file is written with mode `0600`.

## Actual Hours Field Detection

During binding, the app tries to detect a numeric hours field in the selected project.

It first looks for exact names:

- `Actual Hours`
- `actual_hours`
- `ActualHours`

If none match, it falls back to numeric fields named:

- `Hours`
- `hours`
- `Actual`
- `Effort`

If no matching field is found:

- the hours view still works, but totals will read as `0`
- planner submissions will not write an hours custom field

If you add or rename the field later, run `Bind Account` again.

## Rotating Or Resetting Config

You have two simple options:

- open `Bind Account` again and overwrite the saved values
- delete `~/.config/lazy-asana/config.json` and relaunch the app

If the app shows all assignees in the hours or move screens, re-binding is the simplest way to refresh `user_gid`.
