---
status: completed
spec: [056-github-releaser-ai-review-phase]
summary: Wire ai_review phase into CreateAgent factory alongside planning and execution phases
container: maintainer-releaser-ai-review-exec-221-spec-056-factory-registration
dark-factory-version: v0.173.0
created: "2026-05-31T20:35:00Z"
queued: "2026-05-31T20:54:57Z"
started: "2026-05-31T21:02:57Z"
completed: "2026-05-31T21:05:18Z"
branch: dark-factory/github-releaser-ai-review-phase
---

<summary>
- Add third `agentlib.NewPhase(domain.TaskPhaseAIReview, aiReviewStep)` call to CreateAgent
- Factory stays pure: zero business logic, no conditionals, no error return
- Update factory_test.go to assert exactly 3 phases registered
</summary>

<objective>
Wire the ai_review step into the factory so it is registered as a named phase alongside planning and execution. This is the single point of change for phase registration — no other file needs to be updated to add the new phase.
</objective>

<context>
Read `agent/github-releaser/pkg/factory/factory.go` to understand the current two-phase wiring.
Read `agent/github-releaser/pkg/steps_ai_review.go` for the NewAIReviewStep constructor signature.
Read `agent/github-releaser/pkg/githubreview/client.go` for the NewHTTPClient constructor.
Read `agent/github-releaser/pkg/factory/factory_test.go` to understand the existing test.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for factory conventions.

Key verified signatures:
- `domain.TaskPhaseAIReview` = `"ai_review"` from `github.com/bborbe/vault-cli/pkg/domain`
- `domain.TaskPhasePlanning` = `"planning"`, `domain.TaskPhaseExecution` = `"execution"`
- `agentlib.NewPhase(phase domain.TaskPhase, step agentlib.Step) *agentlib.Phase`
- `agentlib.NewAgent(phases ...*agentlib.Phase) *agentlib.Agent`
- `github.com/bborbe/errors`: no errors from factory functions (pure composition)
</context>

<requirements>
1. Open `agent/github-releaser/pkg/factory/factory.go` and modify `CreateAgent`.

2. Add `githubreview "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubreview"` to the import block.

3. In `CreateAgent`, after the existing `executionStep := releaserpkg.NewExecutionStep(...)` line, add two lines:
   ```go
   reviewClient := githubreview.NewHTTPClient(ghToken)
   reviewStep := releaserpkg.NewAIReviewStep(reviewClient, ghToken)
   ```
   Do NOT add a `CreateReviewClient` wrapper — `githubreview.NewHTTPClient(ghToken)` is already a constructor; a 1-line passthrough adds zero value (YAGNI; mirrors how `executionOps := CreateGitOps()` is the only existing pure-plumbing wrapper and it composes nothing).

4. Add `agentlib.NewPhase(domain.TaskPhaseAIReview, reviewStep)` as the third argument to `agentlib.NewAgent(...)`:
   ```go
   return agentlib.NewAgent(
       agentlib.NewPhase(domain.TaskPhasePlanning, planningStep),
       agentlib.NewPhase(domain.TaskPhaseExecution, executionStep),
       agentlib.NewPhase(domain.TaskPhaseAIReview, reviewStep),
   )
   ```

5. The factory function `CreateAgent` must NOT return an error and must NOT contain any conditional logic for phase registration.

6. **Update the `CreateAgent` doc comment** (currently says "assembles the planning + execution agent"). New comment must mention all three phases — planning, execution, ai_review — and briefly note the ordering (planning writes `## Plan`, execution writes `## Result`, ai_review writes `## Review` and drives terminal completion or escalation).

7. Update `agent/github-releaser/pkg/factory/factory_test.go`:
   a. NOTE on agent-lib limitation: `agent/lib v0.63.11` does NOT expose `Agent.Phases()`; `findPhase` is unexported (`agent_agent.go:126`). Direct phase-name assertion in a Go test is not possible without modifying agent-lib (out of scope). The existing test (`CreateAgent` returns non-nil; construction does not panic) is the canonical Go-side assertion and continues to hold for the three-phase composition.
   b. Add the new wiring inputs (e.g. `ghToken` if not already present) to the existing test invocation of `factory.CreateAgent(...)` so the three-phase agent constructs successfully.
   c. Do NOT add new `It` blocks that try to assert phase names — those would require an agent-lib accessor that does not exist. The primary AC for the phase-name presence is the grep on factory.go source (covered by the spec's Verification block) and the `domain.TaskPhaseAIReview` constant usage (also greppable).
   d. The factory test must continue to NOT need `githubreview` imports — `CreateAgent` accepts the same parameters it already does (the client is constructed inside `CreateAgent`, not injected from the test).
</requirements>

<constraints>
- `CreateAgent` must NOT return an error.
- `CreateAgent` must NOT contain any conditional logic, switch statements, or loops for phase registration.
- Existing tests under `pkg/factory/...` must continue to pass.
- `domain.TaskPhaseAIReview` is the typed constant from `github.com/bborbe/vault-cli/pkg/domain` — not a raw string literal.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run `cd agent/github-releaser && make test` — must pass.
Confirm `grep -nE 'agentlib\.NewPhase\(' agent/github-releaser/pkg/factory/factory.go | wc -l` returns 3.
</verification>