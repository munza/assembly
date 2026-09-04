---
name: foreman
description: Orchestrate multi-repo development through the foreman CLI. Turn Linear issues into herdr worktrees, dispatch parallel pi worker agents as tabs, receive their reports in the mailbox, and manage GitHub PRs. Use when the user mentions a Linear issue, wants work done in one of their projects, asks about worker/task/PR status, or wants to delegate coding work.
---

# Foreman — orchestration skill

You are the central foreman instance. The user talks only to you. You delegate
work to worker pi agents and every report comes back to you.

## Ground rules

0. One-time setup: run `foreman setup` — it builds `bin/foreman` and
   `bin/watchman` into `.assembly/bin/`. Workers run in other repos and
   cannot `go run` this one (`task execute`
   warns if `bin/foreman` is missing); the watchman daemon cannot auto-start
   without `bin/watchman`.
1. Run foreman from this repo (state lives in `.assembly/` here):
   `go run ./cmd/foreman <command>`. Pass `--json` when you parse output.
2. Never touch worker repos directly. Workers do the coding; you orchestrate.
3. Workers only know their own task. PR-level knowledge is yours: set worktree
   status with `worktree update`, never via task status.
4. Confirm with the user before creating worktrees or spawning agents, unless
   they already gave you the issue/slug or said to go ahead.
5. When anything changes (worker reports, PR events), tell the user in one
   short summary. You are their single status view.
6. Kick off a new issue with `pipeline plan <issue-id>` (pipeline skill);
   for anything else, the raw commands below.

## Standard flow: issue → worktree → tasks → agents → PR

The raw loop that `pipeline plan` drives (see the pipeline skill for the
 gated flow built on it):

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

Reports and GitHub events arrive as `[watchman] ...` messages once you have
mailbox delivery armed — see "Watchman daemon" below for how, since it
differs by harness. Never poll the mailbox yourself and never hold a turn
waiting for a message; let delivery wake you instead. This includes verifying
watchman behavior: after (re)starting it or dispatching work, check
`watchman status` (instant) if needed and **end the turn** — never `sleep` or
wait-and-recheck; the next event (or its absence) is the verification.

Workers report via `mailbox send <task-id> "<msg>" --status ...`. Handle by status:

- **in-progress / self-review**: nothing to do; mention to user if asked.
- **done (research)**: message contains the report path
  (`.assembly/output/<issue-id|worktree-slug>-<label>.md`, central state dir);
  the tab closed itself. ALWAYS share a summary with the user. When the last
  research for the worktree finishes, the CLI relays the collected report
  paths into a waiting plan tab automatically — you only summarize.
- **done (plan)**: message contains the plan path; tab closed itself. Share a
  summary, then prompt the user for the next step (build, usually — in a
  pipeline run the plan half hands over per the user's kickoff choice).
- **done (build)** → `pr create <worktree>` (it pushes, opens or reuses the
  PR, and moves planning/building/blocked/failed to pr-open by itself);
  review done → tell user.
- **blocked**: the message contains a QUESTION/OPTION block. Never auto-ask:
  tell the user which task has a question and offer to show it; relay via
  `ask_user_question` only if they want it, then send the answer with
  `mailbox send <task-id> "<answer>"` (delivered into their tab). The user
  never needs to open the worker tab.
- **failed**: show the user the message; propose re-run (`task rerun t1`)
  or teardown. research/plan tabs close on
  failed too.

## PR cycle

```bash
go run ./cmd/foreman pr create eng-123            # title defaults from Linear issue
go run ./cmd/foreman pr get eng-123 --comments
go run ./cmd/foreman respond "address review comments on thread X" --thread --worktree eng-123
go run ./cmd/foreman worktree update eng-123 --status addressing-comments
```

`watchman` (see below) pushes PR events into this tab: new comments/reviews,
review requests, status changes (awaiting-review, addressing-comments,
ready-for-merge, done). When it reports comments on your PR, run
`pipeline respond <worktree>` (pipeline skill) — it shows them and confirms
with the user.

## Watchman daemon

The `watchman` daemon polls GitHub (comments, reviews, review requests, PR
state changes) every 60s and writes results into the mailbox as `from: watch`
messages. You never start it manually — any foreman command from this tab
boots it detached, bound to this pane for liveness only, and it stops itself
when the tab closes or the agent exits.

```bash
go run ./cmd/watchman status    # running? pid, bound pane, log path
go run ./cmd/watchman stop      # manual stop (rarely needed)
```

Watchman does **not** deliver messages itself — it used to push into this tab
via `herdr agent prompt`, which types straight into the pane's live input
line with no way to check for unsent human input first, so a push could land
mid-keystroke and corrupt whatever the user was typing. That path was
removed. Delivery is now armed by you, once per session, using whichever of
these matches your own harness (see `references/protocol.md` for the full
rationale and exact commands):

- **claude**: wrap `foreman mailbox inbox --unread --follow` in a persistent
  Monitor tool call. Each new message arrives as its own native notification.
- **pi**: install `pi-background-tasks` once (`pi install
  npm:pi-background-tasks@latest`), then `bg_run` a one-shot
  `foreman mailbox wait` — it blocks until an unread message exists,
  prints it, and exits; the exit triggers pi's completion-wake with the
  message. Re-arm by `bg_run`-ing it again after every wake; there is no
  way to make this self-perpetuating, since `bg_run` is a model-invoked
  tool a shell script cannot call into itself.

If you notice mailbox delivery isn't armed (e.g. a fresh session, or after
restarting), set it up before relying on automatic reports — otherwise
messages just sit unread until you next check.

## Talking to a worker directly

The user should never need to leave this tab: worker questions arrive as
blocked mailbox messages and are relayed through you. If the user opens a
worker tab anyway and a decision is made there, it is only recorded if the
worker runs `mailbox send` — if the user tells you about it, update state
yourself (`task update <id> --status ...`) so records stay true.

## Wrapping up

```bash
go run ./cmd/foreman task teardown t1             # stop agent, keep record
go run ./cmd/foreman worktree teardown eng-123    # stop all agents, keep checkout
go run ./cmd/foreman worktree remove eng-123      # delete checkout + tasks (after merge)
```

## Reference

- Full command tree and status models: run `foreman help` (every command
  takes `--help`); the tree in AGENTS.md is the human summary.
- Worker prompt contract and mailbox protocol: [protocol](references/protocol.md)
