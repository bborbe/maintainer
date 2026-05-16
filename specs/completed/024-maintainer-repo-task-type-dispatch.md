---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-05-14T13:32:23Z"
generating: "2026-05-14T14:17:39Z"
prompted: "2026-05-14T14:23:34Z"
verifying: "2026-05-14T15:36:13Z"
completed: "2026-05-15T14:31:25Z"
branch: dark-factory/maintainer-repo-task-type-dispatch
---

## Summary

- The maintainer-repo binary `maintainer-agent-pr-reviewer` currently runs its hardcoded domain agent regardless of which `task_type` the executor injects, so liveness/healthcheck tasks land as `failed` because the domain parser rejects their minimal body.
- This spec wires per-task-type dispatch into that one binary using the canonical `lib.AgentProvider` pattern from `agent/claude/pkg/factory/factory.go::CreateAgentProvider`: a new `CreateAgentProvider` factory returns an `agentlib.AgentProvider` whose dispatch table maps `TaskTypePRReview` to the existing domain agent and `TaskTypeHealthcheck` to a liveness agent. `main.go` calls `provider.Get(ctx, taskType)`.
- Two accepted task types — `pr-review` (existing behavior, unchanged) and `healthcheck` (new, routes to a liveness agent that replies `ok`). Unknown values fail via `AgentProvider.Get`'s built-in error (no hand-rolled switch).
- Reuses `lib/healthcheck` from `github.com/bborbe/agent/lib v0.62.16` (already pinned).
- One independently deployable change, scoped to a single binary, intended as a single prompt.

## Problem

The executor now injects a `task_type` field on every task it dispatches, and liveness probes carry `task_type: healthcheck`. The maintainer-repo binary `maintainer-agent-pr-reviewer` ignores this field — it always runs the same PR-review domain agent. When a probe task lands at that binary, the agent tries to parse domain-specific fields (clone URL, PR metadata) out of the probe's minimal `reply 'ok'` body, fails, and reports `human_review` or `failed`. Operators see noise instead of a green probe, and probe coverage of this binary is effectively zero.

## Goal

After this work is done, the `maintainer-agent-pr-reviewer` binary inspects the incoming `task_type` and dispatches to the correct agent: existing PR-review behavior for its domain task type, and a separate liveness agent for healthcheck tasks. Any other task type is rejected at start-up of the run with an explicit error listing accepted values, and the run completes with `Status: failed`. The binary is otherwise unchanged: same Kafka entry point, same metrics wiring, same env contract.

## Non-goals

- Trading-repo dispatch — covered by a separate, parallel spec.
- Adding any task type beyond `pr-review` and `healthcheck` to this binary.
- Removing or renaming any `lib.TaskType*` constants from the shared `lib/` module.
- Changing the executor's dispatch shape, env injection, or topic layout.
- Touching the maintainer `cli` binary or any watcher.
- Modifying the existing `CreateAgent` factory signature (it is retained verbatim and called internally by the new dispatch function).

## Desired Behavior

1. The factory exposes a new `CreateAgentProvider(<existing CreateAgent args>) agentlib.AgentProvider` function. Body: build the PR-review domain agent via the existing `CreateAgent` factory, build the liveness agent inline via `healthcheck.NewAgent(healthcheck.NewClaudeStep(claudeRunner))` (reusing the same Claude runner factory the binary already constructs for its domain work), then return `agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{agentlib.TaskTypePRReview: domainAgent, agentlib.TaskTypeHealthcheck: livenessAgent})`. Pure plumbing — no `switch`, no error return.
2. The existing `CreateAgent` factory is retained verbatim and continues to be called by `cmd/run-task/main.go` directly. The new `CreateAgentProvider` calls `CreateAgent` once at construction to obtain the domain agent — there is no duplicate construction.
3. The Kafka entry point in `main.go` constructs the provider once, then calls `agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))`. On error, the existing job-metrics failure path (`RecordRun(AgentStatusFailed)` + `RecordDuration` + wrapped error) records and returns.
4. A `task_type` of `pr-review` runs the existing 3-phase PR-review agent. Behavior identical to today for the production path.
5. A `task_type` of `healthcheck` runs the liveness agent. The healthcheck path produces a task page whose body is `ok` and whose status is `done`, matching the shape produced by the same dispatch in the agent-repo binaries.
6. Any other `task_type` value (empty, unknown, typo) is rejected by `lib.AgentProvider.Get`. Its error message format is `"unknown task_type %q for %s; accepted: %v"` with the binary's serviceName and the sorted accepted-types list — the binary does NOT hand-roll a separate error.
7. The binary's `CHANGELOG.md` gains an `Unreleased` entry: `feat(agent/pr-reviewer): per-task-type dispatch via factory.CreateAgentProvider — healthcheck task type now routes to a dedicated liveness agent`.

## Constraints

- `github.com/bborbe/agent/lib` is already pinned at `v0.62.16` (which exports `lib/healthcheck` with `NewAgent`/`NewClaudeStep`/`NewGeminiStep`/`NewNopStep` and the `lib.AgentProvider` interface + `lib.NewAgentProvider` constructor). Verified by inspection of the tagged tree. No `go get` step is needed.
- The dispatch function name and signature is fixed: `CreateAgentProvider(<existing CreateAgent args>) agentlib.AgentProvider`. NO `ctx context.Context` parameter, NO error return — the function is pure plumbing. The dispatch table is built once and handed to `agentlib.NewAgentProvider`.
- The `name` argument to `agentlib.NewAgentProvider` is the binary's existing `serviceName` constant (defined at the top of `factory.go`).
- Healthcheck agent construction is two-step and lives inline at the provider construction site: `healthcheck.NewAgent(healthcheck.NewClaudeStep(claudeRunner))`. There is NO `NewClaudeHealthcheckAgent` shortcut — that name does not exist in the lib.
- The existing `CreateAgent` factory function is retained with no signature change — `cmd/run-task/main.go` calls it directly and must keep building.
- The existing env contract (`TASK_TYPE`, `PUSHGATEWAY_URL`, all metrics fields wired by a prior spec) is unchanged. No new env vars introduced.
- The Kafka entry point's metrics wiring (the deferred `PushContext`, the `RecordRun`/`RecordDuration` calls at every return path) must continue to fire on every code path, including the new dispatch-error path from `provider.Get`.
- The healthcheck branch must use the same Claude runner factory the binary already constructs for its domain work — no second runner, no second auth path. The probe must therefore exercise the same OAuth credentials the production agent uses.
- No new task type beyond `pr-review` and `healthcheck` is accepted by this binary; the dispatch map is a closed set, not an extension point.
- The Config CR's `taskTypes` list is already `[pr-review, healthcheck]` — `oauth-probe` was removed by direct edit after spec 032 (executor rename) shipped. No CRD changes in this spec; no `TaskTypeOAuthProbe` map entry.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `task_type` is empty | `provider.Get` returns an error naming the accepted types; metrics record a failed run; deferred push delivers metrics | Operator fixes the publishing watcher to set `task_type` |
| `task_type` is an unknown string (typo, future type not yet supported) | Same as empty: `provider.Get` error, failed run, metrics pushed | Operator inspects the failure body, fixes the publisher, retries |
| Claude runner factory fails to construct (e.g. missing OAuth secret) | Both branches fail identically — the runner is built once and shared between the domain agent and the liveness agent | Operator fixes credentials and retries; healthcheck path doubles as the canary that surfaces this |
| Lib module pinned below `v0.62.16` | Build fails because `lib.AgentProvider` and/or `lib/healthcheck` are not importable | Bump the pin; this is caught by `make precommit` before merge |
| Domain agent's parser rejects healthcheck body (current bug) | No longer reachable — healthcheck never enters the domain agent | N/A |

## Security / Abuse Cases

- The `task_type` field is operator-controlled (via the executor, which is in the trusted dispatch path) — not user-controlled. The dispatch map is a closed set, so an unexpected value cannot widen the binary's behavior; `lib.AgentProvider.Get` enforces this and returns a typed error without leaking config or secrets.
- The healthcheck path executes the same Claude runner as the domain path. It must not bypass any auth or sandboxing the domain path enforces; reusing the existing runner factory enforces this by construction.
- No new HTTP, file, or network surface is introduced. No new credentials are read.

## Acceptance Criteria

- [ ] The factory exposes a new `CreateAgentProvider(<existing CreateAgent args>) agentlib.AgentProvider` function. The existing `CreateAgent` function's signature is unchanged.
- [ ] `CreateAgentProvider`'s body wires `agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{agentlib.TaskTypePRReview: domainAgent, agentlib.TaskTypeHealthcheck: livenessAgent})` where `domainAgent` is the result of the existing `CreateAgent` and `livenessAgent` is built inline via `healthcheck.NewAgent(healthcheck.NewClaudeStep(claudeRunner))`. No `switch` statement, no error return.
- [ ] The Kafka entry point in `main.go` constructs the provider once and calls `agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))`, then runs `agent.Run(...)`.
- [ ] On every return path from the entry point — including the new dispatch-error path from `provider.Get` — the run-status and duration metrics are recorded and the deferred push fires.
- [ ] A test asserts `provider.Get(ctx, agentlib.TaskTypePRReview)` returns a non-nil agent and nil error.
- [ ] A test asserts `provider.Get(ctx, agentlib.TaskTypeHealthcheck)` returns a non-nil agent and nil error.
- [ ] A test asserts `provider.Get(ctx, agentlib.TaskType("bogus"))` returns a nil agent and an error whose message contains the literal `unknown task_type`, `"bogus"`, the binary's serviceName, and the accepted-types list (these are guaranteed by `lib.AgentProvider.Get`'s format string `"unknown task_type %q for %s; accepted: %v"`).
- [ ] `cmd/run-task/main.go` is unchanged and still builds (it calls the retained `CreateAgent` entry point).
- [ ] `go.mod` is NOT modified by this prompt. `github.com/bborbe/agent/lib` remains at the already-pinned `v0.62.16`.
- [ ] `CHANGELOG.md` gains one `Unreleased` bullet under the existing `Unreleased` section describing the dispatch change.
- [ ] `make precommit` passes inside the binary's directory.

**Scenario coverage — NO new scenario.** This change is covered end-to-end by the executor-side healthcheck pipeline shipped in agent-repo spec 033 (probe runner) combined with the agent-repo dispatch scenarios in spec 031. Once this binary's image is deployed to dev, the existing healthcheck cron exercises it; the dispatch unit tests above plus `make precommit` are sufficient pre-deploy gating. No unit/integration gap remains that would justify a slow E2E.

## Verification

In the binary's directory:

```
make precommit
```

Expected: build, tests, lint, and changelog checks all pass.

Manual post-deploy check (operator, not gated): after the binary is rolled to dev, the next healthcheck cron tick should land a task with status `done` and body `ok` under the maintainer agent's parked assignee, and the failed-run metric counter should not increment for that task.

## Do-Nothing Option

If we skip this work, the maintainer binary continues to fail every healthcheck task its executor dispatches. Probe coverage of this binary stays at zero, and operators must rely on indirect signals (deploy success, log grepping) to confirm the binary is healthy. The failed probe tasks also add noise to the parked-assignee queue. This is not acceptable now that the rest of the fleet (3 agent-repo binaries) reports clean probe results — the maintainer binary becomes the only blind spot. The cost of the change is low: a single binary, one prompt, no schema or API surface change.

## Verification Result

**Verified:** 2026-05-15T14:30:19Z (HEAD 49bb2e6)
**Binary:** installed `dark-factory v0.156.1-1-g04f3863-dirty` (spec target is maintainer agent, not dark-factory)
**Scenario:** Rung-1 precommit + dispatch unit tests in `agent/pr-reviewer/`; rung-2 live dev cluster healthcheck probe through `maintainer-agent-pr-reviewer` dispatch.
**Evidence:**
- `make precommit` in `agent/pr-reviewer/`: `ready to commit` (gosec 0 issues, trivy clean, all tests green)
- factory_test.go:201-221 asserts `provider.Get(TaskTypePRReview/TaskTypeHealthcheck/"bogus")` paths — passed
- `factory.go:179-211` `CreateAgentProvider` wires `agentlib.NewAgentProvider(serviceName, map{TaskTypePRReview: domain, TaskTypeHealthcheck: liveness})`; `main.go:168-182` dispatchAgent calls `provider.Get(ctx, TaskType(a.TaskType))` with `RecordRun(Failed)+RecordDuration` on the dispatch-error path (main.go:124-126)
- `agent/pr-reviewer/go.mod`: `github.com/bborbe/agent/lib v0.62.16` (matches spec)
- Config CR `dev/maintainer-agent-pr-reviewer`: `taskTypes: [pr-review, healthcheck]` (oauth-probe removed per spec line 59)
- Live probe `4f206885-9096-5c19-96f3-9ccc3ed97575` at 2026-05-15T07:05:35Z: `phase: done`, `status: completed`, `task_type: healthcheck` in `~/Documents/Obsidian/OpenClaw/tasks/probe-pr-reviewer-agent-dev.md`; executor log: `job dev/pr-reviewer-agent-4f206885-20260515070535 succeeded ... trusting agent publish` — bug pre-fix would have produced `human_review`/`failed`
- CHANGELOG.md v0.23.37 contains the spec'd bullet `feat(agent/pr-reviewer): per-task-type dispatch via factory.CreateAgentProvider …`
**Verdict:** PASS
