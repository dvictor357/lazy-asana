# lazy-asana

`lazy-asana` is a terminal UI for logging your work in Asana: turn your commits
into tasks, tag them, drop them in the right month section, and check your
hours per day and week.

It is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
talks directly to the Asana REST API, and keeps its settings in
`~/.config/lazy-asana/config.json`.

## What The App Does

- **Insights**: hours per day and week against 8h/40h, for the default
  project or all your tasks; pick tasks and move them to another project.
- **Task Composer**: one table to add, edit, tag and submit tasks and
  subtasks, with hours per day and a detail panel.
  - Each task lands in the default project's section for its due-date month
    (e.g. `October 2026`).
  - Per-task Company/Entity and Product/Project tags, with search.
  - Mark tasks completed before submitting.
  - A failed submit can be retried without creating duplicates.
- **Import from git**: read your commits from GitLab (gitlab.com or
  self-hosted), GitHub and local clones, and add them as draft tasks, one
  per day per repo, with estimated hours.
- **LLM Auto-Plan** (optional): describe your week, get a task list back
  from OpenRouter or DeepSeek.
- **Settings**: Asana account, LLM provider and keys, git sources.

## Install

### From A Release

Download the archive for your OS from
[Releases](https://github.com/dvictor357/lazy-asana/releases)
(macOS and Linux on amd64/arm64, Windows on amd64), check it against
`checksums.txt`, unpack it and put `lazy-asana` on your `PATH`.

On macOS, a downloaded binary is quarantined the first time; clear it with:

```bash
xattr -d com.apple.quarantine ./lazy-asana
```

### With Go

Requires Go `1.26.2` or newer.

```bash
go install github.com/dvictor357/lazy-asana@latest
lazy-asana --version
```

### From Source

```bash
git clone https://github.com/dvictor357/lazy-asana.git
cd lazy-asana
go build -o lazy-asana .
./lazy-asana
```

## First Run

You need an Asana Personal Access Token
([create one](https://app.asana.com/0/my-apps)).

On first launch the app opens Settings → Asana:

1. Enter your Asana token.
2. Select a workspace.
3. Select the default project (e.g. your team planner). The app detects its
   hours field and the Company/Entity and Product/Project fields.

You land back in Settings, where you can add:

- **LLM**: an OpenRouter or DeepSeek key, for LLM Auto-Plan and `ctrl+g`
  description drafts.
- **Git sources**: where your commits come from.

| Source | Needs | Reads |
| --- | --- | --- |
| GitLab.com | token with `read_api` | all branches |
| GitLab self-hosted | instance URL + token with `read_api` | all branches |
| GitHub | [classic token](https://github.com/settings/tokens/new) with `repo`, or a [fine-grained token](https://github.com/settings/personal-access-tokens/new) with Contents: read-only; API URL only for GitHub Enterprise | default branches only |
| Local folder | a folder of clones, scan depth (default 4) | all branches, including unpushed work |

Tokens are stored in the config file (mode `0600`) and shown masked.

## Daily Use

1. Open **Task Composer**, press `i`, keep the default range (Monday to
   today) and press `enter`.
2. Tick the day/repo entries to log and press `enter`; they become draft
   tasks with the commit list as description.
3. Adjust hours (`e`), set tags (`t`), mark finished work completed (`c`).
4. Press `enter`, check the target month sections, confirm.
5. Open **Insights** to see the week's hours add up.

Hours from git import are estimates from commit size (at most 8h a day).
Adjust them to the time you actually spent.

Full keybindings: [docs/workflows.md](docs/workflows.md).

## Documentation

- [Setup](docs/setup.md)
- [Workflows and keys](docs/workflows.md)
- [Config reference](docs/config.md)
- [Architecture notes](docs/architecture.md)
- [Contributing](CONTRIBUTING.md)

## Configuration

`~/.config/lazy-asana/config.json`, written by Settings. Example:

```json
{
  "asana_token": "2/...",
  "llm_provider": "openrouter",
  "openrouter_key": "sk-or-...",
  "default_workspace_gid": "...",
  "default_project_gid": "...",
  "actual_hours_field_gid": "...",
  "user_gid": "...",
  "git_sources": [
    { "kind": "gitlab", "name": "GitLab.com", "base_url": "https://gitlab.com", "token": "glpat-..." },
    { "kind": "github", "token": "ghp_..." },
    { "kind": "local", "path": "~/Projects/company", "depth": 4 }
  ]
}
```

Field-by-field details: [docs/config.md](docs/config.md).

## Known Limitations

- Insights and Move Tasks search tasks by due date; tasks without one do not
  show up there. Asana search returns at most 100 tasks per query.
- Hours totals need a numeric "actual hours" custom field in the default
  project; without it they show `0` and submissions skip the field.
- A task carries one value per single-select tag field, so work spanning two
  products needs two tasks.
- Tasks go into the default project only.
- GitHub import misses commits on unmerged branches (search only indexes
  default branches); add the local clone as a source to catch them.

## Development

```bash
go build ./...
go vet ./...
go test -race ./...
go run .
```

CI runs build, vet and tests on every push and pull request to `main`.
Contribution guidance lives in [CONTRIBUTING.md](CONTRIBUTING.md).

### Releasing

Push a `v*` tag. The Release workflow runs vet and tests, then GoReleaser
builds the binaries, writes `checksums.txt` and a changelog grouped by
`feat:`/`fix:` commits, and publishes a GitHub release. Tags with a suffix
(`v0.2.0-rc.1`) become pre-releases.

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT. See [LICENSE](LICENSE).
