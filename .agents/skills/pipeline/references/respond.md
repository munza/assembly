# Pipeline respond half — addressing comments on your PR

```
WAKE (comment / CHANGES_REQUESTED on your PR)
  → SHOW → CONFIRM → RESPOND → PUSH → REPLY → CI CHECK → back to pr WATCH
```

Entered from `pipeline pr`'s WATCH stage (a comment or CHANGES_REQUESTED
event on your PR) or directly (`pipeline respond <worktree|pr>`). Move the
cursor on entry — `foreman pipeline update <slug> --half respond` — and back
to pr when the loop ends. One round = one RESPOND cycle, capped per the
shared rules.

## Stages

1. **SHOW** — the watch event body carries the full text of all new comments
   and reviews (top-level, review bodies, inline `path:line`). Show them to
   the user verbatim; `foreman pr get <worktree> --comments` re-prints them
   if needed.
2. **CONFIRM** — ONE `ask_user_question` with two questions, each quoting the
   original comment(s) in the question text (author, file:line, full body
   verbatim) so the user decides with full context:
   1. Dispatch a respond task for these comments?
   2. Reply on the PR thread? Options: `Post "Addressed in <sha>" after the
      fix is pushed (Recommended)` / `No reply` — the user may type their own
      message instead (posted as the reply).
3. **RESPOND** — if dispatched: `foreman respond "Address review comments:
   <threads/summary>" --thread --worktree <slug> --slug respond-pr-<PR#> --stage respond`,
   then `task execute`. The slug auto-rounds per loop (`respond-pr-3`,
   `respond-pr-3-r2`, ...).
4. **PUSH** — `foreman pr create <worktree>` after the respond task reports
   done (per the pr half's rules: the only push path; workers never push).
5. **REPLY** — post the thread reply chosen in CONFIRM: `foreman pr comment
   <worktree> --body "<text>" --reply <inline-comment-id>` (thread reply;
   omit `--reply` for a top-level comment; ids from
   `pr get <worktree> --comments --json`). If a respond task ran, post it
   after PUSH so the body can include the commit sha; otherwise post
   immediately — or not, per the user's choice. The body is posted as
   Markdown: wrap code-like tokens in backticks.
6. **CI CHECK** — the pr half's CI CHECK, re-entered: `foreman pr get
   <worktree>`, read the `ci:` line. Newly pushed commits reset the rollup
   to pending — wait for the next PR event or re-check before calling it
   red. Green → `foreman pipeline update <slug> --half pr` and back to the
   pr half's WATCH.

## Rules

1. Never dispatch a respond task or post anything without the CONFIRM step —
   the user sees every comment verbatim first, exactly what will land where.
2. Rounds cap at 3 per the shared rules; blocked workers pause the loop
   (relay the QUESTION/OPTION block as usual); failed workers → offer
   `task rerun <id>`.
3. If the user declines to act on the comments, record the decision with
   `foreman worktree hold <slug> --note "..."` per the shared pause-and-resume
   rule — do not silently drop the thread.
