# Pipeline plan half — ISSUE → WORKTREE → RESEARCH ∥ PLAN

```
ISSUE ── WORKTREE ── ASK ── RESEARCH (parallel, optional) ── PLAN ── hand over to references/build.md
```

The kickoff every issue starts with. Output: a worktree, its registered
pipeline, and a `done` plan task (or the user's explicit skip) plus any
research reports the plan was built from.

## Stages

1. **ISSUE** — `foreman issue get <ISSUE-ID>`; show the user a one-paragraph
   summary.
2. **WORKTREE** — reuse the issue's worktree if one exists (`foreman worktree
   list`); otherwise derive a 2-3 word slug from the issue title and run
   `foreman worktree add <ISSUE-ID> <word1> <word2> [<word3>]` (slug
   `plat-763-rate-limiter-tests`; project resolved by `issue_prefix`).
   Then register the pipeline: `foreman pipeline add <slug>` (starts at the
   plan half; idempotent — safe on resume).
3. **ASK** — one `ask_user_question`, recommended option first:
   1. Planning — "Run a planning task first?" → "Yes — plan first
      (Recommended)" / "No — skip planning"
   2. Research — "Which research should run in parallel?" (`multiSelect`)
      → "Explore codebase (Recommended)" / "Similar implementations" /
      "Read tests around the area" / "None"
   3. Build — "Start the build half when planning is done?" → "Yes —
      auto-start (Recommended)" / "No — ask me when planning finishes" /
      "No build"
4. **SPAWN** — create and `task execute` immediately, in parallel; tab labels
   via `--slug`:
   - plan: `foreman plan "Plan <issue title>" --worktree <slug> --slug plan-<slug>`
   - research, one task per chosen option:
     `foreman research "<what to investigate>" --worktree <slug> --slug research-<topic>`
   - `task execute` refuses build/test/fix while a plan is pending — that
     dependency guard is the CLI's, not yours.
   - Plan and research workers may spawn their own research subtasks; they
     appear in `task list` on their own.
5. **REPORTS** — handle per the foreman skill's "Reacting to worker reports"
   (summaries, report paths); in particular relay all research report paths
   to a waiting plan task once ALL research for the worktree is done,
   including worker-spawned subtasks. Record every report path for resume:
   `foreman pipeline report <slug> <path>`.
6. **HANDOVER** — planning done (or skipped by the user): summarize, then
   `foreman pipeline update <slug> --half build` and start `pipeline build
   <slug>` per the user's ASK choice (auto-start / ask / no build). A
   skipped plan hands over immediately.

## Rules

- Confirm with the user before creating the worktree or spawning agents
  unless they already gave the issue/slug or said go ahead (foreman ground
  rule).
- The plan half never builds: its last action is the handover.
