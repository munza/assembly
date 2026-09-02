# AGENTS.md — assembly / foreman

Guidelines for any agent (or human) working in this repo.

## What this is

`foreman` is a Go CLI that orchestrates a crew of pi agents in herdr.
It is the single point of control: spawn workers, dispatch tasks, supervise, report.

## Model

- **Project** → one herdr workspace, labeled with the project name.
- **Issue** → one herdr worktree (git worktree + its own workspace), id = Linear issue id or local `tNNN`.
- **Task** → exactly **one tab** in the issue worktree, labeled `<type>-<slug>` (3-4 word slug). One task is one tab — never more.
- **Task types**: `plan`, `research`, `work`, `review`. Each is also a root alias: `foreman work "title"` = `foreman task new --type work`.
- Task files: `.assembly/tasks/<id>-<type>-<slug>.json`.
- Agents are not named; tabs are. herdr agent ops target the pane inside the task tab (`wX:pY`).
- **Mailbox**: `.assembly/mailbox/<to>/<ts>-<from>-<type>.json` — how agents and the user exchange messages.

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

- herdr agent names: lowercase letters, digits, `-`, `_` only, max 32 chars. Sanitize issue ids (`DEMO-42` → `demo-42`).
- `herdr agent read` / `pane read` print plain text, not JSON; `--lines N` can return empty — default to the full snapshot.
- `herdr workspace create` and `worktree create` return the root pane directly — no need to list panes after.
- `pane split` result nests the pane under `result.pane`; workspace create nests under `result.workspace`/`result.root_pane`; `tab create` under `result.tab`/`result.root_pane`.
- `herdr worktree list` `label` is the repo name, not the custom workspace label — match worktrees by `path`.
- Config paths must be absolute before comparing with herdr output (resolve relative to repo dir).
- Failed task creation can leave a stale git worktree + branch — clean both before retrying.
