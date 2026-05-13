---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-10T20:41:27Z"
generating: "2026-05-10T20:41:28Z"
prompted: "2026-05-10T20:44:37Z"
verifying: "2026-05-10T21:20:01Z"
branch: dark-factory/build-watcher-task-type-and-parked-assignee
---

## Summary

- Every task emitted by the github-build watcher carries `task_type: build-fix` so the operator can route by task type alone.
- When the configured assignee is the magic string `human`, the watcher emits `assignee: ""` — matching the cross-repo doctrine that empty assignee is the inbox / parked signal.
- Any other configured assignee value (e.g. `build-fix-planner`, `build-fixer-agent`) flows through unchanged.
- One-package change confined to the github-build watcher; no controller, frontend, registry, or backfill work. Mirrors the shape of the github-pr sibling spec (022).

## Problem

The cross-repo doctrine (`agent/docs/kafka-schema-design.md`, Status Mapping table, 2026-05-10 update) treats `assignee: ""` as the single visibility flag for "needs human attention," independent of phase. `task_type` is the immutable routing primitive the operator uses to know which agent owns a kind of work. The github-build watcher currently violates both rules:

1. No emitted task carries `task_type`, so the operator cannot route by task type and the pipeline concept's central routing primitive is missing from this emitter.
2. The watcher's assignee field is wired straight from CLI / env config. If the operator parks the watcher for human attention by setting the assignee to the literal string `human` (per [[Agent Hub]]: "assignee `human` initially → `build-fix-planner` later"), the watcher emits `assignee: human` verbatim. That's non-conformant — the doctrine says empty `""` means unclaimed/inbox; `human` is not an operator-inbox surface and will not be picked up by inbox queries.

## Goal

After this work, every task produced by the github-build watcher conforms to the doctrine:

- `task_type: build-fix` is present on every emitted creation.
- When the configured assignee value is the magic string `human`, the emitted frontmatter carries `assignee: ""`.
- When the configured assignee value is any agent name (e.g. `build-fix-planner`, `build-fixer-agent`, `go-deps-fixer-agent`), the emitted frontmatter carries that name unchanged.

## Assumptions

- `vault-cli`'s `FrontmatterMap` (in `pkg/domain/frontmatter_map.go`, verified 2026-05-10) preserves all fields — including unknown ones — through read-write cycles. The controller will round-trip `task_type` and an explicit empty `assignee` through to the vault file without dropping or rewriting them.
- The canonical `task_type` value for build-failure tasks is `build-fix` — chosen to match the planned consumer agent name root `build-fix-planner` per [[Agent Hub]].
- Translation of the `human` literal is a build-watcher-local concern in this spec. The same decision in any other watcher is out of scope.

## Non-goals

- Implementing `build-fix-planner` itself (separate task).
- Task-orchestrator / frontend changes to render the new shape.
- Agent registry or CRD `task_type` field — sibling agent-repo spec.
- Backfilling existing build-watcher tasks created before this change (new emissions only).
- The `human` → `""` decision for any other watcher (PR, sentry, deps, etc.).
- Changing the per-repo maintenance override mechanism — assignee resolution order (maintenance file → CLI/env default) is preserved as-is; the translation rule is applied to the final resolved value.

## Desired Behavior

1. A red build for an allowlisted repo, with the watcher configured with `assignee=build-fixer-agent` (or any non-`human` value), emits a task carrying `task_type: build-fix` and `assignee: build-fixer-agent`.
2. The same red build, with the watcher configured with `assignee=human`, emits a task carrying `task_type: build-fix` and `assignee: ""` (explicit empty string in the emitted frontmatter map).
3. A red build whose per-repo maintenance override sets `assignee: human` emits a task carrying `task_type: build-fix` and `assignee: ""`. (Override is resolved first; the `human` → `""` translation runs on the final resolved value, NOT on the raw CLI/env default.)
4. A red build whose per-repo maintenance override sets `assignee: build-fix-planner` emits a task carrying `task_type: build-fix` and `assignee: build-fix-planner`. (Final resolved value is an agent name — translation is a no-op.)
5. The `task_type` field is always present on emission, regardless of which assignee path was taken or whether `phase` is set.

## Constraints

- Single file under change: `watcher/github-build/pkg/watcher.go`. Co-located test file updates as needed (`watcher_test.go`, possibly `watcher_internal_test.go`).
- Frozen contract: doctrine values come from `agent/docs/kafka-schema-design.md` Status Mapping table (2026-05-10) — `task_type: build-fix`, `assignee: ""` for parked. Do not invent alternative spellings.
- The conceptual model is anchored in `~/Documents/Obsidian/Personal/50 Knowledge Base/Agent Pipeline Concept.md` ("Failure Within a Task" → "Two orthogonal axes"). The spec follows that model — do not reinterpret.
- No change to the watcher constructor signature or factory wiring. The CLI flag / env var that supplies `assignee` keeps its existing name and semantics; only the emission-site transformation changes.
- All existing tests in `watcher/github-build/pkg/watcher_test.go` must continue to pass, updated for the new field where they assert on frontmatter.
- `make precommit` in `watcher/github-build` must pass (format + generate + test + lint + license).
- No change to verdict JSON, Kafka topic names, or any cross-service contract.
- No new fields beyond `task_type` and the `human` → `""` translation.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Watcher configured with `assignee=human` | Task emitted with `task_type: build-fix`, `assignee: ""` | Operator picks task from inbox by empty-assignee filter |
| Watcher configured with `assignee=build-fixer-agent` (or any other agent name) | Task emitted with `task_type: build-fix`, `assignee: build-fixer-agent` | Controller spawns the named agent |
| Per-repo maintenance override sets `assignee: human` | Task emitted with `task_type: build-fix`, `assignee: ""` (translation applied after resolution) | Operator picks task from inbox |
| Existing tasks created before this change | Unaffected — no backfill. Future emissions on the same episode SHA are deduplicated by UUID5 task identifier, so pre-change tasks are not re-emitted | Operator may manually edit pre-change tasks if desired |

## Security / Abuse Cases

No new attack surface. No new user input, no new HTTP path, no new file write. The change writes a constant string literal (`build-fix`) and a deterministic transformation (`human` → `""`) into outgoing Kafka command payloads.

## Acceptance Criteria

- [ ] `buildCreateTaskCommand` returns frontmatter containing `task_type: build-fix` for all assignee inputs (default CLI/env value, custom agent name, per-repo maintenance override).
- [ ] When the resolved assignee equals the literal string `human`, the emitted frontmatter contains `assignee: ""` (explicit empty string, key present).
- [ ] When the resolved assignee is any other value (including the existing default `build-fixer-agent` and an agent-name override like `go-deps-fixer-agent`), the emitted frontmatter carries that value unchanged.
- [ ] Unit tests in `watcher/github-build/pkg/watcher_test.go` cover: (a) `task_type: build-fix` present in the default path, (b) `assignee=human` translates to `""`, (c) `assignee=build-fix-planner` flows through unchanged, (d) per-repo maintenance override of `assignee: human` translates to `""`.
- [ ] Existing tests that assert on `assignee` (e.g. `Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fixer-agent"))`) still pass; `task_type: build-fix` is added as an additional assertion where appropriate without weakening existing expectations.
- [ ] `make precommit` in `watcher/github-build` passes.
- [ ] No regression: every previously passing test in `watcher/github-build/pkg/` still passes.

No new scenario test. The behavior is fully reachable by unit tests against `CreateCommand` payloads — no Kafka, no controller, no agent run, no PVC required. (See `docs/scenario-writing.md` "When to Write a Scenario" — none of the four conditions hold here.)

## Verification

```
cd ~/Documents/workspaces/maintainer/watcher/github-build && make precommit
```

Expected: exit 0; tests asserting `task_type: build-fix` and the `human` → `""` translation pass.

## Do-Nothing Option

If we skip this, the github-build watcher remains incompatible with the 2026-05-10 doctrine refinement: emitted tasks carry no `task_type`, blocking any operator tooling that routes by task type, and an operator who configures the watcher with `assignee=human` to park work for human triage will instead ship tasks with the literal string `human` — invisible to inbox queries that filter on `assignee == ""`. Unacceptable; this is the build-side counterpart of the github-pr conformance work already approved as spec 022.

## Verification Result

**Verified:** 2026-05-13T06:47:42Z (HEAD 0a4bb05)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.156.1-1-g04f3863-dirty)
**Scenario:** `make precommit` in `watcher/github-build` per spec's Verification block — no runtime replay required (spec line 91-92: unit-test-only behavior, no Kafka/controller/PVC).
**Evidence:**
- `make precommit` exit 0, "ready to commit" (gosec 0 issues, trivy 0 vulns, vet/lint/test/license all green).
- `watcher.go:296` emits `"task_type": "build-fix"` unconditionally for every CreateCommand.
- `watcher.go:418-422` `translateAssignee` maps `"human"` → `""`, all others pass through.
- `watcher_test.go`: default-path task_type assertion (line 80), `human`→`""` (line 564-574), `build-fix-planner` passthrough (line 577-587), maintenance-override `human`→`""` (line 731-744), `go-deps-fixer-agent` override passthrough (line 693), existing `build-fixer-agent` assertions retained (lines 79, 468, 673).
**Verdict:** PASS
