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
   - `git -C <project-path> fetch origin pull/<N>/head:pr-<N>`
   - `worktree add pr-<N> --project <project>` (branch `pr-<N>` already
     exists from the fetch; the worktree checks it out).
2. **REVIEW** — `review "Review PR #N <title>: correctness, tests, docs,
   scope; check the diff against the PR description" --worktree pr-<N>
   --slug review-pr-<N>`, then `task execute`. The worker's done message ends
   with `FINDINGS: none` or numbered findings — always share them verbatim.
3. **CONFIRM** — `ask_user_question` quoting the findings verbatim:
   - Post the review as-is — verdict auto-picked: `FINDINGS: none` →
     approve; numbered findings → `request-changes` (or `comment` if the
     user prefers).
   - Don't post.
   - Or the user types their own review text (posted instead).
4. **POST** — `pr review <N> --verdict <approve|comment|request-changes>
   --body "<findings text>"` (repo resolves via the `pr-<N>` worktree).
5. **CLEANUP** — `worktree remove pr-<N>`, then
   `git -C <project-path> branch -D pr-<N>`. Summarize for the user.

## Rules

- Never post anything without the CONFIRM step — the user sees the exact
  text and the verdict before it goes to GitHub.
- Never approve silently; `FINDINGS: none` still goes through CONFIRM.
- If the review worker is blocked (question), relay as usual; the pipeline
  waits.
- Re-review after the author pushes fixes: run the pipeline again — fetch
   updates the `pr-<N>` branch (`git -C <project-path> fetch origin
   pull/<N>/head:pr-<N>` fails if the branch exists; use
   `git fetch origin pull/<N>/head && git branch -f pr-<N> FETCH_HEAD`, or
   remove and re-add the worktree).
