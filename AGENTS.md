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
- **harness** — the coding-agent runtime for workers (pi today; selectable via
  settings `harness`, implementations in `internal/harness/`). Worker agents
  run one task at a time (plan, research, build, test, fix, review, respond).
  They report to the central instance through `foreman mailbox send`.

## Architecture

There is exactly one **central foreman pi instance**. The user talks to it. It is the
orchestrator and the single inbox for everything:

- It creates workspaces, worktrees, tabs, and spawns worker agents.
- All worker messages (done / blocked / failed / needs-user) arrive in its mailbox.
- All watch events (PR comments, new review requests, PR status changes) arrive there too.
- The user responds and decides in that one place.

**Worker agents** are pi instances running inside herdr tabs. They run one task at a
time (plan, research, build, test, fix, review, respond). They report to the central instance
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
  setup                    # build foreman + watchman into .assembly/bin/
  project
    list
    add <path>               # register local repo; --name to override inferred name
                             # --issue-prefix, --worktree-init, --branch-prefix
    get <project>
    remove <project>         # unregister only; --purge also tears down its worktrees
  issue
    get <issue-id>           # fetch from Linear
  worktree
    list                     # --project to filter
    add <issue-id|slug> [words...]  # --project (default: by issue_prefix or cwd), --base
                                    # slug = args joined: plat-763-rate-limiter-tests
                                    # branch = settings branch_prefix + slug (default: same as slug)
    get <worktree>
    update <worktree>        # --status planning|building|pr-open|awaiting-review|addressing-comments|ready-for-merge|done|blocked|failed
    hold <worktree>          # --note <text> record a paused pipeline decision/step
    resume <worktree>        # show + clear a hold; the step to redo
    teardown <worktree>      # stop agents + clean up tabs, keep worktree data
    remove <worktree>        # full delete of worktree + tasks + its mailbox
                             #  messages and output reports; --force if dirty
  task
    list                     # --status --type --worktree (filters)
    add                      # --type plan|research|build|test|fix|review|respond
                             # --note --slug --worktree
                             # --general | --thread (note kind)
                             # a repeated --slug auto-rounds: test -> test-r2, -r3
    get <task-id>
    execute <task-id>        # spawn pi agent in a herdr tab and run the task
    rerun <task-id>          # teardown if running, reset to pending, execute
    update <task-id>         # --status pending|in-progress|self-review|done|blocked|failed --note
    teardown <task-id>
    remove <task-id>
  pr
    create <worktree>        # push branch, then open PR --title --base --no-template
                             #  (idempotent: reuses an existing PR for the branch;
                             #  title defaults to Linear issue title, base to repo
                             #  default; body follows the repo PR template; any
                             #  planning/building/blocked/failed status moves to
                             #  pr-open automatically)
    comment <pr|worktree>    # --body <text> [--reply <comment-id>] post a comment
                             #  or a threaded reply to an inline review comment
    review <pr-number>       # --verdict approve|comment|request-changes --body <text>
                             #  submit a review (repo from --repo or the pr-N worktree)
                             #  --comments-json '[{"path":..,"line":N,"body":..}]' for inline
                             #  comments (preferred over folding findings into --body)
                             #  --pending leaves it pending (visible only to you) instead
                             #  of submitting; --submit <review-id> --verdict ... publishes
                             #  a pending review later (comments already attached at creation)
    get <pr|worktree>        # --comments
    checkout <pr-number>    # materialize a PR as a pr-<N> review worktree
                             #  (--project when multiple registered); re-runs
                             #  hard-reset it to the current PR head;
                             #  worktree remove pr-<N> also deletes the branch
    pending <pr-number>      # list reviews left pending (--id shows that
                             #  review's inline comments too) before
                             #  publishing with --submit
  mailbox
    inbox                    # --unread to show only unread
    wait                     # block until an unread message arrives, print it,
                             #  and exit (one-shot delivery primitive)
    send <task-id> <message> # --status done|blocked|failed|in-progress|self-review
  status                     # one-screen overview: worktrees + status, running
                             # tasks, unread mail — the central instance's home view
  reset                      # stop the watchman, clear state.json +
                             # mailbox (settings, .env and bin/ kept)
  help
```

The `watchman` binary (`cmd/watchman`) is the daemon half:

```
watchman [start] [--detached] [--interval N] [--project X] [--pr]
                             [--foreman-pane ID]   # foreground by default;
                             --detached spawns a background instance (idempotent)
watchman stop
watchman status
```

Any foreman command run from the foreman tab (a herdr pane whose `.assembly/`
holds `settings.json`, no `FOREMAN_STATE_DIR` set) lazily starts the detached
watchman bound to that pane; it exits when the pane's agent is gone (tab
closed or pi exited). `FOREMAN_NO_WATCHMAN=1`
disables auto-start.

### Aliases (shortcuts to `task add`)

```
foreman plan <note>      # = task add --type plan --note <note>
foreman research <note>
foreman build <note>
foreman review <note>
foreman test <note>       # = task add --type test --note <note> (test gate: verdict pass|fail)
foreman fix <note>        # = task add --type fix --note <note> (implement findings)
foreman respond <note>
  --general               # general note, no target
  --thread                # note tied to a review thread
  --task
  --issue
  --worktree
```

## Mailbox protocol

One command serves both directions. Sender is detected by comparing the shell's
`HERDR_PANE_ID` with the task's stored pane ID. Worker messages record the
task **type** as `from` (`plan`, `build`, `test`, ...) and the sending tab ID
as `parent_id`; foreman messages use `from: foreman` with the foreman pane as
`parent_id`; watch events use `from: watch`:
- **Worker side** (runs inside the task's tab): `foreman mailbox send <id> <msg>
  --status ...` appends an unread message for the foreman and updates task status.
- **Foreman side** (anywhere else): the message is recorded and also delivered
  into the worker's tab via `herdr agent prompt`.
- Workers report with the foreman binary, not `go run`: `task execute` injects
  `FOREMAN_BIN` (usually `.assembly/bin/foreman`, built once with `foreman setup`)
  and embeds the full path
  in the worker prompt.
- `foreman mailbox inbox` prints messages and marks the shown ones read.
  `--follow` keeps watching the mailbox dir (fsnotify) and prints new
  messages as they arrive — this is the delivery mechanism into the central
  instance, wired differently per harness since neither delivery primitive
  below ever touches the pane's own input line (unlike the old
  `herdr agent prompt`-based push, which did — see watchman below):
  - **claude**: run `foreman mailbox inbox --unread --follow` wrapped in a
    persistent Monitor tool call. Each new message is one native
    notification, delivered as its own turn.
  - **pi** (via the `pi-background-tasks` extension — `pi install
    npm:pi-background-tasks@latest`): `bg_run` only wakes a follow-up turn
    on a job's *completion*, never on new output from a still-running job
    (verified — even their own persistent-watch example only tracks
    crash/exit). So `--follow` alone never wakes pi. Instead `bg_run` a
    one-shot `foreman mailbox wait` — it blocks until an unread message
    exists, prints it, and exits; the exit triggers the completion
    notification, which wakes pi with the message. Re-arm by `bg_run`-ing
    it again after every wake (pi must remember to do this each time —
    there is no way to make it self-perpetuating from a shell script,
    since `bg_run` is a model-invoked tool, not something a script can
    call into itself).
- The watchman daemon polls GitHub (`--interval`, default 60s) and writes
  results into the mailbox as `from: watch` messages; it does not deliver
  anything itself. It previously pushed messages into the foreman pane via
  `herdr agent prompt`, which types straight into the pane's live input
  line — with no way to check for unsent human input there first, a message
  could land mid-keystroke and corrupt what the user was typing. That path
  was removed; `ForemanPane` now exists only so watchman can check liveness
  (`herdr agent get`) and exit when the foreman agent is gone.
- New-comment polling excludes the authenticated `gh` user's own activity
  from every category (comments, reviews, inline comments) via
  `git.CurrentUserLogin`, not just IDs recorded in `self_comments` — leaving
  your own review on a `watched_prs` entry is otherwise indistinguishable
  from the response you're actually waiting for.
- Workers must have the `foreman` binary on PATH (install with
  `go install ./cmd/foreman`); the task prompt tells them the exact command to run.

## Worker prompt contract

`task execute` spawns pi with a prompt that states: task ID, type, worktree slug,
branch, note kind, the note itself, issue ref if any, and the exact mailbox command
(using the `FOREMAN_BIN` binary path) for reporting
`in-progress|self-review|done|blocked|failed`. The prompt also tells plan/build
workers they may spawn `research` subtasks themselves, and every worker to send
questions as `blocked` mailbox messages (`QUESTION:` + `OPTION:` lines) — the
central agent offers the question to the user (never auto-asks) and replies via
`mailbox send`. `task execute` refuses to start a **build/test/fix** task while
a plan task in the same worktree is pending/in-progress/self-review/blocked.
Plan/research/test workers write their report to `<state-dir>/output/<issue-id|worktree-slug>-<label>.md`
(`.assembly/output/` in the assembly repo) and must mention that path in their
done message — `mailbox send` rejects a done from these types without an
`output/` path. Worker tabs close automatically on `done` (all types;
blocked keeps the tab open for the answer), and on `failed` for the
report-writing plan/research/test types. Test workers
report `VERDICT: pass|fail`, review workers `FINDINGS: none` or numbered
findings, and fix workers re-run tests locally before reporting done.
Build/fix/respond workers commit their changes before reporting done. A plan task whose
worktree still has running research gets a "wait for the report paths" line in
its prompt — end the turn, do not poll; the paths arrive as a pushed message.
The central agent sends them once all research is done.
The foreman skill (`.agents/skills/foreman/`, including its `start` command) is
the single skill — keep it in sync with behavior changes. The pipeline skill
(`.agents/skills/pipeline/`, references `build.md` + `pr.md` + `review.md`)
drives the gated make-no-mistake flow: `pipeline build` (plan → build → test →
review with a fix loop), `pipeline pr` (docs → lint → PR → CI → merge),
`pipeline review` (someone else's PR, as reviewer), or plain `pipeline` for
the full run. Keep them all in sync.

## Core flow

1. `project add` registers a local repo (GitHub URL, local path).
2. `issue get <linear-id>` pulls issue details from Linear.
3. `worktree add <issue-id|slug>` creates a git worktree for the issue inside the
   project's herdr workspace.
4. `task add` (or aliases) creates tasks in that worktree.
5. `task execute <task-id>` spawns a pi agent in a new herdr tab in the worktree dir.
6. Workers message the central instance with `mailbox send` and `--status`.
7. The watchman daemon polls GitHub: PR comments, PRs assigned to me as reviewer,
   status changes. Events land in the mailbox and the daemon pushes them into
   the central instance's tab.
8. `worktree teardown` / `remove` ends the cycle.

## Status model (two levels)

- **Task status** — set by worker agents via `mailbox send --status`:
  `pending` → `in-progress` → `self-review` → `done` | `blocked` | `failed`.
  Workers know only their own task and never set PR states.
- **Worktree status** — set only by the central foreman or by the watchman
  daemon, derived from
  tasks + GitHub events:
  `planning` → `building` → `pr-open` → `awaiting-review` → `addressing-comments` →
  `ready-for-merge` → `done` (plus `blocked`/`failed` when work stops).
- The watchman daemon auto-derives worktree status: PR opened → `pr-open`; reviewer assigned →
  `awaiting-review`; new comment → `addressing-comments`; approved + CI green →
  `ready-for-merge`; merged → `done` (and it closes the worktree's open task
  tabs and marks unfinished tasks done).
- `task --status` and `worktree --status` accept only their own level's values.

## Task model

- `type`: plan | research | build | test | fix | review | respond
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
  "harness": "pi",
  "projects": {
    "igloo": {
      "path": "/Users/me/code/igloo",
      "repo": "me/igloo",
      "issue_prefix": "^(ENG|TAW)-",
      "worktree": {
        "init": "python3 -m venv .venv && .venv/bin/pip install -r requirements.txt",
        "branch_prefix": "foreman/"
      }
    }
  }
}
```

- `harness` is an optional override for the worker coding-agent harness;
  harnesses live in `internal/harness/` (`pi.go`, `claude.go`). Left unset
  (the normal case), `task execute` detects it automatically from the herdr
  agent kind running in the central foreman's own pane (`herdr agent get
  $HERDR_PANE_ID`, `mux.CurrentAgentKind`) — workers match whatever agent is
  orchestrating them. Falls back to `pi` only if detection fails (not running
  under herdr, herdr unreachable). Each harness runs with its own defaults —
  no model/thinking overrides.
- The `claude` harness needs one-time-per-project setup before it can run
  unattended: trust the project's main checkout once interactively (git
  worktrees inherit that trust automatically — no per-worktree action
  needed), and give it a permission allow-list via that project's own
  `.claude/settings.json` (Edit/Write/Read plus the exact `Bash(...)`
  patterns the worker needs, e.g. `git status/diff/add/commit/log`,
  `python`/`pytest`/`pip install` — never push, never a blanket bypass) plus
  a `CLAUDE.md` with unattended setup/test instructions so it never stops to
  ask a question nobody is there to answer.
- A project's `worktree` block (both fields optional; set via `project add
  --worktree-init`/`--branch-prefix`, or hand-edit settings.json):
  - `init`: a shell command `worktree add` runs in the new worktree's
    checkout right after herdr creates it — e.g. a venv or
    `docker compose build` so workers never hit missing deps. Runs via
    `sh -c`, output streamed live; a failure only warns (the worktree is
    already created and usable) rather than failing the whole command. Not
    expanded through `${VAR}` — it's a real shell command, so the shell
    resolves its own env vars.
  - `branch_prefix`: prepends to the slug to form the git branch name (e.g.
    `foreman/` + `dem-1-something-slug` → `foreman/dem-1-something-slug`);
    unset means the branch equals the slug, as before. The worktree's own
    `slug` (used for its directory, tab labels, mailbox keys, agent names)
    never carries the prefix — only `Worktree.Branch` does, which already
    existed as a separate field for exactly this.
- The project workspace is adopted, not duplicated: `worktree add` first looks
  for an existing herdr workspace whose non-linked repo root matches the
  project path (symlinks resolved); only when none exists does it create one.
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

- `.assembly/state.json` holds worktrees, tasks, per-project herdr workspace
  IDs, and `watched_prs` (PRs you reviewed but don't own — see below).
  Writes are atomic (tmp + rename). It never exists until the first write.
- `watched_prs` (keyed `<project>#<pr>`): `pr review` registers one
  automatically whenever you leave a review (pending or submitted) on a PR
  no worktree already owns — a `worktree remove pr-<N>` cleanup would
  otherwise leave nothing for watchman to poll for the author's response.
  Tracks `seen_comments`/`self_comments` exactly like `Worktree` does for
  owned PRs (same polling code, `internal/watchman/poll.go`'s
  `newCommentActivity`, shared between both); removed automatically once
  the PR is merged or closed. `from: watch` events from a watched PR carry
  no worktree/issue — just `pr-<N> (<project>)` — since there's no worktree
  to hang them off of.
- `.assembly/mailbox/<id>.json` holds one message per file. Messages carry
  context (project, worktree, issue id, tab label) alongside `task_id` so they
  relate directly to reports and sibling tasks. Workers append new
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
    project, issue, worktree, task, pr, mailbox, status, setup).
  - `cmd/watchman/` — daemon entrypoint: start/stop/status, foreground by
    default, `--detached` for the background lifecycle.
  - `internal/config/` — `.assembly/settings.json` load/save, `${ENV}` expansion, and key accessors (`LinearAPIKey`).
  - `internal/git/` — local git helpers plus GitHub PR calls via `gh`.
  - `internal/mux/` (`herdr.go`; tmux/cmux later), `internal/issue/`
    (`linear.go`; jira later), `internal/harness/` (`pi.go`, `claude.go`) —
    thin wrappers.
  - `internal/store/` — `.assembly/` state load/save.
  - `internal/watchman/` — the daemon core: mailbox watching + push delivery,
    GitHub PR polling, detached spawn, pane liveness (`fsnotify`).
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
- Only targeted regression tests where a real bug bit (e.g. `internal/store`
  MarkRead rewriting hand-written mailbox files); no broad suite yet.
- Stdlib first. Add a third-party dependency only when a concrete, demonstrated
  problem justifies it — never preemptively. Current justified deps:
  - `spf13/cobra` in `cmd/foreman`: stdlib `flag` silently drops flags placed after a
    positional argument; cobra/pflag parse them correctly and add persistent flags
    (e.g. `--dry-run`) that inherit across every subcommand for free.
  - `fsnotify/fsnotify` in `internal/watchman`: stdlib has no cross-platform
    file-change notification primitive.
  - `joho/godotenv` in `internal/config`: stdlib has no dotenv parser, and
    edge cases (export prefixes, quoting, multiline) are worth getting right.
