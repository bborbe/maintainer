---
status: completed
summary: Extracted routing logic from factory into main.go, changed CreateClaudeRunner to accept env map, added AgentRunner interface return from CreateAgent, and injected libtime.CurrentDateTimeGetter from callers — factory is now pure composition.
container: code-reviewer-059-review-agent-pr-reviewer-factory-pattern-fixes
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T15:24:46Z"
started: "2026-04-28T18:12:44Z"
completed: "2026-04-28T18:21:07Z"
---

<summary>
- A factory function contains a conditional that decides whether to include a GitHub token in a subprocess environment — this is routing logic, not composition
- A second factory function encodes business rules (no task ID means noop delivery, no brokers means error) and emits log messages, violating the zero-logic factory contract
- A third factory function returns a concrete pointer type instead of an interface, coupling callers to the implementation
- The time-provider helper is instantiated inside the factory instead of at the application entry point, making it impossible to inject a test double
- Factory functions must be pure composition: no conditionals, no logging, no direct instantiation of stateful helpers
</summary>

<objective>
Bring `pkg/factory/factory.go` into compliance with the project's factory pattern rules by extracting conditional logic to the application layer, returning an interface from `CreateAgent`, and injecting `libtime.CurrentDateTimeGetter` from `main.go`. After this fix the factory is pure composition and all routing decisions live in `main.go` where they are visible and testable.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `agent/pr-reviewer/agent/.claude/CLAUDE.md` if it exists for agent-specific patterns.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/factory/factory.go` — all functions: `CreateClaudeRunner` (~line 57), `CreateDeliverer` (~line 169), `CreateAgent` (~line 126), `CreateKafkaResultDeliverer` (~line 192)
- `agent/pr-reviewer/main.go` — the application entry point that calls factory functions; wiring site for moving routing decisions out of the factory
- `agent/pr-reviewer/pkg/steps/review.go` — uses `ClaudeRunner` to understand what interface `CreateAgent` callers need
- `agent/pr-reviewer/pkg/steps/gh_token.go` — reference for how steps receive their dependencies
</context>

<requirements>
1. **`CreateClaudeRunner` — remove the `if ghToken != ""` conditional** (~line 62). The factory must accept the already-built env map; routing decisions move to `main.go`:

   ```go
   // In main.go — before calling factory.CreateClaudeRunner
   env := map[string]string{}
   if ghToken != "" {
       env["GH_TOKEN"] = ghToken
   }
   runner := factory.CreateClaudeRunner(claudeConfigDir, agentDir, model, env, allowedTools)
   ```

   New `CreateClaudeRunner` signature (the only acceptable form — do NOT keep `ghToken string`):
   ```go
   func CreateClaudeRunner(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       env map[string]string,
       allowedTools claudelib.AllowedTools,
   ) claudelib.ClaudeRunner
   ```

2. **`CreateDeliverer` — move the `taskID == ""` and `len(brokers) == 0` routing into `main.go`**. The factory function should only wire:
   ```go
   // In main.go — before calling factory.CreateDeliverer
   if taskID == "" {
       glog.V(2).Infof("TASK_ID not set, skipping task result publishing")
       deliverer = delivery.NewNoopResultDeliverer()
       cleanup = func() {}
   } else {
       if len(brokers) == 0 {
           return errors.Errorf(ctx, "KAFKA_BROKERS must be set when TASK_ID is set")
       }
       deliverer, cleanup, err = factory.CreateDeliverer(ctx, taskID, brokers, branch, originalContent)
       if err != nil {
           return errors.Wrap(ctx, err, "create deliverer")
       }
   }
   ```
   Remove the conditional and the `glog.V(2).Infof` call from `CreateDeliverer`. The factory function should only construct the sync producer and the Kafka result deliverer — no guards, no log calls.

3. **`CreateAgent` — return an interface instead of `*agentlib.Agent`** (~line 126):

   `agentlib.Agent` is a struct (verified via `go doc github.com/bborbe/agent/lib.Agent`). No exported interface in the library matches its shape. Define a minimal local `AgentRunner` interface in `pkg/factory/` (or wherever the factory lives) that captures the `Run` method, and return that interface:

   ```go
   // pkg/factory/agent_runner.go (or top of factory.go)
   type AgentRunner interface {
       Run(ctx context.Context, phaseName domain.TaskPhase, taskContent string, /* … remaining params from agentlib.Agent.Run … */) (*agentlib.Result, error)
   }
   ```

   The exact `Run` signature is `(ctx context.Context, phaseName domain.TaskPhase, taskContent string, ...) (*Result, error)`. Read `~/go/pkg/mod/github.com/bborbe/agent/lib@<ver>/agent.go` for the full signature and copy it verbatim. Update `CreateAgent` to return `AgentRunner` and update all callers (likely a single caller in `main.go`).

4. **`CreateKafkaResultDeliverer` / `CreateDeliverer` — inject `libtime.CurrentDateTimeGetter` from `main.go`**:
   `libtime.NewCurrentDateTime()` is currently called inside `factory.go` (~line 192). Move this to `main.go`:
   ```go
   // In main.go
   currentDateTime := libtime.NewCurrentDateTime()
   // Pass it to the factory
   deliverer, cleanup, err = factory.CreateDeliverer(ctx, taskID, brokers, branch, originalContent, currentDateTime)
   ```
   Change the signature of `CreateDeliverer` (and `CreateKafkaResultDeliverer` if it is separate) to accept `libtime.CurrentDateTimeGetter` as a parameter.

5. **Run `cd agent/pr-reviewer && make test`** — must pass after all signature changes are propagated.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Factory functions must contain ZERO conditionals, ZERO log calls, ZERO direct instantiation of stateful time helpers — pure wiring only
- Grep-verify any `agentlib.*` symbol before using it — do NOT assume method signatures from memory
- Code changes MUST add or update tests for changed paths
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
