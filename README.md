# assembly

A CLI (`foreman`) that lets one central coding-agent instance orchestrate
software work across multiple repos by delegating tasks to other coding
agents running in [herdr](https://herdr.dev) panes.

You talk to one **central foreman** agent. It turns Linear issues into git
worktrees, spawns **worker** agents to plan/build/test/review/respond, and
relays everything — worker reports, PR comments, CI status — back through a
mailbox so you never have to leave that one conversation.

## How it fits together

- **Linear** — issues, fetched by ID.
- **GitHub** — PRs, reviews, CI status (via `gh`).
- **herdr** — owns workspaces, worktrees, tabs, panes, and agent lifecycle.
  `foreman` drives it entirely through its CLI.
- **harness** — the coding-agent CLI workers run under. `pi` and `claude` are
  both supported today (auto-detected from whatever the central instance
  itself is running); see `internal/harness/`.

```
project add           →  register a local repo
issue get <id>         →  fetch a Linear issue
worktree add <id>      →  git worktree + herdr workspace for that issue
task add / execute     →  spawn a worker agent to do the work
pr create              →  push, open the PR
```

Everything a worker reports — done, blocked, failed — and every GitHub event
watchman picks up lands in one mailbox. The central instance reads it via a
harness-appropriate delivery mechanism (a persistent background monitor for
Claude Code, a re-armed poller for pi) that never touches its own input line.

## Requirements

- Go 1.26+
- [`herdr`](https://herdr.dev) on `PATH`
- [`gh`](https://cli.github.com) authenticated, for PR/CI operations
- A Linear API key, for issue lookups

## Build

```sh
go build -o .assembly/bin/ ./cmd/...
```

Workers need the `foreman` binary on `PATH` or reachable via `FOREMAN_BIN`
(`task execute` wires this up automatically); the detached `watchman` daemon
needs its own binary built the same way.

## Configure

`.assembly/settings.json` (gitignored, created on first write):

```json
{
  "linear": { "api_key": "${LINEAR_API_KEY}", "workspace": "myteam" },
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

Secrets go in `.assembly/.env` (see `.env.example`), referenced as `${VAR}`.

## Quick start

```sh
foreman project add /path/to/repo --issue-prefix '^ENG-'
foreman issue get ENG-123
foreman worktree add ENG-123 rate limiter
foreman plan "map out the fix" --worktree eng-123
foreman task execute t1
foreman status
```

## Full documentation

[`AGENTS.md`](AGENTS.md) is the complete reference — architecture, command
tree, mailbox protocol, status model, settings, and development conventions.
The `foreman` and `pipeline` skills under [`.agents/skills/`](.agents/skills)
(symlinked into `.claude/skills` for Claude Code) drive the actual
orchestration and gated build/PR flow.

## License

[MIT](LICENSE)
