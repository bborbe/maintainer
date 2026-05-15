---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-10T19:02:39Z"
generating: "2026-05-10T19:24:48Z"
prompted: "2026-05-10T19:28:06Z"
verifying: "2026-05-10T20:24:05Z"
completed: "2026-05-12T23:09:14Z"
branch: dark-factory/pr-review-task-type-and-parked-assignee
---

## Summary

- Every task emitted by the github-pr watcher carries `task_type: pr-review` so the operator can look up which agent owns the work by type alone.
- Tasks parked for genuine human attention (untrusted author route) are emitted with an empty `assignee`, matching the cross-repo doctrine that empty assignee is the inbox signal.
- Trusted force-push resets explicitly re-claim the task for `pr-reviewer-agent`, because the controller may have cleared assignee while the task was parked.
- One-package change confined to the github-pr watcher; no controller, frontend, registry, or backfill work.

## Problem

The cross-repo doctrine (kafka-schema-design.md, Status Mapping table, 2026-05-10 update) now treats `assignee: ""` as the single visibility flag for "needs human attention," independent of phase. `task_type` is the immutable routing primitive the operator uses to know which agent owns a kind of work. The github-pr watcher currently violates both rules:

1. Tasks parked at creation for human review (untrusted author) ship with `assignee: pr-reviewer-agent`, even though the agent will never act on them and the operator inbox depends on assignee emptiness to surface them.
2. No emitted task carries `task_type`, so the operator cannot route by task type and the pipeline concept's central routing primitive is missing from this emitter.

## Goal

After this work, every task and frontmatter update produced by the github-pr watcher conforms to the doctrine:

- `task_type: pr-review` is present on every emitted creation and every force-push update.
- Tasks parked for human review on the untrusted-author route carry `assignee: ""`.
- Trusted force-push resets re-assert `assignee: pr-reviewer-agent` so a previously parked-and-cleared task is reclaimed by the agent when the head changes to trusted code.

## Assumptions

- `vault-cli`'s `FrontmatterMap` (in `pkg/domain/frontmatter_map.go`) preserves all fields — including unknown ones — through read-write cycles. `UpdateFrontmatterCommand` payloads can therefore specify only the keys this watcher changes (`task_type`, `assignee`) without listing or clobbering other frontmatter fields the controller wrote.

## Non-goals

- Controller-side clearing of assignee on transient or genuine failures (separate spec, already approved in `agent` repo as `021-clear-assignee-on-escalation-and-reset-trigger-count-on-redelegation`).
- Task-orchestrator / frontend changes to render the new shape.
- Agent registry or "delegate to default agent" UX.
- Backfilling existing vault tasks created before this change (new emissions only).
- Adding a `failure_class` field to agent verdict JSON (separate task).
- Any change to the build-watcher or other watchers.

## Desired Behavior

1. A trusted PR with a new head emits a task carrying `task_type: pr-review` and `assignee: pr-reviewer-agent`.
2. An untrusted-author PR emits a parked task carrying `task_type: pr-review` and `assignee: ""` (explicit empty string in the emitted frontmatter map).
3. A force-push on a previously-trusted PR publishes an `UpdateFrontmatterCommand` that includes `task_type: pr-review` and `assignee: pr-reviewer-agent`, claiming the task for the agent even if the controller had cleared assignee while it was parked.
4. A force-push on an untrusted-author PR publishes an `UpdateFrontmatterCommand` that includes `task_type: pr-review` and `assignee: ""`, keeping the task parked-and-unclaimed.

## Constraints

- Single file under change: `watcher/github-pr/pkg/watcher.go`. Co-located test file updates as needed.
- Frozen contract: doctrine values come from `agent/docs/kafka-schema-design.md` Status Mapping table (2026-05-10) — `task_type: pr-review`, `assignee: ""` for parked, `assignee: pr-reviewer-agent` for claimed. Do not invent alternative spellings.
- The conceptual model is anchored in `~/Documents/Obsidian/Personal/50 Knowledge Base/Agent Pipeline Concept.md` (Two orthogonal axes; Before / after sections). The spec follows that model — do not reinterpret.
- All existing tests in `watcher/github-pr/pkg/watcher_test.go` must continue to pass, updated for the new fields where they assert on frontmatter.
- `make precommit` in `watcher/github-pr` must pass (format + generate + test + lint + license).
- No change to verdict JSON, Kafka topic names, or any cross-service contract.
- No new fields beyond `task_type` and the assignee adjustment.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Untrusted author at creation | Task emitted with `task_type: pr-review`, `assignee: ""`, `phase: human_review`, `status: todo` | Operator picks task from inbox by empty-assignee filter |
| Force-push on parked (untrusted) PR | Update includes `task_type: pr-review`, `assignee: ""`; task stays parked | Operator handles in vault |
| Force-push on trusted PR after controller cleared assignee | Update includes `task_type: pr-review`, `assignee: pr-reviewer-agent`; controller spawns agent | Automatic |
| Existing tasks created before this change | Unaffected — no backfill. First subsequent update from a force-push will retrofit `task_type` and assignee | Operator may manually edit pre-change tasks if desired |

## Security / Abuse Cases

No new attack surface. No new user input, no new HTTP path, no new file write. The change writes constant string literals into outgoing Kafka command payloads.

## Acceptance Criteria

- [ ] `buildFrontmatter` returns frontmatter containing `task_type: pr-review`.
- [ ] `buildHumanReviewFrontmatter` returns frontmatter containing `task_type: pr-review` and `assignee: ""` (explicit empty string).
- [ ] Trusted-branch `publishForcePush` emits an `UpdateFrontmatterCommand` whose updates include both `task_type: pr-review` and `assignee: pr-reviewer-agent`.
- [ ] Untrusted-branch `publishForcePush` emits an `UpdateFrontmatterCommand` whose updates include both `task_type: pr-review` and `assignee: ""`.
- [ ] Unit tests in `watcher/github-pr/pkg/watcher_test.go` cover all four sites and pin the new field values.
- [ ] `make precommit` in `watcher/github-pr` passes.
- [ ] No regression: every previously passing test in `watcher/github-pr/pkg/` still passes.

No new scenario test. The behavior is fully reachable by unit tests against `CreateCommand` and `UpdateFrontmatterCommand` payloads — no Kafka, no controller, no agent run, no PVC required.

## Verification

```
cd ~/Documents/workspaces/maintainer/watcher/github-pr && make precommit
```

Expected: exit 0; tests asserting the new frontmatter fields pass.

## Do-Nothing Option

If we skip this, the github-pr watcher remains incompatible with the 2026-05-10 doctrine refinement: parked tasks ship with a misleading `pr-reviewer-agent` assignee that hides them from operator-inbox queries, and no emitted task carries `task_type`, blocking any operator tooling that routes by task type. Untrusted PRs in particular would languish — emitted as "claimed by an agent that will never run them." Not acceptable; this is the emitter side of the operator-visibility task that has already started shipping on the controller side.

## Verification Result

**Verified:** 2026-05-12T23:09:01Z (HEAD f637c04)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.156.1-1-g04f3863)
**Scenario:** Source inspection + fresh `make precommit` in watcher/github-pr (spec explicitly unit-test scoped, no scenario file)
**Evidence:**
- watcher.go:351,370-371,277-278,290-291 — all four emission sites carry `task_type: "pr-review"` with correct assignee (`pr-reviewer-agent` for trusted, `""` for parked/untrusted)
- watcher_test.go:844-845,975-976,1000-1001,918-919,1052-1053,1106-1107 — unit tests pin new field values across creation + force-push for both trust paths
- `make precommit` → "ready to commit"; gosec 0 issues; trivy 0 vulns
- `go test -race -cover` → pkg coverage 93.4%, all packages `ok`
**Verdict:** PASS
