---
status: approved
tags:
    - dark-factory
    - spec
approved: "2026-05-23T14:04:16Z"
generating: "2026-05-23T14:04:17Z"
branch: dark-factory/bug-pr-reviewer-planning-stale-phase-name
---

## Summary

- The pr-reviewer agent's planning step advances non-empty-concerns tasks with `NextPhase: "in_progress"`, but spec 032 renamed the canonical phase value from `in_progress` to `execution` on 2026-05-20. The agentlib frontmatter validator now rejects the stale literal, the result-deliverer logs `ignoring invalid NextPhase "in_progress"`, the task short-circuits to `done`, and the PR receives zero reviews.
- The same stale literal exists in four places: `pkg/steps_planning.go:102`, `pkg/factory/factory.go:195`, `pkg/steps_planning_test.go:267`, and `k8s/maintainer-agent-pr-reviewer.yaml` `trigger.phases:`.
- Spec 032 (the rename) missed all four sites; spec 034 (always-post LGTM) wrote new planner code that re-cemented the old literal. The bug ships in v0.25.10 (commit `a9b97e0`) and was observed 2026-05-23 on `bborbe/trading#134`.
- The fix replaces the literal with `domain.TaskPhaseExecution` (an exported `string`-typed constant already present in the codebase — confirmed via `grep -rn 'TaskPhaseExecution' agent/`) at the three Go sites and the bare string `execution` at the YAML site.
- After this fix, planning advances non-empty-concerns tasks to the execution phase, the result-deliverer accepts the write, the executor Job spawns, ai_review runs, and the bot posts a real review with verdict — restoring the spec 034 F2 invariant for the non-empty-concerns branch.

## Problem

Observed 2026-05-23 on `bborbe/trading#134` running through the prod pr-reviewer agent (image v0.25.10, freshly deployed with spec 034 F2 always-post code). The planning phase produced a non-empty `concerns: [...]` array and attempted to advance the task to the execution phase. The deployed binary set `NextPhase: "in_progress"` in the result, but the agentlib frontmatter validator (introduced by spec 032) rejected the write:

```
W result-deliverer.go:217] task 65025735-...: ignoring invalid NextPhase "in_progress": unknown task phase 'in_progress': validation error
{"Status":"done","NextPhase":"in_progress","Message":"","ContinueToNext":false}
```

The task fell back to `phase: done` with `trigger_count: 1`, never spawning the execution-phase Job, never reaching the review-POST step. The PR received zero reviews — same observable symptom as the silent-skip bug that spec 034 was supposed to fix, but a different root cause.

Spec 032 (`rename-task-status-phase-taxonomy`, shipped 2026-05-20) renamed the canonical phase value from `in_progress` to `execution`. Spec 032 updated default values in `main.go` flag declarations and the watcher's `BuildTaskStatus` default, but did NOT update:

1. The planner's hard-coded `NextPhase: "in_progress"` literal at `pkg/steps_planning.go:102` (the offending site introduced by spec 034 prompt 130, which copied the pre-existing literal pattern instead of using the renamed phase).
2. The factory's `NewPhase("in_progress", ...)` literal at `pkg/factory/factory.go:195`.
3. The Config CR's `trigger.phases:` list in `k8s/maintainer-agent-pr-reviewer.yaml:47` (still `in_progress`, not `execution`).
4. The planner unit test's assertion at `pkg/steps_planning_test.go:267`.

All four use the stale `"in_progress"` phase name. The validator rejects writes but accepts reads (compat), so the agent runs, plans, attempts to advance, gets silently rejected, and short-circuits to `done`. This is a clear regression — spec 032 should have caught all four sites; spec 034 reintroduced one while writing new planner code.

Note: the YAML file also contains `statuses: - in_progress` (line 44), which is the canonical *status* value (spec 032 only renamed phases, not statuses). That occurrence is correct and stays.

## Goal

After this work, the planner uses `domain.TaskPhaseExecution` (canonical value `execution`) as the next phase, the factory registers the second phase under that name, the Config CR triggers on `execution`, and the unit test asserts the renamed value. Real PRs with non-empty `concerns` advance planning → execution → ai_review correctly, the bot posts a real review with verdict, and the spec 034 F2 invariant ("bot visible on every PR") holds for the non-empty-concerns branch.

## Non-goals

- Do NOT introduce a phase-name alias map or backwards-compat shim in pr-reviewer code. Spec 032's invariant is "one canonical name per phase"; aliases would re-create the drift this spec is fixing. (The agentlib's read-side normalisation already exists for legacy frontmatter compat; this spec does not touch it.)
- Do NOT touch other services (`watcher/github-pr`, `watcher/github-build`, other agents). Their phase literals are independently governed and out of scope.
- Do NOT change the agentlib validator behavior — the validator is doing the right thing by rejecting the stale name.
- Do NOT add new failure-mode handling. Once the rename is applied, existing failure routing handles all paths.
- Do NOT change the status-axis literal `in_progress` (line 44 in the YAML) — spec 032 kept `in_progress` as a canonical status value; only the phase axis was renamed.

## Reproduction

**Triggering incident (verbatim evidence on file):**

- PR: `bborbe/trading` #134 (2026-05-23, prod cluster, image v0.25.10 at commit `a9b97e0`).
- Task vault page: planning phase ran, concerns array non-empty, result delivered `{"Status":"done","NextPhase":"in_progress",...}`.
- Result-deliverer log: `W result-deliverer.go:217] task 65025735-...: ignoring invalid NextPhase "in_progress": unknown task phase 'in_progress': validation error`.
- Vault frontmatter after run: `phase: done`, `trigger_count: 1`. No execution Job spawned. No review on GitHub.

**Minimal in-process reproduction:**

1. Construct a planning-step input where the model emits a `## Plan` JSON block with non-empty `concerns`.
2. Call the planning step against an in-process fake markdown.
3. Observe today: result has `NextPhase: "in_progress"` (rejected by validator at delivery time).
4. Expected after fix: result has `NextPhase: "execution"` (accepted by validator).

**Live reproduction (post-deploy gate):**

1. Open a dev PR on `bborbe/go-skeleton` containing at least one obvious concern (e.g. `fmt.Println` in production code, stdlib `errors.New`).
2. Trigger the reviewer-agent task for that PR.
3. Observe today: planning runs, the result-deliverer logs the invalid-NextPhase warning, task ends at `phase: done` with no execution Job and no GitHub review.
4. Expected after fix: planning advances to `execution`, execution Job runs, ai_review Job runs, GitHub shows a bot review with verdict.

## Expected vs Actual

**Expected** (per spec 032 invariant and spec 034 F2 "bot visible on every PR"): the planner emits `NextPhase` using the canonical phase taxonomy. Non-empty-concerns tasks reach the execution phase, the executor spawns, the ai_review phase spawns, and the bot posts a real review on GitHub.

**Actual** (observed 2026-05-23 on `bborbe/trading#134`): the planner emits the stale `in_progress` phase literal; the agentlib validator at the result-deliverer rejects the write; the task short-circuits to `done`; no execution Job spawns; the PR receives zero reviews.

## Why this is a bug

Three independent invariants are broken:

1. **Canonical phase name invariant (spec 032).** Spec 032 declared exactly one canonical phase value per phase. The planner uses a value that no longer exists in the taxonomy. The validator is acting correctly by rejecting it; the planner is acting incorrectly by emitting it.
2. **Spec 034 F2 invariant.** Spec 034 promised that the bot is visible on every PR (LGTM on empty concerns, full review on non-empty). The empty-concerns path works; the non-empty path silently exits at planning. F2 is half-delivered.
3. **Code/config drift.** The same stale literal exists in code (`steps_planning.go`, `factory.go`), test (`steps_planning_test.go`), and config (`maintainer-agent-pr-reviewer.yaml`). All four must move together; missing any one re-creates the drift.

## Desired Behavior

1. The planning step, when concerns are non-empty, returns a result whose `NextPhase` equals the canonical execution phase value (`execution`), referenced via the exported constant `domain.TaskPhaseExecution`.
2. The factory registers the second phase under the canonical execution phase value, referenced via the same constant.
3. The Config CR (`maintainer-agent-pr-reviewer.yaml`) `trigger.phases:` list contains `execution` instead of `in_progress`. Hard switch — no one-cycle compat. (Spec 032 already shipped a hard switch on the read-normalisation side; the trigger axis is the same generation.)
4. The planner unit test asserts the canonical execution phase value.
5. A grep across `agent/pr-reviewer/pkg/` and `agent/pr-reviewer/k8s/` for the literal `"in_progress"` returns zero matches in non-test production code paths (test files exercising legacy frontmatter alias compat, such as `domain_normalize_test.go`, are allowed to keep the literal because they assert the read-side compat path).

## Constraints

- All errors via `github.com/bborbe/errors`.
- All logging via `github.com/golang/glog`.
- BSD-style license header preserved on all touched files.
- CHANGELOG entry under the next-release heading dark-factory cuts on prompt completion.
- The canonical constant `domain.TaskPhaseExecution` MUST be used at all three Go sites — evidence-resolution rule for the audit: `grep -rn 'TaskPhaseExecution' agent/pr-reviewer/` already matches in `domain_normalize_test.go`, confirming the constant is exported and addressable from the pr-reviewer module. The YAML site uses the bare string `execution` (YAML cannot reference Go constants).
- Spec 032 renamed `in_progress` → `execution` on the phase axis and `todo` → `next` on the status axis. This spec only touches the phase-axis leftover in pr-reviewer; the status axis is already correct (the YAML's `statuses: - in_progress` at line 44 stays untouched).
- Existing passing tests under `agent/pr-reviewer/pkg/` MUST continue to pass except the one assertion in `steps_planning_test.go:267` that encodes the stale literal; that assertion is re-pointed to the canonical constant.
- Domain rules referenced: `specs/completed/032-rename-task-status-phase-taxonomy.md` (the rename this spec restores), `specs/completed/034-pr-reviewer-always-post-lgtm.md` (the F2 invariant this spec un-breaks).
- Verification ladder per `docs/verifying-specs.md`: Rung-1 (local `make precommit` + updated unit test assertion) is the primary gate; Rung-2 (dev deploy + live PR with non-empty concerns) is the live-evidence gate; Rung-3 (prod) only after Rung-2 passes.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---|---|---|---|
| Planning emits non-empty concerns | Result has `NextPhase = domain.TaskPhaseExecution`; validator accepts; executor Job spawns | Unit test row; result-deliverer log shows no `ignoring invalid NextPhase` warning | None needed — happy path |
| Planning emits empty concerns | LGTM path runs unchanged (spec 034 F2); result `NextPhase` is empty; task ends at `done` | Existing spec 034 evidence (LGTM POST to GitHub) | None needed — unchanged behavior |
| Config CR trigger.phases still lists only `in_progress` after rollout | Controller never triggers on `execution`-phase tasks; execution Job never spawns | `kubectlquant -n dev get config maintainer-agent-pr-reviewer -o yaml | yq .spec.trigger.phases` shows the stale list; spawned Job count stays 0 | Re-apply the manifest with the corrected list |
| Factory registers `in_progress` phase but planner emits `execution` | The agentlib runtime cannot find a phase matching `execution`; executor Job fails to dispatch the step | Pod logs in dev cluster show phase-lookup failure | This spec changes both sites in the same commit; only triggered if a partial apply landed |
| Two pods race during dev rollout (old image still serving planning while new image is deployed) | Old pod emits stale literal, gets rejected by validator (current bug); new pod emits canonical, gets accepted | Mixed pod logs during rollout window | Force-reset the affected vault tasks after rollout completes; behaviour converges within one trigger cycle |
| Live PR with non-empty concerns, post-fix, prod | Bot posts a real review with verdict; PR shows `state: APPROVED` / `CHANGES_REQUESTED` / `COMMENTED` from the bot identity | `gh api /repos/<owner>/<repo>/pulls/<N>/reviews | jq` returns ≥1 review from the bot identity at the current head SHA | None needed — happy path |
| Validator behavior changes upstream (agentlib bump removes the validator) | Stale literal would silently re-pass; bug would resurface as wrong-named-phase rather than rejected | Out of scope — spec 032 owns validator contract | Would require a new spec to re-align |

## Security / Abuse Cases

Not applicable. This is a code-internal constant rename. The literal value is not attacker-controllable, does not cross a trust boundary, and is not user-supplied. The Config CR change is operator-controlled and goes through the standard manifest-apply flow.

## Acceptance Criteria

Rung-1 (code + tests):

- [ ] A grep across pr-reviewer production code returns zero stale-literal matches — evidence: `grep -rn '"in_progress"' agent/pr-reviewer/pkg/ agent/pr-reviewer/k8s/ --include='*.go' --include='*.yaml'` returns 0 lines, excluding (a) matches inside `*_test.go` files that exercise the legacy-frontmatter read-compat path (e.g. `domain_normalize_test.go`), and (b) the YAML `statuses: - in_progress` line which is the canonical status (not phase) value.
- [ ] The planning step references the canonical phase constant — evidence: `grep -n 'NextPhase' agent/pr-reviewer/pkg/steps_planning.go` shows the assignment uses `domain.TaskPhaseExecution` (not the string literal `"in_progress"` or `"execution"`).
- [ ] The factory registers the second phase using the canonical constant — evidence: `grep -n 'NewPhase' agent/pr-reviewer/pkg/factory/factory.go` shows the second `NewPhase(...)` call's first argument is `domain.TaskPhaseExecution` (or, if `NewPhase` takes a `string` parameter and a typed constant cannot be passed, the explicit string `"execution"`; record which path was taken in the PR description).
- [ ] The k8s Config CR triggers on the canonical phase — evidence: `grep -nA3 'trigger:' agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer.yaml` shows `phases:` containing `execution` and not containing `in_progress`.
- [ ] The planner unit test asserts the canonical phase value — evidence: `grep -n 'TaskPhaseExecution\|"execution"' agent/pr-reviewer/pkg/steps_planning_test.go` returns ≥1 line in the non-empty-concerns assertion block; `grep -n '"in_progress"' agent/pr-reviewer/pkg/steps_planning_test.go` returns 0 lines.
- [ ] All package tests pass — evidence: `cd agent/pr-reviewer && go test ./pkg/...` exits 0.
- [ ] Local precommit passes — evidence: `cd agent/pr-reviewer && make precommit` exits 0.
- [ ] Reverting the planner assignment back to `"in_progress"` causes the updated unit test to fail — evidence: on a scratch branch with the assignment reverted, `cd agent/pr-reviewer && go test ./pkg/...` exits non-zero and the failure output names the planner test row.

Rung-2 (dev deploy + live verification):

- [ ] **Post-Deploy (Rung-2):** dev cluster runs the new image — evidence: `cd agent/pr-reviewer && BRANCH=dev make buca` (or the `/make-buca agent/pr-reviewer dev` slash-command alias) succeeds and `kubectlquant -n dev rollout status statefulset/agent-pr-reviewer --timeout=120s` reports complete.
  - deploy_check: `kubectlquant -n dev describe statefulset agent-pr-reviewer | grep 'Image:'` shows the new image tag.
  - deploy_target: dev cluster.
- [ ] **Post-Deploy (Rung-2):** A dev PR on `bborbe/go-skeleton` containing at least one obvious concern (e.g. `fmt.Println` in production code, stdlib `errors.New`) advances planning → execution → ai_review and the bot posts a full review — evidence: `gh api '/repos/bborbe/go-skeleton/pulls/<N>/reviews' | jq '[.[] | select(.user.login=="ben-s-pull-request-reviewer-dev[bot]")] | .[-1].state'` returns one of `"APPROVED"`, `"CHANGES_REQUESTED"`, or `"COMMENTED"`, AND `jq '[.[] | select(.user.login=="ben-s-pull-request-reviewer-dev[bot]")] | .[-1].body'` contains the full verdict structure (not just the LGTM template body shipped by spec 034).
  - deploy_check: spawned Job logs show no `"ignoring invalid NextPhase"` warning — `kubectlquant -n dev logs -l agent=pr-reviewer --since=15m | grep 'ignoring invalid NextPhase'` returns 0 lines (label selector across all recent pr-reviewer pods — no need to know the exact Job name).
  - deploy_target: dev cluster Job spawned via the watcher pipeline.

Rung-3 (prod cutover after dev verifies):

- [ ] **Post-Deploy (Rung-3):** prod cluster runs the new image — evidence: `kubectlquant -n prod describe statefulset agent-pr-reviewer | grep 'Image:'` shows the new tag and `kubectlquant -n prod rollout status statefulset/agent-pr-reviewer --timeout=120s` reports complete.
  - deploy_check: rollout status complete.
  - deploy_target: prod cluster.
- [ ] **Post-Deploy (Rung-3):** the discovery case `bborbe/trading#134` receives a real review after a vault-task reset (close + reopen the PR or reset the task frontmatter) — evidence: before deploy, capture the cutover timestamp with `date -u +%FT%TZ > /tmp/bug-phase-cutover.txt`; after deploy + close+reopen, `gh api '/repos/bborbe/trading/pulls/134/reviews' | jq --arg cutover "$(cat /tmp/bug-phase-cutover.txt)" '[.[] | select(.user.login=="ben-s-pull-request-reviewer[bot]" and .submitted_at > $cutover)] | .[-1].state'` returns `"APPROVED"`, `"CHANGES_REQUESTED"`, or `"COMMENTED"` (a review submitted strictly after the cutover, not a stale review from a previous run).
  - deploy_check: spawned Job logs in prod show no `"ignoring invalid NextPhase"` warning — `kubectlquant -n prod logs -l agent=pr-reviewer --since=15m | grep 'ignoring invalid NextPhase'` returns 0 lines (label selector across all recent pr-reviewer pods).
  - deploy_target: prod cluster.

**Scenario coverage — NO new scenario.** This is a constant-rename fix fully covered by the updated planner unit test (Rung-1) and the live cluster verification (Rungs 2 and 3). Adding an E2E scenario file would duplicate either layer and provide no additional regression value.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Expected: exit 0. The updated planner test asserts the canonical phase value.

Revert-test (confirms the unit test actually exercises the rename):

```
cd agent/pr-reviewer
# Temporarily revert pkg/steps_planning.go NextPhase assignment to "in_progress".
go test ./pkg/...
# Expected: non-zero exit; failure cites the planner non-empty-concerns row.
# Revert the change before continuing.
```

Rung-2 live verification (after dev deploy):

```
# Pick a dev test PR (e.g., bborbe/go-skeleton) with at least one obvious concern.
# Wait for the spawned pod cycle:
kubectlquant -n dev get pods -l app=pr-reviewer-agent --sort-by=.metadata.creationTimestamp

# Confirm planning advanced (no rejection warning in any pod log):
kubectlquant -n dev logs <controller-pod> --since=10m | grep 'ignoring invalid NextPhase' || echo "OK: no rejections"

# Confirm review posted:
gh api /repos/bborbe/go-skeleton/pulls/<N>/reviews \
  | jq '[.[] | select(.user.login=="ben-s-pull-request-reviewer-dev[bot]")] | .[-1] | {state, submittedAt, body}'
```

Expected: a review from the bot identity with non-empty body containing the full verdict structure, `state` in `{APPROVED, CHANGES_REQUESTED, COMMENTED}`.

## Do-Nothing Option

Not viable. Leaving the stale literal means spec 034's F2 fix delivers only half its promise: the LGTM-on-empty-concerns path works, but the non-empty-concerns path — the more important one, because that is where the agent's actual review value lives — silently exits at planning. Every real PR with actual concerns continues to receive zero feedback. This defeats both spec 033 (App auth, visible reviews) and spec 034 (bot on every PR). The fix is four lines across four files; the do-nothing cost is the entire reason both prior specs were shipped.
