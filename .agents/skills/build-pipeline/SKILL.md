---
name: build-pipeline
description: Gated build pipeline (make-no-mistake) for one worktree: plan, build, test, review, then a fix loop that escalates to the user instead of looping forever. Use when the user asks to run the pipeline, build with gates, "make no mistake", or ship an issue end-to-end with quality gates.
---

# Build pipeline — gated stages with a fix loop

You are the central foreman instance driving the pipeline for one worktree.
Stages gate strictly: tests gate review, a clean review gates the PR. Every
gate transition is automatic; only loop overruns and worker questions reach
the user.

## Stage machine

```
PLAN → BUILD → TEST → REVIEW → done (offer pr create)
                 ↑       |
                 └── FIX ←┘    findings → fix → test → review again
```

One worktree, never two pipeline stages in parallel. Run everything through
the foreman CLI as usual (`go run ./cmd/foreman ...`); watchman pushes each
worker report back into this tab.

## Stage definitions

- **PLAN** (prerequisite). Reuse a `done` plan task in the worktree if one
  exists. If there is none, create a lightweight plan anyway:
  `plan "small plan: <issue title or the user's note>" --worktree <slug> --slug plan-<slug>`
  and wait for its report path like any other plan.
- **BUILD** — `build "Implement <plan report path>; stay in scope" --worktree <slug> --slug build-<slug>`.
- **TEST** — `test "Run the full test suite against the build" --worktree <slug> --slug test-r<N>`.
  Test workers never modify code; their done message starts with
  `VERDICT: pass` or `VERDICT: fail`.
- **REVIEW** — `review "Review the full diff against <plan report path> and the issue requirements" --worktree <slug> --slug review-r<N>`.
  Their done message ends with `FINDINGS: none` or numbered `FINDINGS:`.
- **FIX** — `fix "Fix round <N>: <the findings or test failures, one per line>" --worktree <slug> --slug fix-r<N>`.
  Fix workers get the findings verbatim in the note.

`<N>` is the round number, starting at 1 for the first pass through
TEST/REVIEW and incremented on every FIX.

## Rules

1. **Advance automatically** on each `done` report. After every stage send
   the user a one-line summary (stage, task id, outcome). No confirmation
   questions between gates.
2. **Gates**:
   - TEST `VERDICT: fail` → dispatch FIX with the test report's failures,
     then TEST of the next round.
   - REVIEW with findings → dispatch FIX with the findings, then TEST, then
     REVIEW of the next round.
   - REVIEW `FINDINGS: none` → pipeline done: summarize for the user and
     offer `pr create <slug>` (then `worktree update <slug> --status pr-open`).
3. **Escalate at round 3.** If FIX→TEST→REVIEW has cycled three times and
   still is not clean, stop auto-looping. Send the user an update — what the
   pipeline did so far, what keeps failing (quote the last findings) — and
   ask via `ask_user_question`: keep looping (raises the cap by 2) /
   user takes over / stop the pipeline. Never loop silently past the cap.
4. **Blocked workers** pause the pipeline: relay the QUESTION/OPTION block as
   usual (see the foreman skill) and resume the same round after answering.
   Blocked rounds do not count toward the cap.
5. **Failed workers**: tell the user, offer re-run (`task update <id>
   --status pending` + `task execute <id>`) or abort. Never silently retry.
6. **Never** skip a gate, and never open a PR while any pipeline task in the
   worktree is unfinished.

## Starting mid-flow

If tasks already exist in the worktree, resume from state — `task list
--worktree <slug>` — instead of restarting: a done plan counts as PLAN, a
done build counts as BUILD, the round number is the highest `r<N>` suffix + 1.
Never re-run a stage that already reported done unless its output was
invalidated by a later FIX.
