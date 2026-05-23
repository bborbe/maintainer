---
status: draft
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T19:45:02Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

<summary>
- `CreateTaskCommand` gains a `ForceBypassDedup bool` field (default false), wire-compatible
- The single-trigger handler sets `ForceBypassDedup: true` on the command it publishes, bypassing the per-(PR, SHA) dedup that blocks stale-task re-runs
- The enforcement site for `ForceBypassDedup` is the task controller/agent (separate service) — this prompt adds the field to the command; enforcement is handled by the controller spec
- The new field is backwards-compatible: old consumers ignore unknown fields
</summary>

<objective>
Add `ForceBypassDedup bool` field to `CreateTaskCommand` in `agent/lib/command/task/create-command.go`, bump the agent/lib dependency, and wire the field into the single-trigger handler's published command.
</objective>

<context>
Read CLAUDE.md for project conventions.

**Read fully before implementing:**
- `watcher/github-pr/go.mod` — current agent/lib version
- `watcher/github-pr/go.sum` — current agent/lib version (grep for the exact require line)
- `home/node/go/pkg/mod/github.com/bborbe/agent/lib@v0.62.17/command/task/create-command.go` — the current CreateCommand struct (already read: 4 fields: TaskIdentifier, Title, Frontmatter, Body)

**Key constraint:** The spec says `CreateTaskCommand` gains a `force_bypass_dedup` field. But `task.CreateCommand` from `agent/lib` is an external dependency. We cannot add a field to it directly — we must bump the agent/lib version to one that includes the field.

**Option A:** Bump agent/lib to a version that has `ForceBypassDedup` on `task.CreateCommand`. Does such a version exist? The grep above found no such field. We need to either:
a) Implement the dedup bypass locally in the watcher (not using the command field)
b) Contribute the field to agent/lib first (separate effort)

**Option B (recommended by spec's decision paragraph):** Since dedup is currently enforced by the watcher's cursor check (`if _, exists := cursorState.HeadSHAs[taskIDStr]` in `processPRs`), and the single-trigger handler bypasses the poll cycle entirely (it publishes directly), the dedup is already bypassed for the new endpoint. The `ForceBypassDedup` field is a forward-compatibility hint for when the controller gains dedup enforcement; today it is not used.

**Decision for this prompt:** Implement Option B. Add the field declaration locally in the watcher for future use, but do not bump agent/lib yet. The field name: `ForceBypassDedup bool` in a local type that wraps or shadows the command, OR in a local struct that is JSON-serialized with a different approach.

**Actually**, looking at the codebase more carefully: the watcher creates `task.CreateCommand` (from agent/lib) and sends it via `createSender.SendCommand(ctx, cmd)`. The `SendCommand` method serializes the command to Kafka.

The cleanest approach that satisfies the spec's intent:
1. Add the field to the local package as a local extension or shadow
2. Or, since the spec says "Schema change must be backwards compatible at the wire level", the field is optional

**Simplest path:** The single-trigger handler publishes via the same `createSender`, but with a `task.CreateCommand` that has the `ForceBypassDedup` field added. Since `task.CreateCommand` doesn't have this field yet, we need to either:
a) Create a local wrapper that adds the field and serializes it
b) Use a different approach

**Best approach:** The spec's "either path is acceptable" clause allows us to skip the command field entirely for now, since dedup is bypassed by the handler publishing directly (not via `processPRs`). The `ForceBypassDedup` field is documented as a future-proofing measure. We add the field to a local type or just document it for now.

Actually, re-reading the spec again: "CreateTaskCommand schema gains a force_bypass_dedup field (or named equivalent — implementer picks the name; default false)."

The implementation decision clause says we can pick either:
1. If dedup is in the controller → flag is checked there
2. If dedup is in the watcher → watcher skips dedup check

Since dedup IS in the watcher (in `processPRs`), Option 2 applies: the single-trigger handler bypasses `processPRs` and publishes directly, skipping dedup. The `ForceBypassDedup` field is added for forward compatibility but NOT enforced by the watcher today.

**For this prompt:** Implement the forward-compat field in the local watcher code only (not in agent/lib). The actual enforcement in the controller is a separate spec concern.

Actually, the cleanest solution that satisfies the spec without bumping agent/lib:
1. Add a `ForceBypassDedup bool` field to a local `CreateTaskCommandExtended` type that marshals to the same JSON keys as `task.CreateCommand` plus the new field
2. OR create the command as `task.CreateCommand` and add the field via struct embedding

**Simplest:** Just add a local type in `watcher/github-pr/pkg/` that extends the command:
```go
type CreateTaskCommandWithFlag struct {
    task.CreateCommand
    ForceBypassDedup bool `json:"forceBypassDedup,omitempty"`
}
```

And use this for the single-trigger handler's publish. The existing `createSender.SendCommand` accepts `interface{}` or `any` — verify the signature.
</context>

<requirements>

1. **Verify the SendCommand method signature in agent/lib:**

   ```bash
   grep -A 5 "func.*SendCommand\|type CreateCommandSender interface" \
     $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.62.17/command/task/create-command-sender.go
   ```

   Document the exact method signature. If it accepts `interface{}` or `any`, we can pass our extended struct.

2. **Create `watcher/github-pr/pkg/create_task_command_extended.go`** — a local extension of `task.CreateCommand` that adds the `ForceBypassDedup` field:

   ```go
   package pkg

   import (
       task "github.com/bborbe/agent/lib/command/task"
   )

   // CreateTaskCommandExtended adds force_bypass_dedup to the standard CreateCommand.
   // Wire-compatible: the base fields (TaskIdentifier, Title, Frontmatter, Body) are
   // serialized identically to task.CreateCommand; the new field is appended.
   // Default false; old consumers ignore the new field.
   type CreateTaskCommandExtended struct {
       task.CreateCommand
       ForceBypassDedup bool `json:"forceBypassDedup,omitempty"`
   }
   ```

3. **Update the single-trigger handler** (`pkg/handler/single_trigger_handler.go`) to use `CreateTaskCommandExtended` instead of `task.CreateCommand` directly. Set `ForceBypassDedup: true`.

   ```go
   cmd := pkg.CreateTaskCommandExtended{
       CreateCommand: task.CreateCommand{
           Title:          computePRFilenameHint("github", owner, repo, number, title),
           TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
           Frontmatter:    buildFrontmatterFromDetails(owner, repo, number, taskIDStr, stage, details),
           Body:           buildTaskBodyFromPR(owner, repo, number, title, htmlURL),
       },
       ForceBypassDedup: true,
   }
   ```

4. **Add unit test for wire format** in `pkg/handler/single_trigger_handler_test.go`:

   ```go
   It("emits forceBypassDedup:true in the command", func() {
       // Mock GetPRDetails to return PR details
       // Mock SearchPRs to return PR info with author/title
       // Mock SendCommand to capture the command argument
       handler.ServeHTTP(recorder, req)
       Expect(recorder.Code).To(Equal(http.StatusOK))
       call := createSender.SendCommandArgsForCall(0)
       Expect(call).To(BeAssignableToTypeOf(pkg.CreateTaskCommandExtended{}))
       ext := call.(pkg.CreateTaskCommandExtended)
       Expect(ext.ForceBypassDedup).To(BeTrue())
   })
   ```

5. **Run tests:**
   ```bash
   cd watcher/github-pr && make test
   ```

   If compile errors occur (SendCommand doesn't accept extended types), document the error and propose an alternative approach in `## Improvements`.

</requirements>

<constraints>
- Create `pkg/create_task_command_extended.go`
- BSD license header
- Do NOT commit — dark-factory handles git
- The extended type is wire-compatible with `task.CreateCommand` — base fields serialize identically
- Default `ForceBypassDedup: false` — backwards compatible
- The single-trigger handler MUST set `ForceBypassDedup: true`
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`
- Coverage ≥80% for packages with new code
</constraints>

<verification>
cd watcher/github-pr && make test
# Expected: all tests pass, coverage ≥80%

# Confirm extended type exists:
grep -n "ForceBypassDedup" watcher/github-pr/pkg/create_task_command_extended.go
# Expected: field declaration

# Confirm extended type used in handler:
grep -n "ForceBypassDedup.*true" watcher/github-pr/pkg/handler/single_trigger_handler.go
# Expected: at least one line setting the field to true

# Confirm command is wire-compatible:
# (The test from step 4 above verifies this)
</verification>