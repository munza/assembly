# Pipeline build half — BUILD → TEST → REVIEW

```
BUILD → TEST → REVIEW → clean (hands off to references/pr.md)
         ↑       |
         └── FIX ←┘    findings → fix → test → review again
```

`<N>` is the round number, starting at 1 for the first pass through
TEST/REVIEW and incremented on every FIX. You never compute it by hand:
stage recipes pass a bare slug stem (`test`, `fix`, `review`) and `task add`
auto-rounds a repeat to `test-r2`, `test-r3`, ... — the slug suffix is the
round number, and `task list --worktree <slug>` shows where a resumed
pipeline stands.

## Stage definitions

- **PLAN** (prerequisite, owned by the plan half) — a `done` plan task in
  the worktree whose report path feeds the BUILD and REVIEW notes. If there
  is none: run `pipeline plan <issue-id>` first — only if the user says so,
  proceed unplanned (their call, never yours).
- **BUILD** — `build "Implement <plan report path>; stay in scope" --worktree <slug> --slug build-<slug> --stage build`
  (unplanned: cite the issue title instead of a plan path).
- **TEST** — `test "Run the full test suite against the build" --worktree <slug> --slug test --stage test`.
  Test workers never modify code; the mailbox rejects a test done that does
  not open with `VERDICT: pass` or `VERDICT: fail`.
- **REVIEW** — `review "Review the full diff against <plan report path> and the issue requirements" --worktree <slug> --slug review --stage review`.
  The mailbox rejects a review done without a `FINDINGS: none` line or a
  numbered `FINDINGS:` block.
- **FIX** — `fix "Fix round <N>: <the findings or test failures, one per line>" --worktree <slug> --slug fix --stage fix`.
  Fix workers get the findings verbatim in the note; `<N>` is the round
  being fixed (the suffix of the failing stage's slug).

## Gates

- TEST `VERDICT: fail` → dispatch FIX with the test report's failures, then
  TEST of the next round.
- REVIEW with findings → dispatch FIX with the findings, then TEST, then
  REVIEW of the next round.
- REVIEW `FINDINGS: none` → build half done (report paths were indexed
  automatically as they arrived). In `pipeline build` mode:
  summarize for the user and offer the pr half (`foreman pipeline update
  <slug> --half pr`, or `pr create <slug>` alone if they just want the PR).
  In full/`pipeline pr` mode: move the cursor (`--half pr`) and continue
  with [pr](pr.md) immediately.
