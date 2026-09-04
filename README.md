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
- **herdr** — the terminal multiplexer/runtime everything else sits on top
  of. It owns workspaces, worktrees, tabs, panes, and agent lifecycle;
  `foreman` never touches a terminal directly, it drives herdr entirely
  through its CLI (`internal/mux/`).
- **harness** — the actual coding-agent CLI herdr runs inside each pane.
  `pi` and `claude` are both supported (`internal/harness/`), auto-detected
  from whatever the central instance is itself running — a worker matches
  its orchestrator. Each worker is one herdr pane running one harness
  instance for one task.
- **watchman** — a small daemon (`cmd/watchman`) that polls GitHub (PR
  comments, reviews, review requests, status changes) and writes what it
  finds into the mailbox as `from: watch` messages. It auto-starts detached
  the first time `foreman` runs from the central instance's own pane, and
  exits when that pane's agent is gone. It does not deliver anything itself
  — see the foreman skill below for how delivery actually reaches you.

```
foreman project add            ->  register a local repo
foreman issue get <id>         ->  fetch a Linear issue
foreman worktree add <id>      ->   git worktree + herdr workspace for that issue
foreman task add / execute     ->  spawn a worker (pi/claude, per the harness) to do the work
foreman pr create              ->  push, open the PR
```

Everything a worker reports — done, blocked, failed — and every GitHub event
watchman picks up lands in one mailbox. The central instance reads it via a
harness-appropriate delivery mechanism (a persistent background monitor for
Claude Code, a re-armed poller for pi) that never touches its own input line.

## The foreman skill

`.agents/skills/foreman/` (symlinked into `.claude/skills/` for Claude Code)
is the playbook the central instance actually follows — the CLI above is
just the mechanism. It defines the `start <issue-id>` kickoff (fetch the
issue, create the worktree, ask what to run — plan first? which research in
parallel? auto-build after plan?), how to react to each kind of worker
report (done/blocked/failed) and PR event, and how the watchman daemon and
mailbox fit together. Read `.agents/skills/foreman/SKILL.md` for the full
flow, or just ask the central instance to run it.

## The pipeline skill

`.agents/skills/pipeline/` is the gated, make-no-mistake version of the same
work for one worktree — stages advance automatically on each `done`, with
capped fix-loops instead of silent retries:

```
build:  PLAN → BUILD → TEST → REVIEW  (findings loop back to a FIX round)
pr:     DOC → LINT → PR → CI CHECK → WATCH → MERGED
```

`pipeline build <worktree|issue>` runs the build half, `pipeline pr
<worktree>` runs the ship half, plain `pipeline <worktree|issue>` runs both
back to back, and `pipeline review <pr>` reviews someone else's PR the same
gated way. See `.agents/skills/pipeline/SKILL.md` and its `references/`.

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

## License

[MIT](LICENSE)
