---
name: pipeline
description: Gated make-no-mistake pipeline for one worktree. Modes "pipeline build" (plan, build, test, review with a fix loop) and "pipeline pr" (docs, lint, PR, CI, review comments, merge), or plain "pipeline" for the full run. Use when the user says pipeline, pipeline build, pipeline pr, build with gates, make no mistake, or wants an issue shipped end-to-end with quality gates.
---

# Pipeline — gated stages with fix loops

You are the central foreman instance driving the pipeline for one worktree.
Stages gate strictly: tests gate review, a clean review gates the PR, CI gates
merge. Gates advance automatically; only loop overruns and worker questions
reach the user.

## Modes

- `pipeline build <worktree|issue>` — the build half only:
  PLAN → BUILD → TEST → REVIEW (fix loop). Read `references/build.md` first.
- `pipeline pr <worktree>` — the ship half: resumes/verifies the build state,
  then DOC → LINT → PR → CI CHECK → WATCH → merged. Read `references/pr.md`
  first; `references/build.md` covers anything still missing in the worktree.
- `pipeline <worktree|issue>` — the full run: build half first, then
  immediately the pr half without stopping between them.

Both halves share one worktree and never run two stages of one pipeline in
parallel. Run everything through the foreman CLI (`go run ./cmd/foreman ...`);
watchman pushes each worker report back into this tab.

## Progress view

Every pipeline update starts with the progress line, then one short sentence.

    build:  ● PLAN ── ● BUILD ── ◉ TEST r1 (t5) ── ○ REVIEW
    pr:     ○ DOC ── ○ LINT ── ○ PR#1 ── ○ CI ── ○ WATCH ── ○ MERGED

- ● done
- ◉ running — the current stage; include the task id and round suffix (TEST r2)
- ○ not started
- ✗ failed or blocked — annotate (`✗ FIX r2 (blocked: awaiting answer)`)

Print the line(s) for the half(s) in play; plain `pipeline` shows both. Round
numbers reset never — they only grow with loops (TEST r1 → FIX r1 → TEST r2).

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
4. **Failed workers**: tell the user, offer re-run (`task update <id> --status
   pending` + `task execute <id>`) or abort. Never silently retry.
5. **Resume from state**: `task list --worktree <slug>` — a done stage counts
   as done; the round number is the highest `r<N>` slug suffix + 1. Never
   re-run a stage that reported done unless a later FIX invalidated it.
6. Reports live in `.assembly/output/<issue-id|worktree-slug>-<label>.md`; worker done
   messages must mention the path. Test-gate workers report
   `VERDICT: pass|fail`; review workers `FINDINGS: none` or numbered findings.
7. Never skip a gate, never merge or force-push yourself, never open or update
   a PR while any pipeline task in the worktree is unfinished.
8. **Tab housekeeping**: only plan/research/test tabs close themselves on
   done. When a build/review/fix/respond stage reports done, immediately
   `task teardown <id>` to close its tab (record is kept).
9. **Pause and resume**: if the user declines, breaks, or ignores any
   question, do not drop it — record it with
   `worktree hold <slug> --note "<the pending decision: question, options,
   comment quotes>"` and say the pipeline is on hold. When the user asks to
   resume, run `worktree resume <slug>` (prints and clears the hold),
   re-derive the stage from `task list --worktree <slug>`, and re-ask or
   re-dispatch exactly what the note says. `status` shows holds.
10. **Never block on watchman**: after dispatching, restarting the daemon, or
   any push-based step, end the turn — reports and events arrive here on
   their own. Use `watchman status` (instant) for health, never `sleep` or
   wait-and-recheck.

## Reference

- Build half — stage machine and task recipes: [build](references/build.md)
- Ship half — docs, lint, PR, CI, watch, merge: [pr](references/pr.md)
