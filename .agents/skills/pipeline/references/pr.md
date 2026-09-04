# Pipeline pr half — DOC → LINT → PR → CI → WATCH → merged

```
(build half: ... → REVIEW clean)
  → DOC → LINT → PR → CI CHECK
                ├─ CI red   → FIX → PR → CI CHECK          (cap 3 rounds)
                └─ CI green → WATCH
WATCH ├─ comments/CHANGES_REQUESTED → respond half (references/respond.md) → back to WATCH
      ├─ APPROVED + CI green → tell user "ready to merge" → user merges
      └─ MERGED → done → offer worktree remove
```

## Stage definitions

- **DOC** — `build "Update docs/README for this change: <one-line summary>; stay in scope" --worktree <slug> --slug doc-<slug> --stage doc`.
  Runs once; on review of the docs finding problems, re-run as another round.
- **LINT** — `test "Run the project's linters/formatters (not the test suite)" --worktree <slug> --slug lint --stage lint`.
  Gate worker: done message starts `VERDICT: pass|fail`. fail → FIX with the
  lint findings, then LINT again.
- **PR** — `pr create <worktree>`. One entry point: it pushes the branch,
  creates the PR or reuses the existing one, follows the repo's PR
  template when present, and moves any planning/building/blocked/failed
  status to `pr-open` by itself.
- **CI CHECK** — `pr get <worktree>` and read the `ci:` line (the command
  itself warns that pending is not red). red → FIX with the failing check
  names, then PR (push), then CI CHECK again.
- **WATCH** — passive. PR events arrive via mailbox delivery (see the
  foreman skill); never poll.
  - comment/CHANGES_REQUESTED event → hand over to the respond half:
    `foreman pipeline update <slug> --half respond`, then run
    `pipeline respond <worktree>` — it shows the comments verbatim, confirms
    with the user, dispatches, pushes, and re-checks CI, then returns here.
  - APPROVED (and `pr get` confirms CI green) → tell the user it is ready to
    merge. The user merges; never merge yourself.
  - MERGED → pipeline done: `foreman pipeline update <slug> --half done`.
    Watchman has already closed any open task tabs and marked unfinished
    tasks done. Run `worktree remove <slug>` automatically (it also purges
    the worktree's mailbox messages, output reports, and pipeline record),
    then send the user the final summary with the progress view fully
    filled in.

## Rules

1. Rounds are counted per loop: CI-fix rounds and comment-response rounds are
   separate caps of 3 (the respond half owns its loop — never dispatch a
   respond task from this half directly). Slug stems track them
   automatically (`fix` → `fix-r2`).
2. Escalate at round 3 exactly like the shared rules in the main SKILL.md.
3. `pr create` is the only push path — workers never push; after any FIX the
   pipeline pushes via `pr create <worktree>` (reuses the PR).
4. Never merge, never force-push, never skip CI CHECK after a push.
