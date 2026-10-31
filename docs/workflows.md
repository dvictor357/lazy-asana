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

## Insights

Main menu label: `Insights` (first item)

What it does:

- week view: hours per day (Mon–Sun) against 8h, the week total against
  40h, and the week's tasks
- month view: hours per week of the month, with total, average and best week
- data comes from tasks in the default project with a due date in the
  period, filtered to your user when `user_gid` is saved

Keys:

| Key | Action |
| --- | --- |
| `←` / `→` | Previous / next week (or month) |
| `↑`/`k`, `↓`/`j` | Move through tasks (week) or weeks (month) |
| `w` / `m` | Week / month view |
| `enter` | Month view: open the selected week |
| `space` | Week view: pick or unpick the task for moving |
| `a` | Week view: pick or clear all tasks |
| `M` | Week view: move the picked tasks (or the highlighted one) to a project |
| `r` | Refresh |
| `esc` | Back to the main menu |

### Move Tasks

Started with `M` from the Insights week view.

- target project picker: type to search, `↑`/`↓` to move, `enter` to pick
- section picker: preselects the section named after the first task's
  due-date month; `No section (default)` keeps Asana's default placement
- source project picker: optionally remove the tasks from one project they
  belong to
- `esc` on the project picker, and `enter`/`esc` when done, return to
  Insights on the same week; after a move the week reloads

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
| `c` | Toggle completed on the selected task (not once it is in Asana) |
| `d` | Delete selected task from the local list |
| `t` | Set tags for the selected task |
| `u` | Return the selected task to the batch default tags |
| `L` | LLM Auto-Plan (needs an LLM key) |
| `enter` | Submit |
| `esc` | Back to the main menu |

### Task Form

- collects title, description, actual hours, due date (`YYYY-MM-DD`) and a
  Status toggle: tasks are created not completed unless you tick
  `[x] Completed` (`space` on the Status field)
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
- in the tag picker, `/` filters the active field's options (every word must match, case-insensitive); `enter` keeps the filter and returns to `j`/`k`/`space`, `esc` clears it
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

## Chat with AI

Main menu label: `Chat with AI` (needs an OpenRouter or DeepSeek key). A plain chat with the configured LLM; it does not read or change Asana data.
