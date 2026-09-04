# Pipeline build half — PLAN → BUILD → TEST → REVIEW

```
PLAN → BUILD → TEST → REVIEW → clean (hands off to references/pr.md)
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

- **PLAN** (prerequisite). Reuse a `done` plan task in the worktree if one
  exists. If there is none, create a lightweight plan anyway:
  `plan "small plan: <issue title or the user's note>" --worktree <slug> --slug plan-<slug>`
  and wait for its report path like any other plan.
- **BUILD** — `build "Implement <plan report path>; stay in scope" --worktree <slug> --slug build-<slug>`.
- **TEST** — `test "Run the full test suite against the build" --worktree <slug> --slug test`.
  Test workers never modify code; their done message starts with
  `VERDICT: pass` or `VERDICT: fail`.
- **REVIEW** — `review "Review the full diff against <plan report path> and the issue requirements" --worktree <slug> --slug review`.
  Their done message ends with `FINDINGS: none` or numbered `FINDINGS:`.
- **FIX** — `fix "Fix round <N>: <the findings or test failures, one per line>" --worktree <slug> --slug fix`.
  Fix workers get the findings verbatim in the note; `<N>` is the round
  being fixed (the suffix of the failing stage's slug).

## Gates

- TEST `VERDICT: fail` → dispatch FIX with the test report's failures, then
  TEST of the next round.
- REVIEW with findings → dispatch FIX with the findings, then TEST, then
  REVIEW of the next round.
- REVIEW `FINDINGS: none` → build half done. In `pipeline build` mode:
  summarize for the user and offer `pr create <slug>` (or the pr half). In
  full/`pipeline pr` mode: continue with [pr](pr.md) immediately.
