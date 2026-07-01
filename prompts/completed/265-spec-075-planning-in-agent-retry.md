---
status: completed
spec: [075-pr-reviewer-planning-in-agent-retry]
summary: Added 3-attempt retry loop to Claude planning call in pr-reviewer agent with full test coverage (87.3%)
execution_id: maintainer-plan-retry-exec-265-spec-075-planning-in-agent-retry
dark-factory-version: dev
created: "2026-07-01T00:00:00Z"
queued: "2026-07-01T09:26:41Z"
started: "2026-07-01T09:26:42Z"
completed: "2026-07-01T09:29:41Z"
branch: dark-factory/spec-075
---

<summary>
- The pr-reviewer planning step will retry the Claude call when it returns malformed JSON, up to 3 attempts total.
- Intermittent bad output (the observed MiniMax "B"-instead-of-`{` case) is transparently recovered without any operator action.
- A review that used to fail on the first bad response and get stuck in REVIEW_REQUIRED now self-corrects in 1-2 attempts most of the time.
- The step still fails (same failure surface as today) only after all 3 attempts return unparseable JSON — but now with a message that says why.
- Claude transport errors (nil result + error) are NOT retried — they fail immediately, exactly as before.
- When a plan already exists from a prior run, the Claude runner is still not called at all (idempotent re-entry unchanged).
- Each malformed-but-not-exhausted attempt logs a grep-able line so operators can see real recoveries in the agent logs.
- No new config, env var, flag, or backoff — the attempt cap is a hardcoded constant of 3.
</summary>

<objective>
Add an in-agent retry loop around the Claude planning call in the pr-reviewer agent so intermittent malformed-JSON responses self-correct: retry up to 3 attempts total on JSON parse failure, persist the first parseable response, and return AgentStatusFailed only after all 3 attempts fail. Every other observable behavior (persistence, routing, idempotency, transport-error handling) stays unchanged.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these files fully before editing:
- `/workspace/agent/pr-reviewer/pkg/steps_planning.go` — the file to modify. The retry loop goes inside `func (s *planningStep) Run(...)` (currently lines ~83-126). Study the existing `runResult, runErr := s.runner.Run(ctx, prompt)` block and the existing `parsePlanningConcerns(ctx, runResult.Result)` validation gate — the retry wraps exactly this pair.
- `/workspace/agent/pr-reviewer/pkg/steps_planning_test.go` — the Ginkgo/Gomega test file. New cases go under a new `Describe("Run — in-agent retry on malformed JSON", ...)` block. Follow the existing test style: `runner = &mocks.ClaudeRunnerMock{}`, `runner.RunReturns(...)`, `runner.RunCallCount()`.

Reference docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, counterfeiter mocks, coverage ≥80%, error-path testing.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` wrapping (already used in this file via `errors.Wrapf`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — changelog entry format.

Verified facts (do not re-derive):
- `claudelib.ClaudeRunner` interface: `Run(ctx context.Context, prompt string) (*ClaudeResult, error)` (from `github.com/bborbe/agent/claude`).
- `claudelib.ClaudeResult` has a single field: `Result string`.
- The fake is `mocks.ClaudeRunnerMock` in `github.com/bborbe/maintainer/agent/pr-reviewer/mocks`. It exposes:
  - `RunReturns(result *claude.ClaudeResult, err error)` — same value for every call.
  - `RunReturnsOnCall(i int, result *claude.ClaudeResult, err error)` — per-call value (use this to make attempt 1 malformed and attempt 2 valid).
  - `RunCallCount() int`.
- `parsePlanningConcerns(ctx, body string) ([]struct{}, error)` — existing helper, unchanged; this is the validity check.
- `agentlib.AgentStatusFailed` and `agentlib.Result{Status:..., Message:...}` — existing return shape.
- The exported test alias `pkg.ParsePlanningConcernsForTest` already exists for direct helper tests.
</context>

<requirements>
1. In `/workspace/agent/pr-reviewer/pkg/steps_planning.go`, add a package-level constant near the top of the file (after the imports, before or beside `planningOutput`):
   ```go
   // maxPlanningAttempts is the hardcoded cap on Claude planning calls per
   // invocation. Malformed-JSON responses are retried up to this many times;
   // AgentStatusFailed is returned only after all attempts fail. Not configurable.
   const maxPlanningAttempts = 3
   ```

2. Rewrite the Claude-call section inside `func (s *planningStep) Run(...)` — specifically the block that currently reads (approximately lines 98-125):
   ```go
   prompt := claudelib.BuildPrompt(s.instructions.String(), nil, taskContent)
   runResult, runErr := s.runner.Run(ctx, prompt)
   if runErr != nil {
       glog.V(2).Infof("planning: claude failed nextPhase=human_review err=%v", runErr)
       return &agentlib.Result{
           Status:  agentlib.AgentStatusFailed,
           Message: fmt.Sprintf("planning claude run failed: %v", runErr),
       }, nil
   }
   if _, parseErr := parsePlanningConcerns(ctx, runResult.Result); parseErr != nil {
       glog.V(2).Infof("planning: malformed JSON from claude, not persisting err=%v", parseErr)
       return &agentlib.Result{
           Status:  agentlib.AgentStatusFailed,
           Message: fmt.Sprintf("planning: malformed JSON: %v", parseErr),
       }, nil
   }
   md.ReplaceSection(agentlib.Section{
       Heading: "## Plan",
       Body:    runResult.Result,
   })
   return s.routeFromPlan(ctx, md, runResult.Result)
   ```
   Replace it with a loop that keeps `runResult` and the two guarantees below:
   ```go
   prompt := claudelib.BuildPrompt(s.instructions.String(), nil, taskContent)

   var lastParseErr error
   for attempt := 1; attempt <= maxPlanningAttempts; attempt++ {
       runResult, runErr := s.runner.Run(ctx, prompt)
       if runErr != nil {
           // Transport error (nil result + err) is NOT retried — controller territory.
           glog.V(2).Infof("planning: claude failed nextPhase=human_review err=%v", runErr)
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: fmt.Sprintf("planning claude run failed: %v", runErr),
           }, nil
       }

       if _, parseErr := parsePlanningConcerns(ctx, runResult.Result); parseErr != nil {
           lastParseErr = parseErr
           if attempt < maxPlanningAttempts {
               glog.V(2).Infof(
                   "planning: attempt %d/%d malformed JSON, retrying err=%v",
                   attempt, maxPlanningAttempts, parseErr,
               )
               continue
           }
           // Exhausted all attempts.
           glog.V(2).Infof(
               "planning: malformed JSON after %d attempts, not persisting err=%v",
               maxPlanningAttempts, parseErr,
           )
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: fmt.Sprintf("planning: malformed JSON after %d attempts: %v", maxPlanningAttempts, lastParseErr),
           }, nil
       }

       // Parseable — persist this response and route.
       md.ReplaceSection(agentlib.Section{
           Heading: "## Plan",
           Body:    runResult.Result,
       })
       return s.routeFromPlan(ctx, md, runResult.Result)
   }

   // Unreachable — the loop returns on every path. Kept for compiler completeness.
   return &agentlib.Result{
       Status:  agentlib.AgentStatusFailed,
       Message: fmt.Sprintf("planning: malformed JSON after %d attempts: %v", maxPlanningAttempts, lastParseErr),
   }, nil
   ```
   Notes for the implementer:
   - `runResult` and `runErr` are declared with `:=` INSIDE the loop body (not before it) so each attempt gets a fresh value. Do NOT hoist them above the loop.
   - The retry log line MUST use the exact substring `planning: attempt` and `malformed JSON, retrying` so the acceptance grep `grep -n 'planning: attempt.*malformed JSON, retrying' agent/pr-reviewer/pkg/steps_planning.go` matches.
   - The exhaustion message MUST contain the exact substring `malformed JSON after 3 attempts` at runtime (via `fmt.Sprintf` with `maxPlanningAttempts=3`) so the test substring assertion passes.
   - Keep the `## Plan already present` idempotency branch (`if section, exists := md.FindSection("## Plan"); exists { ... }`) at the top of `Run` UNCHANGED and BEFORE the loop — the runner must not be called when a plan already exists.
   - If `golines`/`funlen` complains that `Run` is now too long, extract the loop into a private helper method on `*planningStep` (e.g. `runPlanningWithRetry(ctx, md, prompt) (*agentlib.Result, error)`) and call it from `Run`. Keep the idempotency check in `Run`, before the helper call.

3. Do NOT change `parsePlanningConcerns`, `routeFromPlan`, `postLGTMAndDone`, `handleEmptyPRURL`, `writePlanningVerdict`, or any routing logic. The retry loop only wraps the runner call + write-time parse gate.

4. In `/workspace/agent/pr-reviewer/pkg/steps_planning_test.go`, add a new `Describe("Run — in-agent retry on malformed JSON", func() { ... })` block with these cases. Build the markdown with a valid PR URL the same way existing cases do (frontmatter `ref: abc123` + `task_identifier` + body `https://github.com/bborbe/maintainer/pull/14`). Use a valid empty-concerns plan JSON as the "good" response (reuse the `json.Marshal(map[string]interface{}{... "concerns": []interface{}{}})` pattern already in the file, wrapped in ```` ```json ... ``` ````). Wire `prPoster.PostLGTMReturns(pkg.PostResult{Outcome: "success", ReviewID: 12345, PostedEvent: "COMMENT"})` for the success paths so the LGTM route completes.

   a. "attempt 1 succeeds": `runner.RunReturns(goodResult, nil)`. After `step.Run(ctx, md)`: `err` is nil, `runner.RunCallCount()` equals 1, `## Plan` section exists (`md.FindSection("## Plan")` → true), `result.Status` equals `agentlib.AgentStatusDone`.

   b. "attempt 2 succeeds": `runner.RunReturnsOnCall(0, &claudelib.ClaudeResult{Result: "Based on the diff, here is the plan..."}, nil)` (malformed — no JSON), `runner.RunReturnsOnCall(1, goodResult, nil)`. After `step.Run(ctx, md)`: `err` is nil, `runner.RunCallCount()` equals 2, `result.Status` is NOT `agentlib.AgentStatusFailed`, `## Plan` section exists and its `.Body` contains the substring `concerns` (proving the second/valid response was persisted, not the malformed first).

   c. "all 3 attempts fail": `runner.RunReturns(&claudelib.ClaudeResult{Result: "Based on the diff..."}, nil)` (malformed on every call). After `step.Run(ctx, md)`: `err` is nil, `runner.RunCallCount()` equals 3, `result.Status` equals `agentlib.AgentStatusFailed`, `result.Message` contains the substring `malformed JSON after 3 attempts`, and `## Plan` section does NOT exist (`md.FindSection("## Plan")` → false).

   d. "runner transport error not retried": `runner.RunReturns(nil, context.DeadlineExceeded)`. After `step.Run(ctx, md)`: `err` is nil, `result.Status` equals `agentlib.AgentStatusFailed`, `runner.RunCallCount()` equals 1.

   e. "idempotent re-entry — runner not called": build markdown that already contains a `## Plan` section with valid concerns JSON (mirror the `buildMarkdownWithExistingPlan` helper already in the file — you may reuse it if it is in scope, otherwise inline the same construction). `runner.RunReturns(goodResult, nil)` (should be irrelevant). After `step.Run(ctx, md)`: `err` is nil, `runner.RunCallCount()` equals 0.

5. Update `/workspace/CHANGELOG.md`: there is currently NO `## Unreleased` heading (the top release is `## v0.41.1`). Add a `## Unreleased` heading immediately below the intro paragraph block and above `## v0.41.1`, containing a single bullet. The bullet MUST match the acceptance grep `grep -n 'pr-reviewer.*retry.*planning' CHANGELOG.md`. Suggested:
   ```
   ## Unreleased

   - feat(pr-reviewer): retry the Claude planning call up to 3 times on malformed JSON before returning `AgentStatusFailed`, so intermittent MiniMax bad output (e.g. a leading `B` from "Based on...") self-corrects without an operator SHA-bump.
   ```
   (If a `## Unreleased` heading already exists when you get here, append the bullet to it instead of creating a second one.)

6. Run `make test` iteratively in `/workspace/agent/pr-reviewer/` after each meaningful change. Ensure ALL pre-existing test cases in `steps_planning_test.go` still pass — in particular the existing "when ## Plan JSON is malformed" and "when Claude runner returns an error" cases. Note: the existing "malformed JSON" case uses `runner.RunReturns(...)` (same value every call), so with the new loop the runner is now called 3 times before failing — that case asserts only status/message/no-plan, not call count, so it remains valid. If any existing case asserts a specific `RunCallCount()` around a malformed-single-response setup and now breaks, update that assertion to reflect the retry (3 calls) rather than removing the retry.
</requirements>

<constraints>
- The attempt cap is the hardcoded package-level constant `maxPlanningAttempts = 3` in `steps_planning.go`. No env var, no flag, no field on the step struct, no config plumbing.
- Do NOT add backoff, sleep, or jitter between attempts.
- Do NOT make the attempt count configurable.
- Do NOT retry on Claude transport errors (nil result, non-nil err) — fail immediately on the first one.
- Do NOT change the `AgentStatusFailed` return type/shape — only the message string differs.
- Do NOT add controller-side requeue, PR COMMENT posting, or `## Progress` task-file entries — those are out of scope (Layers 2/3).
- Do NOT change the planning prompt text.
- Do NOT add retry to the execution, ai_review, or verdict steps — planning-only.
- `parsePlanningConcerns` is reused UNCHANGED as the validity check.
- Routing downstream of parse-success is UNCHANGED: empty concerns → LGTM/done, non-empty → execution.
- Real Claude runner is NOT called in unit tests — the fake `mocks.ClaudeRunnerMock` drives every case.
- Wrap any new errors with `errors.Wrapf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`, never `context.Background()`. (The `fmt.Sprintf` calls above are for `Message` strings on a `Result`, not error wrapping — those are fine as-is, matching the existing code.)
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Follow project test conventions: external test package (`package pkg_test`), Ginkgo/Gomega, counterfeiter mocks.
</constraints>

<verification>
Run in `/workspace/agent/pr-reviewer/`:

```
cd /workspace/agent/pr-reviewer && make precommit
```
Must exit 0.

Then confirm the acceptance greps (from repo root `/workspace`):
```
grep -n 'maxPlanningAttempts' agent/pr-reviewer/pkg/steps_planning.go        # returns a line, value 3
grep -n 'malformed JSON after' agent/pr-reviewer/pkg/steps_planning.go       # returns a line
grep -n 'planning: attempt.*malformed JSON, retrying' agent/pr-reviewer/pkg/steps_planning.go  # returns a line
grep -n 'pr-reviewer.*retry.*planning' CHANGELOG.md                          # returns a line
```

Targeted test run (all five new cases plus regressions):
```
cd /workspace/agent/pr-reviewer && go test ./pkg/... -run TestPkg 2>&1 | tail -20
```
(Ginkgo suites run under a single `go test` entry; confirm 0 failures.)

Coverage check for the changed package:
```
cd /workspace/agent/pr-reviewer && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep steps_planning
```
Changed code paths (retry loop, exhaustion, transport-error early return, idempotent skip) must be covered.
</verification>
