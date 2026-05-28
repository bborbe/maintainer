---
status: verifying
approved: "2026-05-28T18:44:37Z"
generating: "2026-05-28T20:57:16Z"
prompted: "2026-05-28T20:57:16Z"
branch: dark-factory/bug-github-releaser-escalation-wrong-status
---

## Summary

- `agent/github-releaser` planning-phase escalation returns `agentlib.Result{Status: Done, NextPhase: ""}`, which the agent framework's result deliverer interprets as "task complete" — auto-mutating `status: in_progress → completed` and `phase: planning → done`.
- This breaks the escalation rule documented in [[Agent Task File Contract]] § Escalation rule + spec 027 (`previous_assignee`): "Clear assignee, leave status and phase alone." The resume cursor is lost; operator re-delegation cannot resume planning.
- Root cause: spec 047 line 48 instructs the step to return `Status: Done` on `needs_input`. The framework auto-advances to terminal state for `Done` with empty `NextPhase`. Correct status for escalation is `AgentStatusNeedsInput`.
- Fix: change `steps_planning.go` to return `Status: AgentStatusNeedsInput` (not `Done`) at all three escalation sites (missing frontmatter, P1 fail, P2 fail). The frame keeps `status: in_progress` + `phase: planning` automatically when the step says NeedsInput.

## Problem

End-to-end smoke against a real bborbe repo (see Reproduction) revealed that escalated tasks land with `status: completed` + `phase: done`. That's identical to the terminal-success state — operator inbox queries cannot distinguish escalation from completion, and re-delegation by setting `assignee: <agent>` would re-trigger an already-terminal task.

The bug only surfaces with a real deliverer (file / Kafka). The existing unit test in `pkg/steps_planning_test.go` uses a mocked deliverer that doesn't apply framework-side mutations, so the test passed against the buggy behavior.

## Reproduction

```bash
cd ~/Documents/workspaces/maintainer-github-releaser

# Build a fixture pointing at a real repo with NO ## Unreleased section.
cat > /tmp/repro.md <<'EOF'
---
status: in_progress
phase: planning
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/docker-utils
clone_url: https://github.com/bborbe/docker-utils.git
ref: master
current_version: v1.7.9
task_identifier: gh-release-bborbe-docker-utils-master-repro
---
# Reproduction fixture
EOF

cd agent/github-releaser
go run ./cmd/run-task -task-file /tmp/repro.md -gh-token "$(gh auth token)"
cat /tmp/repro.md | head -12
```

### Observed output

stdout JSON envelope:

```json
{"Status":"done","NextPhase":"","Message":"Unreleased section not found.","ContinueToNext":false}
```

Mutated `/tmp/repro.md` frontmatter:

```yaml
---
assignee: ""
clone_url: https://github.com/bborbe/docker-utils.git
current_version: v1.7.9
phase: done                                  # ← BUG: should stay "planning"
previous_assignee: github-releaser-agent
ref: master
repo: bborbe/docker-utils
status: completed                            # ← BUG: should stay "in_progress"
task_identifier: gh-release-bborbe-docker-utils-master-repro
task_type: github-release
---
```

Verified on commit `332dc48` (HEAD of `feat/github-releaser`) on 2026-05-28.

## Expected vs Actual

| Field | Expected (per [[Agent Task File Contract]] § Escalation rule) | Actual |
|---|---|---|
| `assignee` | `""` (cleared) | `""` ✓ |
| `previous_assignee` | `github-releaser-agent` (set) | `github-releaser-agent` ✓ |
| `status` | `in_progress` (unchanged) | `completed` ✗ |
| `phase` | `planning` (unchanged — resume cursor) | `done` ✗ |
| `## Plan` JSON `outcome` | `needs_input` | `needs_input` ✓ |
| `## Plan` JSON `precondition_failed` | `P2_unreleased_empty` | `P2_unreleased_empty` ✓ |

The `## Plan` section content is correct. Only the frontmatter mutation is wrong.

## Goal

Escalated planning runs leave `status` and `phase` untouched. After fix, repro produces:

```yaml
assignee: ""
previous_assignee: github-releaser-agent
status: in_progress       # unchanged
phase: planning           # unchanged
```

Plus, an integration test using the real `agentlib` deliverer path (not a mock) catches any future regression of this behavior.

## Non-goals

- Spec 047 amendment for the documentation error (the spec said `Status: Done` is correct on escalation; it isn't). Fix in code; mark spec 047 verified after this bug closes. Leave the spec text as-is for historical record — the lesson lives in this bugfix spec.
- The cosmetic double-prefix in error messages (`"fetch CHANGELOG.md: fetch CHANGELOG.md:"`) — separate bug; not in scope.
- Execution / ai_review phase escalations — same pattern will apply there, but those phases ship in separate specs.

## Desired Behavior

1. `pkg/steps_planning.go` returns `agentlib.Result{Status: agentlib.AgentStatusNeedsInput, Message: <reason>, NextPhase: ""}` at all three escalation sites: missing-frontmatter, P1 fail, P2 fail. (Where `AgentStatusNeedsInput` is the typed constant from `github.com/bborbe/agent/lib`.)
2. The `## Plan` section content (the JSON body) is unchanged — still written via `MarshalSectionTyped` with the `PlanOutput` shape from spec 047.
3. `assignee: ""` and `previous_assignee: github-releaser-agent` mutations on the frontmatter are unchanged.
4. A new Ginkgo test in `pkg/steps_planning_test.go` (or new `pkg/steps_planning_integration_test.go`) exercises a full `agentlib` run with the real `delivery.NewNoopResultDeliverer()` (or the file deliverer with a temp file) and asserts the mutated content's frontmatter has `phase: planning` AND `status: in_progress` (verbatim from the input fixture) — proving the framework no longer auto-advances.
5. The existing escalation unit tests are updated to assert `result.Status == agentlib.AgentStatusNeedsInput` (not `Done`) — these caught the right intent but the wrong status enum.

## Constraints

- Error wrapping via `github.com/bborbe/errors` unchanged.
- `## Plan` JSON contract unchanged (downstream execution-phase spec depends on it).
- No new public API in `pkg/steps_planning.go` — the fix is internal.
- Integration test uses a real `agentlib` deliverer (the bug only surfaces past the mock layer). Use `delivery.NewNoopResultDeliverer()` for simplicity; the deliverer's "ApplyToTaskContent" path is what mutates frontmatter, and noop preserves it for assertion.
- Integration test runs OFFLINE — no live GitHub network calls. The Fetcher must be a counterfeiter mock (`mocks.FakeFetcher`) returning a pre-baked CHANGELOG byte slice. The reproduction in `## Verification` uses `gh auth token` for end-to-end re-validation only; that's a one-shot smoke, NOT the AC walk.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| Fix accidentally swaps Done → NeedsInput on HAPPY path too | Happy-path test (mock Claude returning patch/minor/major) fails: `Status: NeedsInput` instead of `Done` | Test fails; PR rejected | Re-read this spec; only escalation sites change |
| `agentlib.AgentStatusNeedsInput` constant not exported in current `agent/lib` version | Compile error in `steps_planning.go` | Build fails fast | Verify `~/Documents/workspaces/go/pkg/mod/github.com/bborbe/agent/lib@v0.63.x/agent_step.go` exports the constant; bump `agent/lib` if needed |
| Real deliverer (Kafka path) has different mutation logic | Smoke against dev cluster shows phase/status still mutated | Reopen this bug; document divergence between file and Kafka deliverers | n/a — fix at framework layer |

## Workaround

Until fix lands, operator re-delegating an escalated task must also manually reset `status: in_progress` and `phase: planning` before setting `assignee:`. Tedious; the fix removes the friction.

## Acceptance Criteria

- [ ] `cd agent/github-releaser && make precommit` exits 0.
- [ ] `grep -c 'AgentStatusNeedsInput' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1 — the three escalation triggers (missing-frontmatter, P1 fail, P2 fail) funnel through a single `escalate` method with one return statement, so the canonical count is 1. Inlining 3 separate returns would violate DRY for no behavior gain.
- [ ] `grep -c 'AgentStatusDone' agent/github-releaser/pkg/steps_planning.go` returns ≥ 1 (the happy path still uses Done). Defense-in-depth against the blanket-swap regression: `grep -B 20 'AgentStatusDone' agent/github-releaser/pkg/steps_planning.go | grep -c 'previous_assignee'` returns 0 (no `AgentStatusDone` return within 20 lines of `previous_assignee` mutation).
- [ ] Existing unit tests for escalation cases in `pkg/steps_planning_test.go` updated to assert `result.Status == agentlib.AgentStatusNeedsInput`.
- [ ] **New integration test** exists in `pkg/steps_planning_test.go` (or new file) that builds the full agent via `factory.CreateAgent`, runs `agent.Run` against an in-memory escalation fixture (P1 or P2), and asserts the resulting content's frontmatter has `phase: planning` AND `status: in_progress` unchanged. Evidence: `grep -c 'status: in_progress' pkg/steps_planning_test.go` returns ≥ 1 inside the integration block; `grep -c 'phase: planning' pkg/steps_planning_test.go` returns ≥ 1 in the same block.
- [ ] Reproduction (Reproduction section above) re-run produces `phase: planning` and `status: in_progress` in the mutated file — evidence: `grep -c '^phase: planning' /tmp/repro.md` returns 1; `grep -c '^status: in_progress' /tmp/repro.md` returns 1.
- [ ] Root `CHANGELOG.md` `## Unreleased` gains a `fix:` bullet referencing the escalation status — evidence: `grep -c 'fix.*escalation' CHANGELOG.md` returns ≥ 1.

## Verification

```bash
cd ~/Documents/workspaces/maintainer-github-releaser/agent/github-releaser
make precommit                                                    # exit 0

grep -c 'AgentStatusNeedsInput' pkg/steps_planning.go            # ≥1 (single escalate funnel)
grep -c 'AgentStatusDone'       pkg/steps_planning.go            # ≥1 (happy path)

# Re-run the reproduction from the Reproduction section
cat > /tmp/repro.md <<'EOF'
---
status: in_progress
phase: planning
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/docker-utils
clone_url: https://github.com/bborbe/docker-utils.git
ref: master
current_version: v1.7.9
task_identifier: gh-release-bborbe-docker-utils-master-repro
---
# Repro
EOF
go run ./cmd/run-task -task-file /tmp/repro.md -gh-token "$(gh auth token)"
grep -c '^phase: planning'   /tmp/repro.md   # =1
grep -c '^status: in_progress' /tmp/repro.md # =1
grep -c '^assignee: ""'      /tmp/repro.md   # =1
grep -c 'previous_assignee: github-releaser-agent' /tmp/repro.md  # =1

# Root CHANGELOG entry
grep -c 'fix.*escalation' CHANGELOG.md   # ≥1
```

No scenario justified — the reproduction itself IS the integration evidence at the right layer. Per [[spec-writing]] § Test-layer responsibilities, the four-condition test fails on every count.

## Verification Result

**Verified:** 2026-05-28T20:59:52Z (HEAD 5312ed7)
**Binary:** /Users/bborbe/Documents/workspaces/go/bin/dark-factory (v0.173.0)
**Scenario:** Rung-1 smoke: re-ran the Reproduction section's `go run ./cmd/run-task` against real `bborbe/docker-utils` master via `gh auth token`; inspected mutated `/tmp/repro.md` frontmatter. Rungs 2/3 N/A (CRD deployment out of scope per spec 047 Non-goals).
**Evidence:**
- `grep -c 'AgentStatusNeedsInput' pkg/steps_planning.go` = 2 (AC2 ≥1)
- `grep -c 'AgentStatusDone' pkg/steps_planning.go` = 1 (AC3a ≥1); `grep -B 20 'AgentStatusDone' ... | grep -c 'previous_assignee'` = 0 (AC3b)
- `pkg/steps_planning_test.go` asserts `Equal(agentlib.AgentStatusNeedsInput)` at lines 89/133/203/230/369 (AC4)
- `grep -c 'status: in_progress' pkg/steps_planning_test.go` = 12; `grep -c 'phase: planning' pkg/steps_planning_test.go` = 11 (AC5)
- `/tmp/repro.md` after fresh `cmd/run-task` run: `phase: planning` =1, `status: in_progress` =1, `previous_assignee: github-releaser-agent` =1 (AC6)
- `grep -c 'fix.*escalation' CHANGELOG.md` = 1 (AC7)
- `cd agent/github-releaser && make precommit` → `ready to commit` exit 0 (AC1)
**Verdict:** PASS
