# Assembly — Project Instructions

This file is the single source of truth for how this project works and how it is built.
Read it fully before making changes. Keep it updated when behavior or design changes.

## Communication rules for the assistant

- Talk to the user in concise, simple, direct sentences. No filler.
- Any new instruction about development, behavior, or design goes into this file.

## What this project is

Assembly provides a CLI tool — `foreman`. It helps one central pi agent (the "foreman")
manage software work across many repos and delegate tasks to other pi agents.

External systems:

- **GitHub** — code, PRs, reviews.
- **Linear** — issues (fetched by issue ID).
- **herdr** (`herdr.dev`) — the agent runtime. It owns workspaces, worktrees, tabs, panes,
  and agent lifecycle. Foreman drives herdr via its CLI (JSON output on stdout).
  Always capture IDs from JSON responses, never predict them.
- **pi** — the agent harness. Worker agents are pi instances (each with the foreman
  skill in `.agents/skills/`).

## Architecture

There is exactly one **central foreman pi instance**. The user talks to it. It is the
orchestrator and the single inbox for everything:

- It creates workspaces, worktrees, tabs, and spawns worker agents.
- All worker messages (done / blocked / failed / needs-user) arrive in its mailbox.
- All watch events (PR comments, new review requests, PR status changes) arrive there too.
- The user responds and decides in that one place.

**Worker agents** are pi instances running inside herdr tabs. They run one task at a
time (plan, research, build, review, respond). They report to the central instance
through `foreman mailbox send`. The user may also open any tab and chat with that
agent directly; decisions made there must still be reported back with
`foreman mailbox send` + `--status`, so the central instance stays the source of truth.

### Mapping to herdr

- 1 herdr **workspace** = 1 project (registered repo), created with `herdr workspace create`.
- 1 git **worktree** = 1 issue (Linear ID) or custom slug. Use herdr's native
  `herdr worktree create --workspace <project-id> --branch <slug>` — it creates the git
  checkout, opens it as its own workspace, and groups it under the project workspace.
- 1 herdr **tab** = 1 running task agent (`herdr tab create --workspace <worktree-id>`),
  then `herdr agent start <name> --kind pi --pane <pane-id>`.

Key herdr commands foreman uses: `workspace create/list/get/close`,
`worktree create/open/remove`, `tab create/close/list`, `agent start/prompt/wait/read/`
`send-keys`, `pane split/run/wait-output`. See https://herdr.dev/docs/cli-reference/.

## Command tree

Global flags on every command: `--json` (machine-readable output for agents),
`--dry-run` (read commands run normally and show real output; write commands do not
execute — they print the change they would make, including any `git`/`herdr` calls
they would run).

Identifier rules: project = name or path; worktree = slug or issue ID; task = task ID;
pr = PR number or worktree slug. Every ID positional accepts the human-friendly form.

```
foreman
  project
    list
    add <path>               # register local repo; --name to override inferred name
    get <project>
    remove <project>         # unregister only; --purge also tears down its worktrees
  issue
    get <issue-id>           # fetch from Linear
  worktree
    list                     # --project to filter
    add <issue-id|slug> [words...]  # --project (default: by issue_prefix or cwd), --base
                                    # slug = args joined: plat-763-rate-limiter-tests
    get <worktree>
    update <worktree>        # --status planning|building|pr-open|awaiting-review|addressing-comments|ready-for-merge|done|blocked|failed
    teardown <worktree>      # stop agents + clean up tabs, keep worktree data
    remove <worktree>        # full delete of worktree + its tasks; --force if dirty
  task
    list                     # --status --type --worktree (filters)
    add                      # --type plan|research|build|review|respond
                             # --note --slug --worktree
                             # --general | --thread (note kind)
    get <task-id>
    execute <task-id>        # spawn pi agent in a herdr tab and run the task
    update <task-id>         # --status pending|in-progress|self-review|done|blocked|failed --note
    teardown <task-id>
    remove <task-id>
  pr
    create <worktree>        # --title --base (title defaults to Linear issue title,
                             #  base defaults to repo default branch)
    get <pr|worktree>        # --comments
  mailbox
    inbox                    # --unread to show only unread
    send <task-id> <message> # --status done|blocked|failed|in-progress|self-review
  watch                      # --interval --pr --project
  status                     # one-screen overview: worktrees + status, running
                             # tasks, unread mail — the central instance's home view
  help
```

### Aliases (shortcuts to `task add`)

```
foreman plan <note>      # = task add --type plan --note <note>
foreman research <note>
foreman build <note>
foreman review <note>
foreman respond <note>
  --general               # general note, no target
  --thread                # note tied to a review thread
  --task
  --issue
  --worktree
```

## Mailbox protocol

One command serves both directions. Sender is detected by comparing the shell's
`HERDR_PANE_ID` with the task's stored pane ID:
- **Worker side** (runs inside the task's tab): `foreman mailbox send <id> <msg>
  --status ...` appends an unread message for the foreman and updates task status.
- **Foreman side** (anywhere else): the message is recorded and also delivered
  into the worker's tab via `herdr agent prompt`.
- Workers report with the foreman binary, not `go run`: `task execute` injects
  `FOREMAN_BIN` (usually `.assembly/bin/foreman`, built once with
  `go build -o .assembly/bin/foreman ./cmd/foreman`) and embeds the full path
  in the worker prompt.
- `foreman mailbox inbox` prints messages and marks the shown ones read.
  `--follow` keeps watching the mailbox dir (fsnotify) for new messages.
- Workers must have the `foreman` binary on PATH (install with
  `go install ./cmd/foreman`); the task prompt tells them the exact command to run.

## Worker prompt contract

`task execute` spawns pi with a prompt that states: task ID, type, worktree slug,
branch, note kind, the note itself, issue ref if any, and the exact mailbox command
(using the `FOREMAN_BIN` binary path) for reporting
`in-progress|self-review|done|blocked|failed`. The prompt also tells plan/build
workers they may spawn `research` subtasks themselves, and every worker to send
questions as `blocked` mailbox messages — the central agent asks the user and
replies via `mailbox send`. `task execute` refuses to start a **build** while a
plan task in the same worktree is pending/in-progress/self-review/blocked.
The foreman skill (`.agents/skills/foreman/`, including its `start` command) is
the single skill — keep it in sync with behavior changes.

## Core flow

1. `project add` registers a local repo (GitHub URL, local path).
2. `issue get <linear-id>` pulls issue details from Linear.
3. `worktree add <issue-id|slug>` creates a git worktree for the issue inside the
   project's herdr workspace.
4. `task add` (or aliases) creates tasks in that worktree.
5. `task execute <task-id>` spawns a pi agent in a new herdr tab in the worktree dir.
6. Workers message the central instance with `mailbox send` and `--status`.
7. `watch` polls GitHub: PR comments, PRs assigned to me as reviewer, status changes.
   Events are delivered to the central instance's mailbox.
8. `worktree teardown` / `remove` ends the cycle.

## Status model (two levels)

- **Task status** — set by worker agents via `mailbox send --status`:
  `pending` → `in-progress` → `self-review` → `done` | `blocked` | `failed`.
  Workers know only their own task and never set PR states.
- **Worktree status** — set only by the central foreman or by `watch`, derived from
  tasks + GitHub events:
  `planning` → `building` → `pr-open` → `awaiting-review` → `addressing-comments` →
  `ready-for-merge` → `done` (plus `blocked`/`failed` when work stops).
- `watch` auto-derives worktree status: PR opened → `pr-open`; reviewer assigned →
  `awaiting-review`; new comment → `addressing-comments`; approved + CI green →
  `ready-for-merge`; merged → `done`.
- `task --status` and `worktree --status` accept only their own level's values.

## Task model

- `type`: plan | research | build | review | respond
- Every task belongs to a worktree; every worktree belongs to a project.
- Task ID + slug must be unique and stable (used by mailbox, tabs, and labels).

## Settings

`.assembly/settings.json` (same dir as state; honors `FOREMAN_STATE_DIR`) holds
configuration, not runtime state:

```json
{
  "linear": {
    "api_key": "${LINEAR_API_KEY}",
    "workspace": "myteam"
  },
  "pi": {
    "model": "glm-5.3",
    "thinking": "high"
  },
  "projects": {
    "igloo": {
      "path": "/Users/me/code/igloo",
      "repo": "me/igloo",
      "issue_prefix": "^(ENG|TAW)-"
    }
  }
}
```

- `pi.model` / `pi.thinking` are passed to every worker as
  `pi --model <m> --thinking <t>`; without them workers use the pi default.
- The default tab herdr creates with each worktree is closed automatically
  after the first task agent spawns (closing it earlier would close the
  workspace).
- `issue_prefix` is a regex matched against Linear issue IDs. One prefix or many:
  `^ENG-` or `^(ENG|TAW)-`. `worktree add ENG-123` uses it to pick the project
  without `--project`, and rejects an issue routed to a project whose prefix
  does not match. Ambiguous matches (two projects match) require `--project`.

- At startup every command loads `.assembly/.env` into the process environment
  (godotenv; existing variables win, missing file ignored). `${VAR}` in any value
  then expands at read time (`config.Expand`). Files keep the raw reference, so
  saving never clobbers `${...}` entries.
- Secrets belong in `.assembly/.env` (gitignored); `settings.json` stays
  secret-free and references them. Workers resolve it too — `FOREMAN_STATE_DIR`
  points them at the same `.assembly/` dir.
- Projects are settings (name → path + repo + optional `issue_prefix`). Paths are
  absolute — `project add` writes them that way.
- Runtime per-project data (herdr workspace ID) stays in `state.json`, not settings.

## State

- State is **global** (covers all projects), stored in `.assembly/` in the current
  working directory (the `assembly` repo). Later it moves to `~/.assembly/`.
Both files are created lazily on first write — an empty `.assembly/` with only
`bin/` is normal until you register a project or create a worktree.

- `.assembly/state.json` holds worktrees, tasks, per-project herdr workspace IDs.
  Writes are atomic (tmp + rename). It never exists until the first write.
- `.assembly/mailbox/<id>.json` holds one message per file. Workers append new
  files — no read-modify-write races between parallel agents.
- `FOREMAN_STATE_DIR` env var overrides the state directory. `task execute`
  injects it into worker tabs via `herdr tab create --env`, because workers run
  in other repos' worktree checkouts and cannot find `.assembly/` by cwd.
- Task IDs are derived (`t<max existing + 1>`); there is no counter in state.
- Known limitation: `state.json` updates (task status changes) from multiple
  processes can still race. Accept for now; revisit if it bites.

## Implementation

- Language: **Go**. Binary: `foreman` (package `cmd/foreman`).
- Layout:
  - `cmd/foreman/` — entrypoint + cobra command tree (one file per group:
    project, issue, worktree, task, pr, mailbox, watch).
  - `internal/config/` — `.assembly/settings.json` load/save, `${ENV}` expansion, and key accessors (`LinearAPIKey`).
  - `internal/git/` — local git helpers plus GitHub PR calls via `gh`.
  - `internal/herdr/`, `internal/linear/` — thin wrappers.
  - `internal/store/` — `.assembly/` state load/save.
  - `internal/watchman/` — event watching (`fsnotify`).
- Prefer thin wrappers: foreman shells out to `git`, `herdr`, `gh` (GitHub), and the
  Linear API. Do not reimplement them.

## Development conventions

These apply to every change to this repo, human or agent:

- Commit small and often. One commit = one independent, reviewable unit, with a
  brief, plain message (no multi-paragraph bodies).
- Before landing a genuinely large or multi-part change, ask if it is really several
  independent units — split it into multiple commits when the pieces stand alone.
  To verify a commit that is hard to reason about in isolation, check it out into a
  disposable `git worktree` and build it there (see the `setup-and-settings` skill)
  instead of assuming it is correct.
- No comments by default. Add one only when the _why_ is invisible in the code
  itself — a non-obvious constraint, a workaround, a gotcha a future reader would
  trip over.
- Pragmatism over abstraction. Duplicating a little code across two or three call
  sites is fine; extract a shared abstraction only when a third or fourth real use
  case demands it.
- Functional style: plain functions over structs-with-methods, composition over
  inheritance. No heavy FP machinery — no monads, no generic combinator libraries.
  Plain Go.
- Command wiring (cobra):
  - No `init()` in command files and no package-level `*cobra.Command` variables.
    Calling methods on another file's package var hides the wiring and depends on
    implicit file-name initialization order.
  - Each command group is a constructor (`newIssueCmd() *cobra.Command`) that
    declares its flag variables locally and captures them in its RunE closures.
  - `newRootCmd()` in `root.go` is the only place that registers commands; it
    assembles the whole tree explicitly.
  - The only shared mutable package vars are the global flags (`flagJSON`,
    `flagDryRun`) in `root.go`.
- Colocate helpers with their owner: project helpers live in `project.go`, task
  helpers in `task.go`. No `common.go` dumping grounds.
- get-style text output uses one `text/template` per command file (e.g.
  `issueText`), parsed once with `template.Must`. Keys are fixed, so alignment
  is hardcoded in the template. Config values and their
  accessors belong to `internal/config` (e.g. `config.LinearAPIKey()`), never
  re-implemented in cmd files.
- No tests yet. Personal tool in early scaffolding; revisit once the core loop is
  proven.
- Stdlib first. Add a third-party dependency only when a concrete, demonstrated
  problem justifies it — never preemptively. Current justified deps:
  - `spf13/cobra` in `cmd/foreman`: stdlib `flag` silently drops flags placed after a
    positional argument; cobra/pflag parse them correctly and add persistent flags
    (e.g. `--dry-run`) that inherit across every subcommand for free.
  - `fsnotify/fsnotify` in `internal/watchman`: stdlib has no cross-platform
    file-change notification primitive.
  - `joho/godotenv` in `internal/config`: stdlib has no dotenv parser, and
    edge cases (export prefixes, quoting, multiline) are worth getting right.
