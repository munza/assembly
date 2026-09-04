# Pipeline review half — reviewing someone else's PR

```
○ FETCH ── ◉ REVIEW (tN) ── ○ CONFIRM ── ○ POST ── ○ CLEANUP
```

Single pass — no fix loops (the author fixes; we only review). Use for PRs
where the user is the requested reviewer (mailbox delivery surfaces
`review requested: PR #N <title>`) or when the user says
`pipeline review <pr>`.

## Stages

1. **FETCH** — show the user the PR (number, title, author, url via
   `gh pr view <N> --repo <repo> --json title,author,url,body,additions,deletions`)
   and confirm starting the review. Then materialize a checkout:
   - Find the project: the repo in the watch event / the user's reference
     matched against registered projects (`project list`).
   - `pr checkout <N> --project <project>` — it fetches the PR head and
     creates the `pr-<N>` worktree (branch `pr-<N>`, never branch-prefixed).
2. **REVIEW** — `review "Review PR #N <title>: correctness, tests, docs,
   scope; check the diff against the PR description" --worktree pr-<N>
   --slug review-pr-<N>`, then `task execute`. The worker's done message ends
   with `FINDINGS: none` or numbered findings, each tagged with its location
   (`<path>:<line> — ...`, `<path> — ...`, or plain text if untied to a
   file) — always share them verbatim.
3. **CONFIRM** — one `ask_user_question` call, two questions. Show the exact
   posting shape, not just the raw findings: the general `--body` text as
   its own block, and every inline comment as its own `path:line — body`
   line — the user sees precisely what will land where before anything goes
   to GitHub.
   1. Post the review as-is — verdict auto-picked: `FINDINGS: none` →
      approve; numbered findings → `request-changes` (or `comment` if the
      user prefers). / Don't post. / Or the user types their own review
      text (posted instead).
   2. Publish it now, or leave it pending on GitHub? Options: `Publish
      immediately (Recommended)` / `Leave pending — visible only to you,
      so you can review or edit it on GitHub before publishing`.
4. **POST** — inline comments are preferred over folding everything into the
   body: for each finding with a `path:line`, build a
   `--comments-json '[{"path":"...","line":N,"body":"..."}]'` entry (one
   array, all findings together); a file-level finding without a line, or
   one untied to any file, goes in `--body` instead (a short summary line
   plus those). It's posted as Markdown — keep every code-like token
   (routes, paths, identifiers) backtick-wrapped when carrying findings
   text over; a bare `/items/{item_id}` renders as plain text, `` `/items/{item_id}` ``
   renders as code. Then:
   - Publish immediately → `pr review <N> --verdict <approve|comment|request-changes>
     --body "<summary/untied findings>" --comments-json '<array>'` (repo
     resolves via the `pr-<N>` worktree).
   - Leave pending → `pr review <N> --pending --body "<summary/untied findings>"
     --comments-json '<array>'`. Its output includes the review ID and the
     GitHub URL — share both with the user and tell them how to publish it
     later: either from GitHub's own review UI, or
     `pr review <N> --verdict <verdict> --submit <review-id>` (comments were
     already attached at creation; `--submit` only assigns the verdict).
5. **CLEANUP** — `worktree remove pr-<N>` (it also deletes the fetched
   `pr-<N>` branch). Runs either way — the pending review lives on GitHub,
   not in the local checkout. Summarize for the user; if the review was left
   pending, say so plainly (this pass isn't fully done from GitHub's
   perspective until they publish it).

## Publishing a pending review later

If the user asks to publish a review left pending earlier (possibly a
different session, no `pr-<N>` worktree around anymore) — re-fetch it rather
than trusting memory, since it may have sat a while or been hand-edited on
GitHub: `pr pending <N> --id <review-id>` (with `--repo` if no `pr-<N>`
worktree exists) shows the body and every inline comment. Run CONFIRM
(verdict question only — it's already pending, nothing left to choose
there) showing that fetched body and every inline comment verbatim, then
`pr review <N> --verdict <verdict> --submit <review-id>`.

## When the author responds

Leaving a review (pending or submitted, via `pr review`) automatically starts
watching that PR for replies — no separate step, and no worktree required
(`state.json`'s `watched_prs`, cleaned up automatically once the PR is
merged or closed). New activity arrives the same way any other watch event
does: a `[watchman] ... pr-<N> (<project>): <N> new comment(s)/review(s) ...`
message, full comment/reply text included, addressed to you.

React like the WATCH stage in `pr.md` does for your own PRs — show the user
the comments verbatim and ask what to do — but the follow-up is different
since this isn't your PR to fix: offer `pipeline review <pr>` again
(`pr checkout` updates the existing worktree) rather than a `respond` task.
If they just want to reply without a full re-review, `pr comment <pr>` needs
a worktree to resolve against; re-fetch one via FETCH's steps 1-2 first, then
`worktree remove pr-<N>` again when done.

## Rules

- Never post anything without the CONFIRM step, **and CONFIRM always shows
  both the general body and every inline comment separately** — this
  applies whether creating a review (pending or immediate) or publishing an
  already-pending one. The user sees exactly what lands where, verdict
  included, before anything goes to GitHub.
- Never approve silently; `FINDINGS: none` still goes through CONFIRM.
- GitHub refuses `request-changes` (and sometimes `approve`) on your own
  PR — if the reviewer is also the PR's author, fall back to `comment` and
  say why, rather than treating the rejection as a bug.
- If the review worker is blocked (question), relay as usual; the pipeline
  waits.
- Re-review after the author pushes fixes: run the pipeline again —
   `pr checkout <N>` hard-resets the existing `pr-<N>` worktree to the
   current PR head (review checkouts are disposable; it never conflicts
   with a stale branch).
