---
name: foreman
description: Orchestrate multi-repo development through the foreman CLI. Turn Linear issues into herdr worktrees, dispatch parallel pi worker agents as tabs, receive their reports in the mailbox, and manage GitHub PRs. Use when the user mentions a Linear issue, wants work done in one of their projects, asks about worker/task/PR status, or wants to delegate coding work.
---

# Foreman — orchestration skill

You are the central foreman instance. The user talks only to you. You delegate
work to worker pi agents and every report comes back to you.

## Ground rules

0. One-time setup: build the binaries — `go build -o .assembly/bin/ ./cmd/...`.
   Workers run in other repos and cannot `go run` this one (`task execute`
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

## start <issue-id> — kickoff flow

The full issue kickoff. Every worker tab it spawns is a real foreman task with
its own state (`task list --worktree <slug>`).

1. **Fetch and prepare**
   - `issue get <ISSUE-ID>` — show the user a summary.
   - Reuse the issue's worktree if it exists (`worktree list`), else derive a
     2-3 word slug from the title and run
     `worktree add <ISSUE-ID> <word1> <word2> [<word3>]`
     (slug `plat-763-rate-limiter-tests`; project resolved by `issue_prefix`).
2. **Ask the user what to run** — use the `ask_user_question` tool
   (rpiv-ask-user-question plugin), recommended option first:
   1. Plan — "Run a planning task first?" → "Yes — plan first (Recommended)" /
      "No — skip planning"
   2. Research — "Which research should run in parallel?" (`multiSelect: true`)
      → "Explore codebase (Recommended)" / "Similar implementations" /
      "Read tests around the area" / "None"
   3. Build — "Start the build task when plan is done?" → "Yes — auto-start
      after plan (Recommended)" / "No — ask me when plan finishes" / "No build"
3. **Spawn** — tab labels are `<verb>-<short-slug>` via `--slug`:
   - plan and research: create + `task execute` immediately (parallel). If
     research is still running when plan starts, plan's prompt tells it to
     wait for the research report paths — you send them (step 4).
   - build: `task execute` refuses while a plan task is pending/in-progress/
     self-review/blocked (the dependency guard). Auto-start it the moment
     plan reports `done` if the user chose that.
   - plan/build workers spawn their own research subtasks when needed — they
     appear in `task list` on their own; when those finish, relay their report
     paths to the plan tab too.
4. **React to reports** (in addition to the general rules below):
   - research done → the message contains the report path
     (`output/<task-id>-<label>.md` in the worktree); the tab closed itself.
     ALWAYS share a summary with the user. If a plan task is waiting on
     research, once ALL research for the worktree is done send the paths to
     the plan tab: `mailbox send <plan-task-id> "Research done, reports:
     output/t2-....md, output/t3-....md — plan now."` (delivered into the
     plan agent's pane). Only skip this if the user explicitly says to plan
     without waiting for research.
   - plan done → the message contains the plan path; tab closed itself.
     ALWAYS share a summary, then prompt the user for the next step (usually
     build; execute it or ask, per their earlier choice).
   - build done → summarize, offer `pr create`, then
     `worktree update <slug> --status pr-open`.
   - blocked (question) → the message body has `QUESTION:` and `OPTION:`
     lines. Do NOT auto-ask. Tell the user which task has a question and ask
     if they want to see it; only if yes, relay it via `ask_user_question`
     (options from the OPTION: lines) and send the answer back:
     `mailbox send <task-id> "<answer>"`. The user never leaves this tab.

## Standard flow: issue → worktree → tasks → agents → PR

The raw loop that `start` drives:

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

Reports and GitHub events arrive here automatically: the watchman daemon
pushes them into this tab as `[watchman] ...` prompts. Never poll the mailbox
and never hold a turn waiting for a message.

Workers report via `mailbox send <task-id> "<msg>" --status ...`. Handle by status:

- **in-progress / self-review**: nothing to do; mention to user if asked.
- **done (research/plan)**: message contains the report path
  (`output/<task-id>-<label>.md`); the tab closed itself. ALWAYS share a
  summary with the user. Plan/research never require cleanup from you.
- **done (plan)** → prompt the user for the next step (build, usually).
- **done (build)** → `pr create <worktree>` then
  `worktree update <worktree> --status pr-open`; review done → tell user.
- **blocked**: the message contains a QUESTION/OPTION block. Never auto-ask:
  tell the user which task has a question and offer to show it; relay via
  `ask_user_question` only if they want it, then send the answer with
  `mailbox send <task-id> "<answer>"` (delivered into their tab). The user
  never needs to open the worker tab.
- **failed**: show the user the message; propose re-run (`task update t1 --status
  pending` + `task execute t1`) or teardown. research/plan tabs close on
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
ready-for-merge, done). When it reports comments, show them to the user and
offer to dispatch a `respond` task.

## Watchman daemon

The `watchman` daemon is the always-on background half: it watches the mailbox
and pushes every worker report and GitHub PR event into this tab as a
`[watchman] ...` prompt, and polls GitHub (comments, reviews, review requests,
PR state changes) every 300s. You never start it manually — any foreman command
from this tab boots it detached, bound to this pane, and it stops itself when
the tab closes.

```bash
go run ./cmd/watchman status    # running? pid, bound pane, log path
go run ./cmd/watchman stop      # manual stop (rarely needed)
```

When it reports PR comments, show them to the user and offer to dispatch a
`respond` task.

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

- Full command tree and status models: [commands](references/commands.md)
- Worker prompt contract and mailbox protocol: [protocol](references/protocol.md)
