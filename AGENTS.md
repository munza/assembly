# AGENTS.md — assembly / foreman

Guidelines for any agent (or human) working in this repo.

## What this is

`foreman` is a Go CLI that orchestrates a crew of pi agents in herdr.
It is the single point of control: spawn workers, dispatch tasks, supervise, report.

## Coding style — functional, no hidden state

- **No package-level mutable state.** No globals, no `init()` side effects, no `var` shared across files for wiring.
- **Commands are pure constructors.** Each cobra command is a `newXCmd(deps) *cobra.Command` function. Flags are local variables inside the constructor.
- **Registration happens in one place.** All commands are added to root only in `main()`. Never call `rootCmd.AddCommand` from other files.
- **Dependencies are passed, not fetched.** A small `deps` struct carries config and state; pass it explicitly.
- **Short functions, value + error returns.** Business logic lives in `runX(...)` funcs that take inputs and return `(T, error)` or `error`. Keep RunE bodies to one call when possible.
- **Logic in `internal/`, wiring in `cmd/`.** `internal/` packages expose pure functions over their inputs; they do not read config or state behind the caller's back (config/state loaders are the exception — they are explicit `Load()` calls made in `main()`).

## Config

- Layers: defaults → `.assembly/config.json` → env vars. Env always wins.
- Env prefix `FOREMAN_` (e.g. `FOREMAN_MAX_WORKERS`); `LINEAR_API_KEY` is an accepted fallback.

## Dependencies

- stdlib first. Only add a dependency with strong justification (currently: cobra).
- herdr and pi are external tools, always invoked via the `internal/herdr` wrapper — never shell out directly from command code.

## Commits

- Short imperative subject, no prefix (e.g. `add task state machine`).
- Add a body when the change needs explanation: what and why.
- Squash work-in-progress commits before finishing a milestone.

## Gotchas learned

- `herdr agent read` / `pane read` print plain text, not JSON; `--lines N` can return empty — default to the full snapshot.
- `herdr workspace create` returns the root pane directly — no need to list panes after.
