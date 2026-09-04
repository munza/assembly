---
name: pipeline
description: Gated make-no-mistake pipeline for one worktree. Modes "pipeline plan" (kickoff: issue, worktree, research, plan), "pipeline build" (build, test, review with a fix loop), "pipeline pr" (docs, lint, PR, CI, watch, merge), "pipeline respond" (address PR comments), "pipeline review" (someone else's PR, as reviewer), or plain "pipeline" for the full chained run. Use when the user says pipeline, pipeline plan/build/pr/respond/review, build with gates, make no mistake, or wants an issue shipped end-to-end with quality gates.
---

# Pipeline — gated halves chained by handover

You are the central foreman instance driving the pipeline for one worktree.
Stages gate strictly: research gates the plan, the plan gates the build,
tests gate review, a clean review gates the PR, CI gates merge. Gates advance
automatically; only loop overruns and worker questions reach the user.

## Modes — plan → build → pr, plus respond and review

- `pipeline plan <issue-id|worktree>` — the kickoff: fetch the issue, ensure
  the worktree, run research in parallel if wanted, produce the plan.
  Hands over to the build half. Read `references/plan.md` first.
- `pipeline build <worktree|issue>` — BUILD → TEST → REVIEW (fix loop),
  consuming the plan half's output. Hands over to the pr half. Read
  `references/build.md` first.
- `pipeline pr <worktree>` — DOC → LINT → PR → CI CHECK → WATCH → merged.
  Comment events during WATCH hand over to the respond half. Read
  `references/pr.md` first.
- `pipeline respond <worktree|pr>` — address comments on your PR: show them
  verbatim, confirm with the user, run the respond task, push, re-check CI,
  return to WATCH. Read `references/respond.md` first.
- `pipeline review <pr>` — review someone else's PR as the reviewer: checkout,
  review task, confirm findings with the user, post the review or not.
  Read `references/review.md` first. Triggered by `review requested:` watch
  events or the user directly.
- `pipeline <worktree|issue>` — the full run: plan → build → pr, chained
  without stopping between halves.

All halves share one worktree and never run two stages of one pipeline in
parallel. Run everything through the foreman CLI (`go run ./cmd/foreman ...`);
reports and events arrive here via mailbox delivery (see the foreman skill).

## Progress view

Every pipeline update starts with the progress line(s), then one short
sentence. Never hand-draw them — run `foreman pipeline status <slug>` and
show its output verbatim (● done, ◉ running with task id and round suffix,
✗ blocked/failed, ○ not started, per half; respond and review annotate
their line). Round numbers come from the auto-appended `-rN` slug suffixes
and only grow with loops (TEST r1 → FIX r1 → TEST r2).

## Shared rules

1. **Advance automatically** on each `done` report; after every stage send
   the user the progress line plus a one-line summary (stage, task id,
   outcome). No confirmations between gates.
2. **Loops cap at round 3.** A round is one FIX (or RESPOND) cycle. When a
   loop hits round 3 without clearing, stop auto-looping: send the user an
   update — what ran so far, what keeps failing (quote the last findings) —
   and ask via `ask_user_question`: keep looping (raises the cap by 2) /
   user takes over / stop. Never loop silently past the cap.
3. **Blocked workers pause the pipeline**: relay the QUESTION/OPTION block as
   usual (see the foreman skill) and resume the same round after answering.
   Blocked rounds do not count toward the cap.
4. **Failed workers**: tell the user, offer re-run (`task rerun <id>`)
   or abort. Never silently retry.
5. **Resume from state**: `foreman pipeline get <slug>` — the half cursor
   plus every recorded report; its task list shows stage detail (a done task
   is a done stage — never re-run it; loop rounds are the auto-appended
   `-rN` slug suffixes, `test`, `test-r2`, ..., so stage recipes always pass
   a bare stem). Never re-run a stage that reported done unless a later FIX
   invalidated it.
6. Reports live in `.assembly/output/<issue-id|worktree-slug>-<label>.md`.
   The mailbox enforces the done contract — a plan/research/test done must
   mention its `output/` path, a test done must open with
   `VERDICT: pass|fail`, a review done must close with a `FINDINGS:` block —
   and indexes every reported path into the pipeline record automatically.
   `foreman pipeline report` remains for manual additions only.
7. Never skip a gate, never merge or force-push yourself, never open or update
   a PR while any pipeline task in the worktree is unfinished.
8. **Pause and resume**: if the user declines, breaks, or ignores any
   question, do not drop it — record it with
   `worktree hold <slug> --note "<the pending decision: question, options,
   comment quotes>"` and say the pipeline is on hold. When the user asks to
   resume, run `worktree resume <slug>` (prints and clears the hold),
   re-derive where the pipeline stands (`foreman pipeline get <slug>`), and
   re-ask or re-dispatch exactly what the note says. `status` shows holds,
   `task list`
   marks tasks of held worktrees, and `foreman resume` (top level, no arg)
   resumes the only hold — or `foreman resume --task <id>` /
   `foreman resume --worktree <slug>`.
9. **Never block on watchman**: after dispatching, restarting the daemon, or
   any push-based step, end the turn — reports and events arrive here on
   their own. Use `watchman status` (instant) for health, never `sleep` or
   wait-and-recheck.
10. **Handovers move the cursor, never re-runs**: the pipeline record
    (`foreman pipeline`) is the half-level state. Register once at kickoff
    (`foreman pipeline add <slug>`, plan half; `--half review` for a
    `pr-<N>` checkout). At every handover, move it —
    `foreman pipeline update <slug> --half <build|pr|respond|done>` — then
    start the next half (`pipeline plan` → `pipeline build` → `pipeline
    pr`); the pr half's WATCH hands comment events to `pipeline respond`,
    which moves the cursor back to pr once CI is green again. A half never
    re-enters a previous half; resuming mid-chain is rule 5's job.

## Reference

- Plan half — issue → worktree → research → plan: [plan](references/plan.md)
- Build half — stage machine and task recipes: [build](references/build.md)
- Pr half — docs, lint, PR, CI, watch, merge: [pr](references/pr.md)
- Respond half — address PR comments: [respond](references/respond.md)
- Review-only — someone else's PR, as the reviewer: [review](references/review.md)
