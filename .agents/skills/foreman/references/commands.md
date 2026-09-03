# Foreman command reference

Run as `go run ./cmd/foreman <cmd>` from the assembly repo. Global flags:
`--json` (machine-readable output), `--dry-run` (reads real; writes print what
they would do, including herdr/gh calls).

## project (settings, not state)

```
project list
project add <path> [--name NAME] [--issue-prefix RE]
project get <project>
project remove <project> [--purge]
```

Ref: name, `owner/repo`, repo basename, or path.

## issue

```
issue get <issue-id>        # Linear, e.g. ENG-123
```

## worktree

```
worktree list [--project P]
worktree add <issue-id|slug> [words...] [--base REF]   # slug = args joined; project by issue_prefix
worktree get <worktree>
worktree update <worktree> --status planning|building|pr-open|awaiting-review|addressing-comments|ready-for-merge|done|blocked|failed
worktree teardown <worktree>     # close tabs, keep checkout
worktree remove <worktree> [--force]  # delete checkout + tasks
```

Ref: slug, Linear issue ID, or branch. Issue IDs become slugs (ENG-123 → eng-123).

## task

```
task list [--status S] [--type T] [--worktree W]
task add --type plan|research|build|review|respond --note TEXT [--slug S] [--worktree W] [--general|--thread]
task get <task-id>
task execute <task-id>      # spawn pi agent in a herdr tab
task update <task-id> [--status pending|in-progress|self-review|done|blocked|failed] [--note TEXT]
task teardown <task-id>
task remove <task-id>
```

## aliases

```
plan <note> | research <note> | build <note> | review <note> | respond <note>
  [--general|--thread] [--slug S] [--task T] [--issue ID] [--worktree W]

task execute build tasks refuse to start while a plan task in the same
worktree is pending/in-progress/self-review/blocked.
```

`--task`/`--issue` pick the worktree and get referenced in the note.

## pr

```
pr create <worktree> [--title T] [--base B]   # title defaults to Linear issue title
pr get <pr|worktree> [--comments]
```

## mailbox

```
mailbox inbox [--unread] [--follow]   # shown messages are marked read
mailbox send <task-id> <message> [--status pending|in-progress|self-review|done|blocked|failed]
```

Direction is automatic: running inside the task's tab → report to foreman;
anywhere else → prompt delivered into the worker's tab.

## watch / status

```
watch [--interval SECS] [--pr] [--project P]   # PR events into mailbox
status                                          # overview
```

## Status models

- Task (worker-owned): pending → in-progress → self-review → done | blocked | failed
- Worktree (foreman/watch-owned): planning → building → pr-open → awaiting-review
  → addressing-comments → ready-for-merge → done (+ blocked/failed)
