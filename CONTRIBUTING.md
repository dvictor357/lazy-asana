# Contributing

Thanks for contributing. This repository is small, but the UI behavior is stateful enough that docs and code can drift quickly if changes are not verified carefully.

## Local Setup

Requirements:

- Go `1.26.2` or newer
- An Asana Personal Access Token for manual testing
- An OpenRouter API key if you are changing LLM behavior

Common commands:

```bash
go run .
go build ./...
go test ./...
```

At the moment, `go test ./...` is mainly a compile-level safety check because the repo does not yet contain test files.

## Repository Map

```text
main.go              Entry point
internal/config      Config persistence
internal/asana       Asana API client
internal/llm         OpenRouter client
internal/tui         Bubble Tea screens and flows
docs/                User and contributor documentation
```

## What To Verify Before Opening A PR

- `go test ./...` passes
- the changed flow still works end to end in the TUI
- docs are updated if labels, keybindings, setup, or behavior changed
- no secrets or real tokens are committed

If your change touches user-visible behavior, manually test the affected screen:

- `Bind Account`
- `Show Week Hours`
- `Plan Day/Week`
- `Move Tasks`

## Documentation Standard

Keep the docs accurate to the code that is merged, not the behavior you intend to build later.

That means:

- do not advertise screens that are not reachable from the menu
- call out important limitations when they affect setup or daily use
- update `README.md` when workflow names or scope change
- update `docs/workflows.md` when keybindings or screen steps change
- update `docs/config.md` when persisted fields change

## Pull Requests

Small, focused PRs are easier to review than broad mixed changes.

A good PR should include:

- a short summary of what changed
- why the change was needed
- how it was tested
- any follow-up work that remains

If you change external API assumptions for Asana or OpenRouter, include links to the relevant official docs in the PR description or the updated docs page.
