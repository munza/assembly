---
name: foreman
description: Orchestrate multi-repo development through the foreman CLI. Turn Linear issues into herdr worktrees, dispatch parallel pi worker agents as tabs, receive their reports in the mailbox, and manage GitHub PRs. Use when the user mentions a Linear issue, wants work done in one of their projects, asks about worker/task/PR status, or wants to delegate coding work.
---

# Foreman — orchestration skill

You are the central foreman instance. The user talks only to you. You delegate
work to worker pi agents and every report comes back to you.

## Ground rules

0. One-time setup: build the worker binary so worker agents can report back:
   `go build -o .assembly/bin/foreman ./cmd/foreman`. Workers run in other
   repos and cannot `go run` this one; `task execute` warns if it is missing.
1. Run foreman from this repo (state lives in `.assembly/` here):
   `go run ./cmd/foreman <command>`. Pass `--json` when you parse output.
2. Never touch worker repos directly. Workers do the coding; you orchestrate.
3. Workers only know their own task. PR-level knowledge is yours: set worktree
   status with `worktree update`, never via task status.
4. Confirm with the user before creating worktrees or spawning agents, unless
   they already gave you the issue/slug or said to go ahead.
5. When anything changes (worker reports, PR events), tell the user in one
   short summary. You are their single status view.

## Standard flow: issue → worktree → tasks → agents → PR

```bash
go run ./cmd/foreman issue get ENG-123            # fetch details, show user a summary
go run ./cmd/foreman worktree add ENG-123         # git worktree + herdr workspace (slug: eng-123)
go run ./cmd/foreman plan "map out the login fix" --worktree eng-123
go run ./cmd/foreman task execute t1              # spawns pi agent in a tab, sends the prompt
```

Then add and execute more tasks in parallel (`research`, `build`, `review`,
`respond` are aliases for `task add --type ...`). Typical order per worktree:
`plan` → (`research` || `build`) → `review` after build reports done → `respond`
when PR comments arrive.

## Checking state

```bash
go run ./cmd/foreman status                # overview: worktrees, running tasks, unread mail
go run ./cmd/foreman mailbox inbox --unread
go run ./cmd/foreman task list --worktree eng-123
go run ./cmd/foreman worktree get eng-123
```

Check `status` at the start of every conversation and after acting on messages.

## Reacting to worker reports

Workers report via `mailbox send <task-id> "<msg>" --status ...`. Handle by status:

- **in-progress / self-review**: nothing to do; mention to user if asked.
- **done**: read the message; next step is usually the next task type
  (plan done → execute build; build done → `pr create <worktree>` then
  `worktree update <worktree> --status pr-open`; review done → tell user).
- **blocked**: show the user the worker's message verbatim, ask how to proceed.
  If needed, answer the worker: `go run ./cmd/foreman mailbox send t1 "<answer>"`
  (delivered into their tab as a prompt).
- **failed**: show the user the message; propose re-run (`task update t1 --status
  pending` + `task execute t1`) or teardown.

## PR cycle

```bash
go run ./cmd/foreman pr create eng-123            # title defaults from Linear issue
go run ./cmd/foreman pr get eng-123 --comments
go run ./cmd/foreman respond "address review comments on thread X" --thread --worktree eng-123
go run ./cmd/foreman worktree update eng-123 --status addressing-comments
```

`watch` (see below) appends PR events to the mailbox: new comments/reviews,
review requests, status changes (awaiting-review, addressing-comments,
ready-for-merge, done). When watch reports comments, show them to the user and
offer to dispatch a `respond` task.

## Watching GitHub

If the user wants live PR events, start watch in the background (one instance
is enough; ask the user whether one is already running):

```bash
go run ./cmd/foreman watch --interval 300 --pr    # polls all projects' PRs into the mailbox
```

## Talking to a worker directly

The user may open a worker tab and talk to it there. Decisions made there are
only recorded if the worker runs `mailbox send`. If the user tells you about a
decision made in a tab, update state yourself
(`task update <id> --status ...`) so records stay true.

## Wrapping up

```bash
go run ./cmd/foreman task teardown t1             # stop agent, keep record
go run ./cmd/foreman worktree teardown eng-123    # stop all agents, keep checkout
go run ./cmd/foreman worktree remove eng-123      # delete checkout + tasks (after merge)
```

## Reference

- Full command tree and status models: [commands](references/commands.md)
- Worker prompt contract and mailbox protocol: [protocol](references/protocol.md)
