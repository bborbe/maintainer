---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-05-16T10:32:03Z"
generating: "2026-05-16T10:39:02Z"
prompted: "2026-05-16T10:56:51Z"
branch: dark-factory/bug-task-controller-respawns-on-terminal-phase
---

## Summary

- The task-controller spawned a second pr-reviewer-agent pod for the same vault task even though the first pod had already set `phase: human_review`, a terminal phase that means "operator handoff — do not re-run".
- Two pods for task `22fda7e7` ran ~5 minutes apart (09:25:17Z and 09:30:19Z) on 2026-05-16; pod 2 dismissed pod 1's just-posted GitHub review and re-ran the full pipeline against the same SHA.
- Root cause is the spawn predicate in the task-controller: it gates on task `status` but does not gate on `phase`, so any reconcile cycle while `status: in_progress` re-spawns regardless of whether the previous run already escalated to a human.
- Fix: add a terminal-phase gate so `phase ∈ {human_review, done, aborted}` blocks spawning, with an info-level log line on every suppressed spawn for operator visibility.
- This is the last of a 3-bug chain (verdict parser, dismissal filter, this spec); Bug 1 + Bug 2 alone make re-spawn idempotent, Bug 3 prevents the re-spawn from happening at all.

## Problem

The task-controller — whether it lives in `maintainer/` or `bborbe/agent/task/executor/` — currently treats a vault task with `status: in_progress` as "spawn-eligible" regardless of `phase`. When an agent finishes a run and writes a terminal phase like `human_review`, the next reconcile cycle (≤30s later, driven by obsidian-git auto-commit churn) re-reads the same `status: in_progress` and spawns a fresh pod. The terminal phase is the agent's contract for "operator must intervene"; ignoring it both wastes compute and amplifies any non-idempotent behavior in the agent itself. On 2026-05-16 this re-spawn caused pod 2 to dismiss pod 1's correctly-posted GitHub review on `bborbe/maintainer` PR #5 d04d349, hiding evidence of a real hallucination that the human reviewer needed to see.

## Goal

After this fix is deployed, a vault task whose `phase` is one of the project's terminal phases is never spawned again by the controller, regardless of `status`. Operators see an info-level log line whenever a spawn is suppressed for terminal-phase reasons, so a "stuck" task is diagnosable from logs alone. The terminal-phase invariant is encoded in the source code with a comment naming the contract, and is covered by a regression test that fails if the gate is removed.

## Non-goals

(Bug spec — fix scope is bounded by the reproduction. Listed for clarity.)

- Not fixing Bug 1 (verdict-parser silent inversion at `verdict.go:50`) — that is spec 030.
- Not fixing Bug 2 (dismissal-filter inversion at `poster.go:195`) — that is a separate spec.
- Not generalising the gate to other Pattern-B agents (backtest, trade-analysis, claude); those may have their own predicates and are out of scope here.
- Not introducing new terminal phases (`needs_input`, `escalated`, etc.); the implementor scopes the terminal set to phases the project's phase enum already defines today.
- Not migrating ownership between repos. If the predicate lives in `bborbe/agent/`, the fix lands there; the spec name reflects the triggering pr-reviewer task, not the repo location.
- Not changing the agent's own phase-writing behavior (`steps_review.go:117` continues to set `human_review` on hallucination detection); the fix is controller-side only.

## Reproduction

**Triggering incident (verbatim evidence on file):**

- Vault task: `~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-maintainer - 5 - d04d349a - confirm-new-env-vars-are-documented-in-help.md`
- Task UUID: `22fda7e7`
- Pod 1: `pr-reviewer-agent-22fda7e7-20260516092517` (started 09:25:17Z; posted GitHub review `4303450851`; wrote `## Verdict` block; set `phase: human_review` per `agent/pr-reviewer/pkg/steps_review.go:117` hallucination-detection branch)
- Pod 2: `pr-reviewer-agent-22fda7e7-20260516093019` (started 09:30:19Z, ~5 min after pod 1 finished; spawned despite `phase: human_review` already in the task file)
- Both pods visible: `kubectlquant -n prod get jobs | grep 22fda7e7` returns ≥2 rows
- PR: `bborbe/maintainer` #5, head SHA `d04d349a`
- Contributing factor (informational only): an earlier conflict-resolution commit on the task file kept `phase: in_progress` while the dev-side merge had `phase: done`. After resolution, the controller saw `status: in_progress, phase: in_progress` and spawned pod 1 legitimately. Pod 1 then set `phase: human_review`. The bug is that pod 2 followed at all.

**Predicate location (to be confirmed during implementation):**

The implementor must, during the investigation step, locate the spawn predicate and record its file:line in this section before the spec moves to `verifying`. Candidate locations to grep:

- `~/Documents/workspaces/maintainer/agent/` and `~/Documents/workspaces/maintainer/watcher/` for `Spawn`, `CreateJob`, `phase`, `status` (maintainer-side controller, if one exists).
- `~/Documents/workspaces/agent/task/executor/` (or the equivalent path in `bborbe/agent`) for the generic Pattern-B controller that watches vault tasks and spawns agent Jobs.

The expected predicate shape is `shouldSpawn := state.Status == StatusInProgress && !terminalPhases[state.Phase]`. The current predicate is suspected to either omit the phase check entirely or carry the wrong terminal-phase set.

**Minimal in-process reproduction:**

1. Construct a fake task file with `status: in_progress, phase: human_review`.
2. Run the controller's reconcile loop against this file (single tick).
3. Observe today: a spawn is enqueued.
4. Expected: no spawn is enqueued; an info log line names the suppression reason.

**Cross-cycle reproduction (the prod scenario):**

1. Construct a fake task file with `status: in_progress, phase: in_progress`.
2. Run reconcile cycle 1 — observe one spawn.
3. Between cycles, mutate the file to `status: in_progress, phase: human_review` (simulating the agent's write and obsidian-git auto-commit).
4. Run reconcile cycle 2 — observe zero new spawns.

## Expected vs Actual

**Expected** (per the controller's documented Pattern-B contract and the agent's phase semantics): a vault task with `phase ∈ {human_review, done, aborted}` is treated as terminal and is not eligible for spawning until a human operator resets it. Reading the task file is the source of truth on every reconcile cycle; no stale phase is cached across cycles.

**Actual** (observed 2026-05-16 on task `22fda7e7`): the controller spawned pod 2 at 09:30:19Z while the task file's `phase` was `human_review` (written by pod 1 at ~09:29Z). Two pods ran against the same task, against the same PR SHA, within a 5-minute window. The second pod's run dismissed the first pod's GitHub review and replaced it with a different verdict, hiding the hallucination signal pod 1 had captured.

## Why this is a bug

Three invariants are broken:

1. **Terminal-phase contract.** `human_review`, `done`, and `aborted` are documented as terminal in the agent phase enum — they mean "no more automated work on this task". The controller violating this is a direct contract breach.
2. **Idempotency.** Even if `human_review` were not strictly terminal, spawning twice in 5 minutes is non-idempotent: pod 2's run produces side effects (GitHub API calls, review dismissals, audit-log entries) that pod 1 already produced. Any agent with non-pure side effects is exposed to amplification.
3. **Operator diagnosability.** When the controller silently re-spawns, an operator inspecting "why did the agent run twice" has no log evidence of the decision. The current code path is invisible.

The combination is that the controller is both wrong (1) and silent (3), with consequences amplified by any downstream non-idempotency (2). This is the precondition that turned Bug 1 + Bug 2 into a visible incident; fixing it closes the trigger.

## Desired Behavior

1. The controller's spawn predicate returns `false` whenever the task file's `phase` is one of `{human_review, done, aborted}`, regardless of `status` or any other field.
2. The terminal-phase set is encoded as a single source of truth in the controller code — either as an `IsTerminal()` method on the existing phase type (preferred, per coding guideline `go-enum-type-pattern.md`) or as a package-level `var terminalPhases = map[Phase]bool{...}` with the same set.
3. Phase is re-read from the task file on every reconcile cycle; no in-memory cache of phase persists across cycles. If a task's phase changes between cycles, the next cycle sees the new value.
4. Every suppressed spawn emits an info-level log line of the form `controller: spawn suppressed phase=<phase> task=<task-id>`. The log line is emitted exactly once per reconcile cycle per suppressed task (not on every read inside a cycle).
5. The existing happy-path behavior — `status: in_progress, phase: in_progress` spawns exactly one pod — is preserved unchanged.
6. The source contains a code comment at the gate naming the invariant: "terminal phases must not be spawned again — operator escalation required".

## Constraints

- The phase enum's existing constants MUST NOT change. The fix may add an `IsTerminal()` method on the existing type but must not rename or remove `PhaseInProgress`, `PhaseHumanReview`, `PhaseDone`, `PhaseAborted`, etc.
- The controller's reconcile interval and existing log keys for spawn events MUST NOT change. The new suppression log line is additive.
- Existing controller tests MUST continue to pass; new tests are additive.
- The fix must work whether the predicate lives in `maintainer/` or `bborbe/agent/`. The implementor investigates and lands the change in whichever repo owns the predicate; spec verification follows the owning repo's `make precommit` and deployment path.
- Verification ladder per `docs/verifying-specs.md`: Rung-1 (local unit + table test green) is the primary correctness gate; Rung-2 (dev k8s deploy + multi-cycle reconcile observation against a real task whose phase flips mid-cycle) is the only way to prove cross-cycle idempotency under obsidian-git auto-commit churn; Rung-3 (prod) only after Rung-2 passes.
- Domain reference: `bborbe/agent` task-controller architecture (Pattern-B Job consumer model); see vault note `[[Agent Task Controller Architecture]]` (Personal vault).

## Failure Modes

| Trigger | Expected behavior | Detection | Reversibility | Concurrency | Recovery |
|---|---|---|---|---|---|
| Task file has `phase: human_review` mid-reconcile | No spawn; info log line names suppression | `kubectlquant -n <stage> logs <controller-pod> \| grep 'spawn suppressed'` shows the task id | n/a (no side effects produced) | Multiple controllers — each independently re-reads the file and reaches the same suppression decision | Operator edits the task file to reset `phase: in_progress` if re-run is desired |
| Task file `phase` flips `in_progress` → `human_review` between cycles | Cycle 1 spawns, cycle 2 suppresses | Cycle 1 log shows spawn, cycle 2 log shows suppression for same task id | n/a | Mid-action crash between cycles is safe because phase is re-read from disk every cycle | n/a |
| obsidian-git auto-commit lands while controller is mid-read | Controller re-reads on next reconcile tick (≤30s); decision uses the latest committed phase | File mtime advances; next cycle log reflects new phase | n/a | Two reconcile cycles racing on the same file: file read is atomic at the OS level; both see a coherent snapshot | n/a |
| Task file unreadable (parse error, git conflict markers, missing frontmatter) | No spawn; existing parse-error path is taken; suppression log is NOT emitted (the spawn was blocked for a different reason) | Existing controller error log | n/a | n/a | Operator fixes the file by hand |
| Phase enum gains a new value the controller does not know about | Conservative default: unknown phase is treated as NON-terminal (preserves existing spawn behaviour) and an info log warns about the unknown phase | Controller log line `unknown phase=<value> task=<id>` | Reversible by extending the terminal set | Multiple controllers behave identically | Operator files a follow-up spec to classify the new phase |
| Two controllers running (rolling deploy overlap) | Both independently suppress on terminal phase; idempotent | Each pod's log shows the same suppression decision | n/a | Job creation in k8s is name-collision-safe; suppressing on both is the safe default | n/a |
| Clock skew on the controller pod | No impact — the gate is purely on file content, not timestamps | n/a | n/a | n/a | n/a |

## Security / Abuse Cases

(Touches no HTTP, no user input. Operator-written task frontmatter is the only external input; the controller already parses it via the existing path. The fix adds one more value comparison and one log line, neither of which crosses a trust boundary.)

- Attacker control: an operator with write access to the vault could set `phase: human_review` to suppress legitimate spawns. This is acceptable — operators already have full control over task state by design.
- No new code path can hang or retry: the gate is a single map/method lookup.

## Acceptance Criteria

- [ ] The spawn predicate's location is recorded in the `## Reproduction` section of this spec by the implementor before the spec moves to `verifying` — evidence: this file contains a line of the form `Predicate location: <repo>/<path>:<line>` under "Predicate location (to be confirmed during implementation)".
- [ ] The terminal-phase set is encoded in a single named symbol in the controller code — evidence: `grep -nE 'IsTerminal|terminalPhases' <controller-path>` returns ≥1 line in the implementation file and ≥1 line in the test file; the set contains exactly `human_review`, `done`, `aborted` (or the project's existing constants for these).
- [ ] The spawn predicate calls the terminal-phase check — evidence: reading the predicate function, the body contains a call to `IsTerminal()` or a lookup in `terminalPhases` before returning `true`.
- [ ] A code comment at the gate names the invariant — evidence: `grep -n 'terminal phases must not be spawned' <controller-path>` returns ≥1 line.
- [ ] Ginkgo regression tests cover all 6 rows from the bug report — evidence: `go test -run <controller-test-pattern> -v` output contains the table-test row names: `status=in_progress phase=in_progress => spawn`, `status=in_progress phase=human_review => no spawn`, `status=in_progress phase=done => no spawn`, `status=in_progress phase=aborted => no spawn`, `status=completed phase=* => no spawn`, `sequential reconcile in_progress->human_review => exactly 1 spawn total`.
- [ ] The cross-cycle test asserts exactly one spawn — evidence: the sequential-reconcile test counts spawn invocations via a mock/spy and asserts the counter equals 1 after two reconcile cycles; revert-test confirms that removing the gate makes the counter equal 2.
- [ ] Revert-test proves the gate is load-bearing — evidence: on a branch with the gate removed (`IsTerminal()` call deleted or `terminalPhases` lookup short-circuited), `go test ./...` exits non-zero and the failure cites the `phase=human_review` or sequential-reconcile row.
- [ ] Info-level log line on every suppressed spawn — evidence: a unit test captures the controller's log output and asserts the line `controller: spawn suppressed phase=human_review task=<id>` (or the project's structured-log equivalent with keys `event=spawn_suppressed phase=human_review task=<id>`) is emitted exactly once for a suppressed reconcile cycle.
- [ ] Log line is NOT emitted on the parse-error path — evidence: a unit test feeds the controller a task file with corrupted frontmatter (e.g. unresolved git conflict markers in the YAML block), asserts `spawn_suppressed` is NOT in the captured log output (the existing parse-error log path runs instead), and the spawn-counter remains 0. This guards Failure Modes row 4: parse errors must not masquerade as terminal-phase suppression.
- [ ] `make precommit` exits 0 in the repo that owns the predicate — evidence: exit code in the owning repo (`~/Documents/workspaces/maintainer/` or `~/Documents/workspaces/agent/<controller-path>/`).
- [ ] Existing controller tests still pass — evidence: no test in the owning repo regresses; `git diff` on test files shows additions only or test-renames only, not assertion changes that weaken existing behaviour.
- [ ] Live replay (Rung-2): after dev deploy, resetting the PR #5 task to `phase: in_progress` causes exactly one pod to spawn and stay terminal — evidence: `kubectlquant -n dev get jobs | grep <task-uuid> | wc -l` returns `1` after a ≥10-minute observation window; `kubectlquant -n dev logs <controller-pod> --since=15m | grep 'spawn suppressed'` shows ≥1 line referencing the task id after pod 1 has set `phase: human_review` or `phase: done`.
- [ ] Live replay (Rung-3, prod): the same observation as Rung-2 against prod after ≥1 day of dev soak — evidence: within 7 days of the dev-soak window starting, the next pr-reviewer task that writes a terminal phase (`human_review`, `done`, or `aborted`) in prod has `kubectlquant -n prod get jobs | grep <task-uuid> | wc -l` return `1` exactly. If no terminal-phase task arrives in prod within 7 days, the verifier may force one by re-resetting PR #5 once dev soak is green.

(Scenario coverage: none. The unit + integration tests in the owning repo cover the predicate; the Rung-2 live replay covers the cross-cycle obsidian-git churn behavior. No new dark-factory scenario is warranted.)

## Verification

```
# Step 1 — Rung 1, in the owning repo:
cd <owning-repo>/<controller-path> && make precommit
```

Expected: exit 0. The new Ginkgo tests report all 6 rows passing.

```
# Step 2 — Rung 2, dev cluster e2e:
# After dev deploy, reset the vault task for PR #5 d04d349 so the controller
# re-spawns the agent. Operator edits the frontmatter of:
#   ~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-maintainer - 5 - d04d349a - confirm-new-env-vars-are-documented-in-help.md
# Set: phase=in_progress, status=in_progress; drop current_job + job_started_at;
# bump trigger_count. obsidian-git auto-commits; the dev task-controller picks
# it up on the next reconcile (≤30s).
#
# Wait until the spawned pod completes and writes a terminal phase
# (human_review or done). Then wait ≥10 minutes and verify no second pod:

kubectlquant -n dev get jobs | grep <task-uuid> | wc -l
# Expected: 1

kubectlquant -n dev logs <controller-pod> --since=15m | grep 'spawn suppressed'
# Expected: ≥1 line referencing the task id, emitted after the pod's terminal phase write
```

```
# Step 3 — Rung 3, prod (only after ≥1 day of dev soak):
kubectlquant -n prod get jobs | grep <next-task-that-escalates> | wc -l
# Expected: 1
```

## Do-Nothing Option

Not viable. The current controller will re-spawn any pr-reviewer task that lands in `human_review`, repeatedly, every reconcile cycle, until an operator manually changes `status` to `completed` or `aborted`. Each re-spawn (a) wastes compute, (b) dismisses the previous run's GitHub review via Bug 2, (c) re-runs the agent against the same SHA producing a fresh (possibly different) verdict, and (d) hides the operator-escalation signal that `human_review` was supposed to surface. The PR #5 d04d349 incident is the canonical evidence: the first run correctly detected a hallucination and escalated; the second run was spawned by this bug, dismissed the escalation, and amplified the failure. Bug 1 + Bug 2 fixes alone make the second spawn harmless but the second spawn still wastes compute and confuses operators reading the job list. Bug 3 is the only fix that prevents the trigger.
