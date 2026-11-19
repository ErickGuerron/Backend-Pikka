# ODD: Organic Driven Development

ODD is the default workflow for changes in this repository. It replaces the
SDD phase chain (proposal, spec, design, tasks, apply, verify, archive) for
day-to-day work. It keeps one feature document per change, one bounded writer per
work unit, and evidence for every claim.

## When to use it

Every request. Use ODD unless the user explicitly asks for SDD.

## Feature document

Each feature lives in `odd/tasks/<feature>.md` with these sections, in order:

1. `Branch` and base (always `develop` unless the user says otherwise).
2. `Objective`: one paragraph, the sprint deliverable.
3. `Scope`: In and Out lists. Anything Out is not implemented in this change.
4. `Constraints`: conventions to mirror, TDD mode, tooling, commit rules.
5. `Design assumptions`: decisions the docs do not define, flagged for review.
6. `Tasks`: checklist. Each task has a `Route` and a `Check`.
7. `Acceptance criteria`: observable checks that close the feature.
8. `Delivery`: size forecast and PR strategy.
9. `Progress / evidence`: commits, RED/GREEN observations, review lineages.
10. `Next step`: the single next action.

## Flow

1. **Explore inline.** Read 1 to 3 files to decide. For 4 or more files, delegate
   one narrow mapping task and keep only its short summary.
2. **Write the feature document** before any code. Record assumptions and open
   product questions. Ask the user only about product or business decisions, one
   question at a time.
3. **Size forecast** before implementation. If the forecast exceeds 400 authored
   changed lines (excluding generated code), split into stacked PRs before writing
   code, not after.
4. **Execute tasks in order.** One task at a time, marked `[x]` only after its
   check passes.
5. **Verify each task** with its `Check` command. Record the command and result in
   `Progress / evidence`.
6. **Measure the branch** with `git diff --shortstat <base>...HEAD` per branch,
   never by hand. Compare against the budget.
7. **Review** the candidate when the review mode is enabled. Record the lineage
   in the feature document. Never mark a review approved by hand.
8. **Commit and push only when the user asks.** Conventional Commits, no AI
   attribution, one commit per work unit.

## Routing

| Work | Route |
|------|-------|
| Read 1-3 files to decide or verify | Inline |
| Read 4+ files to understand | One delegated mapper |
| One mechanical, already-understood file | Inline |
| 2+ non-trivial files | One delegated writer, sequential tasks |
| Tests, builds, installs | Inline or one fresh worker, no change of route |
| `git`, `gh` state checks | Inline |

Keep a single writer per branch. Parallel writers only in isolated worktrees with
explicit approval.

## TDD

When the project configuration enables strict TDD, every behavior task follows
RED then GREEN. Record the RED evidence (for example, compile failure on an
undefined symbol, or a failing assertion) and the GREEN result. Contract-only and
documentation-only tasks record why no RED is possible.

## Persistence

- Engram mirror: topic key `odd/<feature>/tasks`. Save it after each completed
  task group and at session end.
- Session resume point: topic key `odd/<feature>/resume-point`.
- Do not create `sdd/*` artifacts for ODD features.

## Stop points

Stop and report when:

- A product decision is needed that the docs do not settle.
- A check fails twice with the same cause.
- The size budget is exceeded and the split is not yet decided.
- The user asked to pause.

## What ODD does not do

- No proposal, spec, design, or tasks phases as separate artifacts.
- No SDD session preflight, init guard, or chain-strategy menu.
- No automatic gatekeeper loop. A failed check is a stop point, not a retry loop.
