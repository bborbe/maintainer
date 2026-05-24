---
status: completed
tags:
    - dark-factory
    - draft
approved: "2026-05-24T20:28:53Z"
generating: "2026-05-24T20:28:54Z"
prompted: "2026-05-24T20:33:14Z"
verifying: "2026-05-24T22:43:03Z"
completed: "2026-05-24T22:52:53Z"
branch: dark-factory/github-build-watcher-filter-dependabot-graph-update
---

## Summary

`watcher/github-build` should skip workflow runs whose name starts with `Graph Update:` or `Dependabot Updates` — these are Dependabot internal graph-maintenance jobs, not real CI. Their failures don't represent code bugs and shouldn't surface as OpenClaw build-failure tasks.

## Problem

On 2026-05-24 the build-fix-agent (stage 1, `/task-to-bug-spec`) processed 5 OpenClaw build-failure tasks. **5 of 5** were Dependabot graph-update workflow failures (`Graph Update: go_modules`) failing with HTTP 503 — Dependabot's own service being temporarily flaky. The real CI workflows on the SAME commits had succeeded. The build was green; the watcher had no business emitting these as build-failure tasks.

Concrete cost:
- Every Dependabot 503 episode generates noise tasks the agent has to process to verdict `no_fix_needed`.
- Operator visibility wasted: 5 OpenClaw tasks in TODO that weren't actionable.
- Stage-2 Pattern B Job will burn Kafka cycles + LLM tokens on these false positives.

Today the agent's Step 5b filters these correctly — but the right fix is detector-side, not consumer-side. If the watcher never emits them, the agent never needs to filter them.

## Goal

`watcher/github-build` does NOT publish a `CreateTaskCommand` for any workflow run whose `workflow.name` starts with `Graph Update:` or `Dependabot Updates`. These runs are silently ignored. Real CI workflows on the same SHA are still evaluated normally.

## Desired Behavior

- In the watcher's per-run evaluation, add a workflow-name filter that returns early when the name matches the Dependabot internal pattern. Implementing agent picks the exact file/function — typically `pkg/watcher.go` near the GitHub API result iteration, but the symbol location is implementation detail.
- Filter is applied **before** the red/green state-machine logic so these workflows have no effect on cursor advancement or task emission.
- The filtered prefixes are **hardcoded constants** in the watcher package: `"Graph Update:"` and `"Dependabot Updates"`. No env-var configuration — these are Dependabot-owned conventions, not operator policy. Adding a third prefix later is a code change + new spec, not a config flag.

## Constraints

- Match against the GitHub API field `workflow.name` (the user-visible name, what gh/UI shows). NOT workflow file path, workflow id, job name, or `display_title`.
- Prefix match is **case-sensitive** (the literal Dependabot strings always start with capital `G` / `D`).
- Filter applies only to `watcher/github-build`; the PR watcher is unaffected.
- Do NOT skip real CI workflows that happen to fail at the same time as a Dependabot internal — emit a task only if a real CI workflow failed.
- Empty / nil `workflow.name` from the API → treat as non-matching (do NOT panic, do NOT skip — emit the task if the run otherwise meets emission criteria).

## Acceptance Criteria

1. **Unit test — mixed case**: Given two failing workflow runs on the same SHA with names `"CI"` and `"Graph Update: go_modules"`, the watcher's mock command-publisher receives **exactly one** `CreateTaskCommand`, and that command's `workflow_name` (or equivalent body field carrying the originating workflow name) equals `"CI"`.
2. **Unit test — pure Dependabot case**: Given one failing workflow run on a SHA with name `"Graph Update: go_modules"` and no other failing runs, the watcher's mock command-publisher receives **zero** `CreateTaskCommand`s.
3. **Unit test — `Dependabot Updates` variant**: Same as #2 but with name `"Dependabot Updates"`. Zero commands.
4. **Unit test — case sensitivity guard**: Workflow name `"graph update: x"` (lowercase) does NOT match the filter; one command emitted.
5. **Unit test — nil/empty name guard**: Workflow name `""` (or nil pointer) does NOT crash; treated as non-matching; one command emitted (assuming other emission criteria met).
6. `cd watcher/github-build && make precommit` exits 0.
7. `grep -n 'Graph Update\|Dependabot Updates' CHANGELOG.md` returns ≥1 line under `## Unreleased` documenting the new filter.

## Failure Modes

| Trigger | Expected | Recovery |
|---|---|---|
| Dependabot renames their internal workflow pattern in the future (e.g. `Graph Update v2:`) | New pattern not filtered; the agent's stage 1 / stage 2 Step 5b downstream filter catches them as `no_fix_needed`; no task spam in OpenClaw long-term because they auto-close, but more agent runs than necessary | File a follow-up spec to add the new prefix to the hardcoded list |
| GitHub API returns a workflow run with empty / nil `workflow.name` | Filter does not match → run is evaluated normally (does not crash, does not skip silently) | None needed; covered by AC #5 |
| A user repo defines a custom workflow literally named `Graph Update: my-thing` | False negative — that workflow's failures don't generate tasks | Acceptable; the prefix is established Dependabot convention; if a real user repo collides, that's a renaming conversation, not a filter-config conversation |

## Do-Nothing Option

The agent's Step 5b handles these correctly at the consumer side (verdict `no_fix_needed`, task closes successfully). So functionally nothing breaks. But operator experience suffers: OpenClaw TODO column accumulates non-actionable items; stage-2 Pattern B Job burns LLM tokens on false positives; metrics show inflated agent-task volume that's mostly noise.

## Non-goals

- Filtering other Dependabot internals beyond `Graph Update:` and `Dependabot Updates` — only the two confirmed false-positive patterns. Adding new patterns is a follow-up data-driven decision.
- Per-repo filter customization — single global prefix list is sufficient.
- Filtering on the PR watcher — PRs are a different surface; no false positives observed there.
- Backfilling already-emitted tasks — the 5 from 2026-05-24 are already verdict'd; this spec is forward-only.

## Verification

- After deploy: trigger a Dependabot graph-update failure (e.g. wait for the next Dependabot run; they happen multiple times per day). Observe no new OpenClaw task is created.
- After deploy: trigger a real CI failure (push a broken commit to a test repo on the allowlist). Observe an OpenClaw task IS created with the real CI workflow's name in the title.

## Related

- Triggered by: 5 false positives processed today via `/task-to-bug-spec`. See [[Failed Build Fix Agent]] § "Real Stage 1 invocations (2026-05-24)" and [[Run Build Auto-Fix Prototype End-to-End]] Progress table.
- Companion spec: `040-github-build-watcher-task-suffix.md` (completed) — same code area.
- Companion vault task: [[Watcher Assigns Build-Failure Tasks to build-fix-agent]] (largely done after env-var deploy).
- Touches: `watcher/github-build/pkg/watcher.go` (or wherever the workflow filter belongs) — code only. No env-var changes, no k8s manifest changes, no `dev.env` / `prod.env` changes.
