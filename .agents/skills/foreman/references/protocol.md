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
  tab via `herdr agent prompt`.
- `--status` on send updates the task status in `state.json`.

## Watch events

`watch` appends messages with `from: watch`, target = worktree slug or project
name: new PR comments/reviews, review requests, and worktree status changes
derived from GitHub (awaiting-review, addressing-comments, ready-for-merge,
done).
