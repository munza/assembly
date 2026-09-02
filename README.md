# assembly

Agent crew for solo development. One command — `foreman` — spawns and
supervises a crew of pi coding agents in herdr, driven end to end:
pick Linear tasks, plan, code in isolated worktrees, open PRs, respond
to reviews, review others' PRs.

## Status

- [x] M1 — herdr control: spawn pi agents, prompt, wait, read, close
- [x] M2 — task state, herdr worktrees per issue, labeled panes per task, mailbox
- [x] M3 — watcher: poll Linear + GitHub, deliver as mail, nudge foreman
- [ ] M4 — foreman/worker skills, full task lifecycle
- [ ] M5 — restart recovery, blocked detection, PR review flow

## Commands

```
foreman task new TITLE... --type work [--issue ID] [--message M]
foreman plan|research|work|review TITLE... [--issue ID]   # type aliases
foreman task list|show REF|close REF
foreman prompt REF TEXT [--wait]   # REF = task ref or pane id
foreman read REF [--lines N]
foreman wait REF [--until done,idle,blocked]
foreman mail send BODY --from A --to B --type question|result|handoff|status
foreman mail list [BOX]
foreman watch [--once]              # poll loop: linear + github + mailbox
foreman init [--repo DIR]
```

## Model

```
project   1 herdr workspace (project name)
└─ issue  1 herdr worktree, branch <prefix><id>-<slug>
   └─ task 1 tab labeled <type>-<slug>, pi inside
      task file: .assembly/tasks/<id>-<type>-<slug>.json
mailbox: .assembly/mailbox/<to>/<ts>-<from>-<type>.json
```

## Layout

```
cmd/foreman/          CLI entry (constructor commands, one registration point)
internal/config/      layered config: defaults → .assembly/config.json → FOREMAN_* env
internal/herdr/       herdr wrapper (workspace/pane/worktree/agent ops)
internal/task/        task files: one task = one pane
internal/mailbox/     on-disk message bus
internal/orchestrator/ task lifecycle (new/close)
```

## Requirements

- Go 1.26+
- herdr (running: just `herdr` once)
- pi on PATH
