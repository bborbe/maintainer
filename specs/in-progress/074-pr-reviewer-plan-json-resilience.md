---
status: verifying
approved: "2026-06-26T12:12:56Z"
generating: "2026-06-26T12:25:48Z"
prompted: "2026-06-26T12:31:29Z"
verifying: "2026-06-26T12:41:07Z"
branch: dark-factory/pr-reviewer-plan-json-resilience
---

## Summary

- pr-reviewer's planning step today writes Claude's raw output to `## Plan` without validating the JSON; when the output contains unescaped quotes inside `concerns[].note`, every subsequent retry re-parses the same garbage and routes the task to `human_review` — a dead-end no human can fix without editing the agent.
- Add **two complementary defenses**: (a) instruct Claude in the planning output-format prompt to escape inner double quotes in string values (especially code snippets like `name != ""`); (b) parse-validate `runResult.Result` before persisting to `## Plan` — if parsing fails, do NOT write the malformed body; return a failed Result so the next retrigger gets a fresh planning attempt instead of replaying the broken Plan.
- Together these eliminate the malformed-JSON → human_review dead-end class. The failure surface goes from "permanent stuck task no human can recover" to "transient planning failure that re-runs automatically on retrigger".
- No change to the routing decision when `## Plan` parses cleanly (empty concerns → LGTM → done; non-empty → execution phase). No change to execution / ai_review / verdict logic.

## Problem

Observed live 2026-06-26 on `bborbe/vault-cli#27`:

```
planning: parse failed nextPhase=human_review 
err=parse ## Plan JSON: invalid character '"' after object key:value pair
```

Root cause in the persisted `## Plan` body:

```json
"note": "Arg order matters: -n must appear after --print and before -p. The new args construction (append -n after --print when name != "" then append -p/-oformat) looks correct ..."
                                                                                                      ^^                                          ^^
                                                                                                      unescaped double quotes break JSON
```

Claude's planning output embedded `name != ""` as literal source-code-style text inside a JSON string. The bare `""` closes the JSON string early; the parser fails on the next character.

`steps_planning.go:108-114` writes `runResult.Result` to `## Plan` directly. The idempotency path at `:84-87` then re-uses the persisted body on every retrigger. `routeFromPlan` at `:128-134` correctly catches the parse failure but routes to `human_review` — a phase no human can productively action (the markdown is correct from a human's POV; only the JSON parser is unhappy). Three triggers on vault-cli#27 all hit this same dead-end.

The runbook recovery (reset `phase: human_review → planning`, `assignee: "" → pr-reviewer-agent`) does not help: the controller's vault scanner re-emits the task, the planning step finds the stale `## Plan`, fails to parse again, routes to `human_review` again. The only paths out are admin-merge or hand-editing the task file.

## Goal

After this work, both the generation and parse-recovery sides of the pipeline tolerate the quotes-in-source-code class of failure:

1. The planning output-format prompt explicitly tells Claude to escape inner double quotes in string values (especially code snippets), so the generator produces parseable JSON in the first place.
2. The planning step validates `runResult.Result` parses as `planningOutput` JSON before persisting it. If parse fails, do not write the bad body — return `AgentStatusFailed` so the controller's normal retry kicks in.
3. On retrigger, the idempotency path (`if section, exists := md.FindSection("## Plan"); exists`) is unchanged for valid Plans. If no valid Plan was persisted on the prior run (because validate-and-skip kicked in), the retrigger sees no `## Plan` and re-runs the Claude planning call from scratch.

The net behavioral change: the malformed-JSON → human_review terminal state goes away. Failures become transient (retried automatically) instead of permanent (stuck pending manual recovery).

## Non-goals

- No change to the routing decisions when `## Plan` parses cleanly: empty `concerns` → LGTM → done; non-empty → execution phase. All downstream phases (execution, ai_review, verdict posting) are unchanged.
- No JSON-repair / quote-auto-escape post-processing. The fix is parse-or-discard, not parse-or-repair. Auto-repair is a deep rabbit-hole (which quote is the boundary? which is the inner literal?) — for the cost, just retry the LLM call.
- No schema migration from JSON to YAML (mentioned as a candidate but a bigger surface — kept for a follow-up spec if this fix doesn't fully close the class).
- No change to `## Review` / `## Verdict` persistence paths. The vault-first invariant (`steps_review.go`) stays as-is — only `## Plan` gets the validate-before-write guard.
- No change to the controller-side retry / backoffLimit configuration. The fix is agent-side: stop writing garbage; let the existing retry mechanism do its job.
- No prompt redesign for the substance of what Claude reviews (the focus areas, concern format, decision logic). The only prompt change is the explicit "escape inner quotes" instruction.

## Acceptance Criteria

- [ ] `planning_output-format.md` contains an explicit instruction telling the LLM to escape inner double quotes in string values, with a code-snippet example. Evidence: `grep -F 'name != \"\"' agent/pr-reviewer/pkg/prompts/planning_output-format.md` returns ≥1 line (proves the load-bearing example landed) AND `grep -niE 'escape.*quote|backslash.*quote|\\\\\\\\"' agent/pr-reviewer/pkg/prompts/planning_output-format.md` returns ≥1 line.
- [ ] `agent/pr-reviewer/pkg/steps_planning.go::planningStep.Run` calls `parsePlanningConcerns(ctx, runResult.Result)` before `md.ReplaceSection("## Plan", ...)`. If parsing fails, the step returns `Status: AgentStatusFailed` with a message containing `planning: malformed JSON`, and the `## Plan` section is NOT written. Evidence: `go test ./agent/pr-reviewer/pkg/ -run TestPlanningRejectsMalformedJSON -v` reports `PASS`.
- [ ] When parsing succeeds, the step writes `## Plan` and routes exactly as today (empty concerns → LGTM, non-empty → execution). Evidence: existing tests in `pkg/steps_planning_test.go` continue to pass: `go test ./agent/pr-reviewer/pkg/ -run TestPlanning -v` reports `PASS` for the pre-existing scenarios.
- [ ] A regression test reproduces the live failure: a `## Plan` body containing the literal substring `name != ""` (unescaped) is rejected at write-time with `AgentStatusFailed`. Evidence: `go test ./agent/pr-reviewer/pkg/ -run TestPlanningRejectsLiveSample -v` reports `PASS`.
- [ ] The retrigger-with-existing-`## Plan` idempotency path (line 84-87 of `steps_planning.go`) is unchanged. On retrigger with a valid persisted Plan, no Claude call is made and routing is identical to today. Evidence: `go test ./agent/pr-reviewer/pkg/ -run TestPlanningRereadsExistingPlan -v` reports `PASS`.
- [ ] `make precommit` exits 0 (vet + lint + full test suite + race detector).
- [ ] `CHANGELOG.md` has a new bullet under `## Unreleased` describing the change. Evidence: `awk '/^## Unreleased/,/^## v/' CHANGELOG.md | grep -niE 'pr-reviewer.*plan.*json|planning.*malformed' | head -1` returns ≥1 line.

## Verification

```
make precommit
```

## Desired Behavior

1. `agent/pr-reviewer/pkg/prompts/planning_output-format.md` gains a new "JSON safety" rule near the schema, with explicit examples for the common offender: code snippets that contain double quotes. Sample wording (exact phrasing the prompt-writer agent will pick): "Inside JSON string values, double quotes MUST be escaped as `\\\"`. Code snippets containing quotes (e.g. `name != \"\"`) must be written as `name != \\\"\\\"`. Single quotes and backticks do NOT need escaping. If unsure, prefer rephrasing without quotes."
2. `agent/pr-reviewer/pkg/steps_planning.go::planningStep.Run` is restructured so that — when Claude returns a fresh result — the body is parsed via `parsePlanningConcerns` BEFORE `md.ReplaceSection("## Plan", ...)`. On parse success, the existing flow runs unchanged (write Plan, call `routeFromPlan`). On parse failure, return `&agentlib.Result{Status: AgentStatusFailed, Message: fmt.Sprintf("planning: malformed JSON: %v", parseErr)}` and do NOT mutate the markdown.
3. The existing `if section, exists := md.FindSection("## Plan"); exists` idempotency path (line 84-87) is unchanged. On retrigger with a persisted Plan, the code re-uses the stored body and re-runs `routeFromPlan` — exactly as today.
4. When the validate-and-skip path returns `AgentStatusFailed`, the controller's standard retry policy kicks in: the task stays in `phase: planning` with no `## Plan` section, the controller spawns a fresh Job (subject to `backoffLimit`), and the next Job's planning step re-runs Claude from scratch.
5. The malformed-JSON-on-retrigger code path at `:128-134` (re-parse fails inside `routeFromPlan`) stays as defense-in-depth for any historical task files that have a stale malformed `## Plan` from before this fix shipped. Operator runbook recovery (delete the `## Plan` section, reset phase) still works for those cases.

## Constraints

- Must not change the public `planningStep` type / constructor signature.
- Must not change the routing decisions for parseable Plans (empty concerns → LGTM → done; non-empty → execution).
- Must not modify `steps_review.go`, `steps_checkout_execution.go`, `steps_gh_token.go`, or any other step beyond planning.
- Must not introduce JSON-repair / auto-escape post-processing — the fix is parse-or-discard, not parse-or-repair.
- Must not change the schema described in `planning_output-format.md` beyond adding the "JSON safety" guidance. Field names (`concerns`, `area`, `file`, `note`) are unchanged.
- Must not change the controller-side `backoffLimit`, `activeDeadlineSeconds`, or k8s Job spec.
- Must preserve the vault-first invariant for `## Review` and `## Verdict` writes — only `## Plan` gets the validate-before-write guard.
- Must not regress any existing test in the suite.
- Must keep the `## Plan already present — re-routing without claude` idempotency intact (cost-saver on retriggers).

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| Claude returns unparseable JSON (the live bug) | Step returns `AgentStatusFailed`; `## Plan` not written; controller schedules a retry per `backoffLimit`. Subsequent Job spawns a fresh Claude call. | Automatic — controller's existing retry handles it. |
| Claude returns parseable JSON but with semantically empty `concerns: []` array on a PR that genuinely needs review | Unchanged from today — empty concerns route to LGTM. Spec acknowledges this is a pre-existing class (false-clean review) and is out of scope. | None — covered by `[[PR Reviewer Agent Guide#Verdict not valid JSON]]`. |
| Stale malformed `## Plan` from a pre-fix run lands in a task file | Defense-in-depth at `routeFromPlan:128-134` catches it and routes to `human_review` (unchanged behavior). Operator removes the `## Plan` section via vault-cli + retriggers. | Manual one-time recovery per runbook; future runs use the new write-time validation. |
| Claude's escaping instruction is ignored (LLM bug) | Same as today's behavior pre-fix: parse fails → step returns `AgentStatusFailed` → retry. The prompt fix is a probability reduction, not a guarantee; the write-time validate is the load-bearing safety. | Automatic — retry kicks in. |
| Network / timeout on Claude call | Already handled — `runErr != nil` branch at `:100-105` returns `AgentStatusFailed`. Unchanged. | Automatic. |
| Two parallel Jobs race to write `## Plan` (defensive) | Already handled by the controller's job-level mutual exclusion; both Jobs are for the same task and at most one runs at a time. Not applicable. | None. |

## Do-Nothing Option

Leaving the bug in place: every time Claude emits a JSON-poisoning quote inside a `concerns[].note`, the task is permanently parked in `human_review` until an operator either hand-edits the `## Plan` markdown to escape quotes (no public runbook for this) or admin-merges the underlying PR. Both options bypass the agent's review entirely — the bot's whole purpose is defeated for that PR.

This failure class has hit at least one production PR (vault-cli#27 on 2026-06-26, three triggers consumed) and is structurally guaranteed to recur whenever Claude's `## Plan` output references code with literal double-quote pairs (very common for Go zero-string-checks: `name != ""`, `if s == ""`, etc.). The probability scales with PR count.

The fix is single-package (pkg/), single-step (planningStep), no behavior change for the happy path. Cost is one prompt edit + one parse-before-write reorder + 4 tests + a CHANGELOG bullet. Sub-30-minute container time.
