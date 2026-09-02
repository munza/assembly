# assembly

Agent crew for solo development. One command — `foreman` — spawns and
supervises a crew of pi coding agents in herdr, driven end to end:
pick Linear tasks, plan, code in isolated worktrees, open PRs, respond
to reviews, review others' PRs.

## Status

- [x] M1 — herdr control: spawn pi agents, prompt, wait, read, close
- [ ] M2 — task state machine, git worktrees, on-disk mailbox
- [ ] M3 — watcher: poll Linear + GitHub, nudge foreman
- [ ] M4 — foreman/worker skills, full task lifecycle
- [ ] M5 — restart recovery, blocked detection, PR review flow

## Commands

```
foreman init [--repo DIR]           write .assembly/config.json
foreman spawn NAME [--task T]       spawn pi agent in new herdr workspace
foreman prompt NAME TEXT [--wait]   send prompt, optionally wait for done
foreman read NAME [--lines N]       read agent terminal output
foreman wait NAME [--until STATE]   wait for idle|working|blocked|done
foreman agents                      tracked agents + live state
foreman close NAME                  stop agent, free workspace
```

## Layout

```
cmd/foreman/          CLI entry
internal/config/      .assembly/config.json
internal/herdr/       herdr CLI wrapper (workspace/pane/agent ops)
internal/state/       .assembly/agents.json — restart-safe registry
internal/orchestrator/ agent lifecycle (spawn/close)
```

## Requirements

- Go 1.26+
- herdr (running: just `herdr` once)
- pi on PATH
