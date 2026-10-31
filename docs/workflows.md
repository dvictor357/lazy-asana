# Workflows

This page documents the current user-facing flows and keybindings as implemented in the app today.

## Global Navigation

Common keys used across screens:

| Key | Action |
| --- | --- |
| `up` / `k` | Move selection up |
| `down` / `j` | Move selection down |
| `enter` | Confirm current action |
| `esc` | Go back |
| `q` | Quit or return to menu on some screens |

## Bind Account

Main menu label: `Bind Account`

What it does:

- collects the Asana token
- collects the optional OpenRouter key
- fetches workspaces
- lets you choose the default project for later workflows
- auto-detects a numeric hours field when possible

Project selection supports:

- typing to search
- `up` / `k` and `down` / `j` to move through the filtered list
- `enter` to confirm the default project

## Show Week Hours

Main menu label: `Show Week Hours`

What it does:

- fetches tasks from the default project
- supports either an ISO week number or a custom start and end date
- totals the detected hours custom field
- shows memberships and assignee details for each result

Important behavior:

- the query is scoped to the saved default project
- if `user_gid` is saved, results are filtered to your Asana user
- tasks without a due date are excluded because the flow depends on Asana due-date search filters

Input flow:

- `tab` switches between week mode and date-range mode
- in date-range mode, `tab` also switches between start and end date fields
- `enter` fetches tasks
- `esc` goes back

## Task Composer

Main menu label: `Task Composer`

One screen to build, review and submit tasks. Tasks stay in local memory
until you submit them.

Layout:

- a table with one row per task: due date, title, hours, Company/Entity,
  Product/Project, target month section and status (`·` not sent,
  `◐` partly sent, `✓` in Asana); subtasks show indented under their parent
- hours per due date against an 8h day, plus the total
- a detail panel for the selected task (title, description, hours, due
  date, section, tags, status)
- columns narrow on small terminals so rows never wrap

Keys:

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Select task |
| `a` | Add task |
| `e` | Edit selected task (not once it is in Asana) |
| `s` | Add subtask to selected task |
| `d` | Delete selected task from the local list |
| `t` | Set tags for the selected task |
| `u` | Return the selected task to the batch default tags |
| `L` | LLM Auto-Plan (needs an LLM key) |
| `enter` | Submit |
| `esc` | Back to the main menu |

### Task Form

- collects title, description, actual hours and due date (`YYYY-MM-DD`)
- `tab`/`shift+tab` switch fields, `enter` saves (`ctrl+s` inside the
  description), `ctrl+g` drafts a description with the LLM, `esc` cancels
- editing keeps the task's subtasks and tags

### LLM Auto-Plan

Requirements:

- a saved OpenRouter API key

What it does:

- sends your planning prompt to OpenRouter
- expects a JSON array of tasks in response
- parses tasks, descriptions, hours, due dates, and subtasks into the local planner state

The current default model is `deepseek/deepseek-chat-v3-0324`.

### Submit

Submission behavior:

- creates each parent task in the default project
- creates subtasks beneath the new parent task
- writes `due_on` when available
- writes the numeric hours custom field only when `actual_hours_field_gid` is known
- moves each task with a due date into the default project's section for that month (`October`, `October 2026`, `Oct` or `Oct 2026`, case-insensitive; a name with the matching year wins)
- tasks without a due date stay in the project's default section

Tags (Company/Entity and Product/Project):

- in the composer, `t` sets tags for the selected task and `u` returns it to the batch default
- on submit, the default-tag picker appears only if some task has no tags of its own
- single-select (`enum`) fields keep one option; picking another replaces it. `multi_enum` fields accept several
- the composer table and Confirm show each task's tags; one task can carry only one value per `enum` field, so work spanning two products needs two tasks

Month sections:

- the confirm screen lists each task's target section before anything is sent
- if a task's month section does not exist, submission is blocked until you create it in Asana

Failures and resubmits:

- each task remembers what already reached Asana (task, month section, each subtask)
- after a failure you land back in the composer, where sent tasks show `✓ in Asana` and half-sent ones `◐ partly sent`
- submitting again skips what Asana already has, so nothing is created twice
- `enter` and `esc` are ignored while a submit is running

## Move Tasks

Main menu label: `Move Tasks`

What it does:

- loads tasks for a selected week
- lets you select one or many tasks
- lets you pick a target project
- lets you pick an optional target section
- lets you pick an optional source project to remove tasks from

Task selection screen:

- `space` toggles the current task
- `a` selects or clears all tasks
- `enter` continues with either the checked tasks or the single task under the cursor

Target project picker:

- type to search
- `up` / `k` and `down` / `j` navigate the filtered list
- `enter` confirms the target project

Section picker:

- the app tries to preselect a section whose name matches the first selected task's due-date month
- `No section (default)` keeps Asana's default placement behavior

Source project picker:

- choose a source project if you want the task removed from one existing project membership
- choose `Skip (just add, don't remove from any project)` if you want to keep the existing project memberships untouched

Execution behavior:

- the app first adds the task to the target project
- if a source project is selected and the task is actually in that project, it then removes that membership

Like the hours flow, tasks without due dates are not included in the weekly fetch.

## Unexposed Chat Screen

There is a `chat.go` implementation in the codebase, but the main menu does not currently route into it. It is not documented as a supported user workflow yet.
