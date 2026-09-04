# Worker prompt contract and mailbox protocol

## What `task execute` sends

The spawned pi agent receives a prompt like:

```
You are worker task t3 (build) in git worktree "eng-123" (branch eng-123).
Note kind: thread.
Task: implement the login fix
Issue: ENG-123 (run `foreman issue get ENG-123` for details).
Work in the current directory only. Report progress with:
foreman mailbox send t3 "<summary>" --status in-progress|self-review|done|blocked|failed
```

Plus, depending on type:

- plan with running research: "Research tasks t2, t3 are still running. Do NOT
  plan yet and do NOT poll the mailbox. End your turn and wait: their report
  paths will be delivered into this tab as a new message; plan using them when
  it arrives."
- plan/research/test: "When finished, write your full report to
  `<state-dir>/output/<issue-id|worktree-slug>-<label>.md` ... then send ONE final mailbox
  message that mentions that exact file path with --status done. Your tab
  closes automatically. `mailbox send` rejects a done from these types whose
  message lacks an `output/` path."
- plan/build/fix: they may spawn `research` subtasks themselves.
- all types: blocked questions use `QUESTION:` / `OPTION:` lines in the body.

Keep this contract in sync with `buildPrompt` in `cmd/foreman/task.go`.

## Environment inside a worker tab

- `FOREMAN_STATE_DIR` → absolute path of the central `.assembly/` dir, so
  `foreman` commands from any repo find state and settings.
- `FOREMAN_BIN` → absolute path of a foreman binary the worker can execute
  (usually `.assembly/bin/foreman`; the task prompt embeds the full path).
- `HERDR_PANE_ID` → set by herdr; used to detect "this command runs inside the
  task's own tab" (sender = worker).

## Mailbox semantics

- One JSON file per message in `.assembly/mailbox/`. Append-only from workers.
- `mailbox inbox` prints and marks shown messages read. `--follow` streams new
  ones (fsnotify) and never exits — this is the delivery primitive into the
  central instance (see below), not just a debugging aid.
- `mailbox send` from the foreman side also delivers the text into the worker's
  tab via `herdr agent prompt` (used to hand research report paths to a waiting
  plan agent, and answers to blocked workers). Worker tabs run unattended, so
  the collision risk that ruled this out for the foreman's own pane (below)
  doesn't apply here.
- `--status` on send updates the task status in `state.json`.
- A worker `mailbox send --status done|failed` from a research or plan tab
  closes its own tab automatically (detached close, so the message lands
  first). Blocked workers keep their tab open for the answer.
- The watchman daemon polls GitHub (`--interval`, default 60s) and writes
  results into the mailbox as `from: watch` messages; it does not deliver
  anything itself.

## Delivery into the central instance

Watchman used to push unread messages into the foreman pane via
`herdr agent prompt <pane-id>` on every mailbox write. That command types
directly into the pane's live input line with no way to check for unsent
human input there first — landing mid-keystroke, it would corrupt whatever
the user was typing into a garbled mix of both. That push path was removed
entirely (`ForemanPane` now exists only so watchman can exit when the pane's
agent is gone). Delivery is armed by the central instance itself instead,
using a mechanism that never touches the pane's input line:

- **claude**: `foreman mailbox inbox --unread --follow` wrapped in a
  persistent Monitor tool call. Each new message becomes one native
  notification (verified: a message delivered mid-draft left the draft
  byte-for-byte intact).
- **pi** (`pi-background-tasks` extension): `bg_run` only wakes a follow-up
  turn when a job *completes* — a persistent `--follow` job never does,
  verified even against the extension's own persistent-watch example, which
  tracks crash/exit only. So instead `bg_run` a one-shot poller:
  `while true; do out=$(./.assembly/bin/foreman mailbox inbox --unread); if [ "$out" != "mailbox empty" ]; then echo "$out"; break; fi; sleep 2; done`
  — its exit is the completion that triggers the wake. Verified the same
  way: draft left untouched. Re-arm by `bg_run`-ing the same poller again
  after each wake; a plain script can't call `bg_run` on itself, so this
  can't be made self-perpetuating — the central instance must remember.

## Watch events

The watchman daemon appends messages with `from: watch`, target = worktree slug
or project name: new PR comments/reviews, review requests, and worktree status
changes derived from GitHub (awaiting-review, addressing-comments,
ready-for-merge, done). They reach the central instance the same way any
other mailbox message does — see "Delivery into the central instance" above.

New comments/reviews are also polled for PRs you reviewed but don't own
(`pipeline review`'s `pr review` registers these in `watched_prs` since
there's no worktree left to poll against after CLEANUP) — the target is
`pr-<N>` with no worktree/issue context, since none exists. See
`pipeline/references/review.md`'s "When the author responds" for how to react.
