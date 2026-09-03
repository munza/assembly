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
  ones (fsnotify).
- `mailbox send` from the foreman side also delivers the text into the worker's
  tab via `herdr agent prompt` (used to hand research report paths to a waiting
  plan agent, and answers to blocked workers).
- `--status` on send updates the task status in `state.json`.
- A worker `mailbox send --status done|failed` from a research or plan tab
  closes its own tab automatically (detached close, so the message lands
  first). Blocked workers keep their tab open for the answer.
- The watchman daemon watches the mailbox (fsnotify + 30s sweep) and pushes
  unread worker/watch messages into the foreman tab via `herdr agent prompt
  <pane-id>`, marking them read — no polling anywhere. Messages the foreman
  itself sent are skipped: mailbox send already delivered those.

## Watch events

The watchman daemon appends messages with `from: watch`, target = worktree slug
or project name: new PR comments/reviews, review requests, and worktree status
changes derived from GitHub (awaiting-review, addressing-comments,
ready-for-merge, done). It pushes them to the foreman tab like worker reports.
