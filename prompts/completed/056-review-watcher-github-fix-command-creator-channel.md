---
status: completed
summary: Replaced per-call make(chan base.RequestID, 1) + base.NewCommandCreator pattern in buildCommandObject with a long-lived commandCreator field on kafkaPublisher, initialized once via base.RequestIDChannel(ctx) in NewCommandPublisher.
container: code-reviewer-056-review-watcher-github-fix-command-creator-channel
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:24:46Z"
started: "2026-04-28T18:05:33Z"
completed: "2026-04-28T18:07:49Z"
---

<summary>
- buildCommandObject creates a new one-shot buffered channel on every invocation
- The cqrs library provides RequestIDChannel(ctx) as the correct long-lived pattern
- Creating a new channel and CommandCreator per call bypasses the library's design intent
- The CommandCreator should be constructed once and stored as a field on kafkaPublisher
- This reduces per-call allocations and makes the code intent clearer
- The publisher_test.go uses FakeCommandObjectSender and may need minor updates to account for the new constructor signature
</summary>

<objective>
Replace the per-call `make(chan base.RequestID, 1)` + `base.NewCommandCreator` pattern in `buildCommandObject` with a long-lived `commandCreator` field on `kafkaPublisher`, initialized once in `NewCommandPublisher` using `base.RequestIDChannel(ctx)`. The `buildCommandObject` free function becomes a method on `*kafkaPublisher`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/publisher.go` (full): `kafkaPublisher` struct, `NewCommandPublisher`, `buildCommandObject` (~lines 74-87)
- `watcher/github/pkg/publisher_test.go` (full): existing tests that exercise `PublishCreate` and `PublishUpdateFrontmatter`
- `watcher/github/pkg/publisher_export_test.go`: any exported internals

Grep-verify cqrs symbols before writing:
```bash
grep -rn "func RequestIDChannel\|func NewCommandCreator" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/cqrs@*/base/ 2>/dev/null | head -10
```
Note the exact signature of `RequestIDChannel` — confirm it accepts a `context.Context` and returns `<-chan base.RequestID` or `chan base.RequestID`.
</context>

<requirements>
1. **Update `kafkaPublisher` struct** in `watcher/github/pkg/publisher.go` to add a `commandCreator` field. `base.CommandCreator` is an **interface** in the cqrs library — use the interface type directly, NOT a pointer:

   ```go
   type kafkaPublisher struct {
       sender         cdb.CommandObjectSender
       commandCreator base.CommandCreator
   }
   ```

2. **Update `NewCommandPublisher`** to accept a `ctx context.Context` parameter and initialize the `commandCreator`:

   ```go
   func NewCommandPublisher(ctx context.Context, sender cdb.CommandObjectSender) CommandPublisher {
       return &kafkaPublisher{
           sender:         sender,
           commandCreator: base.NewCommandCreator(base.RequestIDChannel(ctx)),
       }
   }
   ```

   Grep-verify `base.NewCommandCreator` accepts a channel type and confirm the return type before writing.

3. **Convert `buildCommandObject` to a method** on `*kafkaPublisher`, using the stored `commandCreator`:

   ```go
   func (p *kafkaPublisher) buildCommandObject(op base.CommandOperation, event base.Event) cdb.CommandObject {
       return cdb.CommandObject{
           Command: p.commandCreator.NewCommand(
               op,
               cqrsiam.Initiator("github-pr-watcher"),
               "",
               event,
           ),
           SchemaID: agentlib.TaskV1SchemaID,
       }
   }
   ```

4. **Update call sites** for `buildCommandObject` in `PublishCreate` and `PublishUpdateFrontmatter` to use the method form `p.buildCommandObject(...)`.

5. **Update `watcher/github/pkg/factory/factory.go`** (~line 39): pass `ctx` to `NewCommandPublisher`:
   ```go
   return pkg.NewCommandPublisher(ctx, sender), cleanup, nil
   ```

   Confirmed signatures via `go doc`: `RequestIDChannel(ctx) <-chan RequestID`, `NewCommandCreator(<-chan RequestID) CommandCreator` (interface return).

6. **Update `watcher/github/pkg/publisher_test.go`**: pass a `context.Background()` (tests are allowed) or the existing test ctx to `pkg.NewCommandPublisher` in any test setup that constructs the publisher directly.

7. Run `cd watcher/github && make test` — must pass.

8. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Grep-verify `base.RequestIDChannel`, `base.NewCommandCreator`, and the `CommandCreator` type before writing any struct literal
- Do NOT use `context.Background()` in `pkg/` business logic — the `ctx` in `NewCommandPublisher` is passed in from the factory
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd watcher/github && grep -n "make(chan base.RequestID" pkg/publisher.go
# Expected: no matches (per-call channel removed)

cd watcher/github && grep -n "commandCreator" pkg/publisher.go
# Expected: field declaration + usage in buildCommandObject

cd watcher/github && make precommit
</verification>
