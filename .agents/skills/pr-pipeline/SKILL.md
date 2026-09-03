---
name: pr-pipeline
description: Ship a reviewed change end-to-end. Wraps the build-pipeline and continues after its clean review: document, lint, PR (push + create, repo PR template), CI check with fix loop, watch for human review comments with respond loop, done at merge. Use when the user asks to ship, open a PR with gates, or run the full PR flow on a worktree.
---

# PR pipeline — from clean review to merged

You are the central foreman instance driving the second half of the pipeline
for one worktree. The build-pipeline skill owns everything up to the clean
review; this skill takes over from there. Same style: automatic between
gates, one-line summaries to the user, escalate at round 3 of any loop.

## Stage machine

```
(build-pipeline: ... → REVIEW clean)
  → DOC → LINT → PR → CI CHECK
                ├─ CI red   → FIX → PR → CI CHECK          (cap 3 rounds)
                └─ CI green → WATCH
WATCH ├─ comments/CHANGES_REQUESTED → show user, ask → RESPOND → PR → CI CHECK → WATCH  (cap 3)
      ├─ APPROVED + CI green → tell user "ready to merge" → user merges
      └─ MERGED → done → offer worktree remove
```

## Stage definitions

- **DOC** — `build "Update docs/README for this change: <one-line summary>; stay in scope" --worktree <slug> --slug doc-<slug>`.
  Runs once; on review of the docs finding problems, re-run as another round.
- **LINT** — `test "Run the project's linters/formatters (not the test suite)" --worktree <slug> --slug lint-r<N>`.
  Gate worker: done message starts `VERDICT: pass|fail`. fail → FIX with the
  lint findings, then LINT again.
- **PR** — `pr create <worktree>`. One entry point: it pushes the branch,
  creates the PR or reuses the existing one, and follows the repo's PR
  template when present. After create: `worktree update <slug> --status pr-open`
  (pr create sets it on first open).
- **CI CHECK** — `pr get <worktree>` and read the `ci:` line. red → FIX with
  the failing check names, then PR (push), then CI CHECK again. Newly pushed
  commits reset the rollup to pending — wait for watchman's next PR event or
  re-check before calling it red.
- **WATCH** — passive. Watchman pushes PR events into this tab; never poll.
  - comment/CHANGES_REQUESTED event → fetch content with
    `pr get <worktree> --comments`, **show the user and ask** before
    dispatching. Approved → `respond "Address review comments: <threads/summary>" --thread --worktree <slug> --slug respond-r<N>`,
    then on its done → PR (push) → CI CHECK → back to WATCH.
  - APPROVED (and `pr get` confirms CI green) → tell the user it is ready to
    merge. The user merges; never merge yourself.
  - MERGED → pipeline done. Summarize and offer `worktree remove <slug>`.

## Rules

1. Rounds are counted per loop: CI-fix rounds and comment-response rounds are
   separate caps of 3. Slug suffixes track them (`fix-r2`, `respond-r2`).
2. Escalate at round 3 exactly like the build-pipeline: summarize what keeps
   failing, ask keep looping / take over / stop.
3. Blocked workers pause the pipeline; answered questions resume the same
   round. Failed workers: tell the user, offer re-run or abort.
4. `pr create` is the only push path — workers never push; after any FIX or
   RESPOND the pipeline pushes via `pr create <worktree>` (reuses the PR).
5. Never dispatch a respond task without showing the user the comments first.
6. Never merge, never force-push, never skip CI CHECK after a push.
