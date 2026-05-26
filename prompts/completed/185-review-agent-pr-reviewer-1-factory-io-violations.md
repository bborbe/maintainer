---
status: completed
summary: Refactored CreateSyncProducer/CreateDeliverer in factory.go — moved Kafka lifecycle to main.go, CreateDeliverer now pure wiring with no error/cleanup closure
container: maintainer-exec-185-review-agent-pr-reviewer-1-factory-io-violations
dark-factory-version: v0.173.0
created: "2026-05-24T00:00:00Z"
queued: "2026-05-26T05:58:50Z"
started: "2026-05-26T05:58:51Z"
completed: "2026-05-26T06:02:24Z"
---

<summary>
- `CreateSyncProducer` in factory.go does Kafka I/O at construction — violates zero-I/O factory rule
- `CreateDeliverer` calls `CreateSyncProducer` (I/O) and returns a cleanup closure with side effects
- Fix: delete `CreateSyncProducer`, change `CreateDeliverer` to accept an already-constructed `libkafka.SyncProducer`, move connection lifecycle + `defer Close()` to `main.go` (the composition root)
</summary>

<objective>
Refactor `CreateSyncProducer` and `CreateDeliverer` in `agent/pr-reviewer/pkg/factory/factory.go` so factories do pure dependency wiring. After this change: `CreateDeliverer` takes a connected `libkafka.SyncProducer` and returns only `agentlib.ResultDeliverer` (no error, no cleanup closure); `CreateSyncProducer` is deleted; `main.go` owns the Kafka connection lifecycle via `defer`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory zero-business-logic rule.

Files to read before making changes:
- `agent/pr-reviewer/pkg/factory/factory.go` — full file; current `CreateSyncProducer` (~line 95) and `CreateDeliverer` (~line 269)
- `agent/pr-reviewer/main.go` — lines ~285-310; current `factory.CreateDeliverer` caller
- `agent/pr-reviewer/pkg/factory/factory_test.go` — line ~104; existing test for `CreateDeliverer`

Real types (verified in factory.go):
- `libkafka.SyncProducer` — interface from `github.com/bborbe/kafka`; `libkafka.NewSyncProducerWithName(ctx, brokers, name)` constructs one
- `CreateKafkaResultDeliverer(syncProducer libkafka.SyncProducer, branch, taskID, originalContent, currentDateTime) agentlib.ResultDeliverer` — already pure wiring, leave it alone
</context>

<requirements>

**Execute in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Delete `CreateSyncProducer` from `agent/pr-reviewer/pkg/factory/factory.go`.**

   The function (~line 95) opens a Kafka connection — pure I/O. main.go will call `libkafka.NewSyncProducerWithName` directly instead. Remove the function and any imports it uniquely required (e.g., `serviceName` constant if only used here — but keep it if other factories reference it; grep first).

2. **Refactor `CreateDeliverer` in `agent/pr-reviewer/pkg/factory/factory.go` (~line 269).**

   New signature accepts an already-connected `libkafka.SyncProducer`. Returns only the deliverer — no error, no cleanup closure:

   ```go
   func CreateDeliverer(
       syncProducer libkafka.SyncProducer,
       taskID agentlib.TaskIdentifier,
       branch base.Branch,
       originalContent string,
       currentDateTime libtime.CurrentDateTimeGetter,
   ) agentlib.ResultDeliverer {
       return CreateKafkaResultDeliverer(
           syncProducer,
           branch,
           taskID,
           originalContent,
           currentDateTime,
       )
   }
   ```

   - Drop the `ctx context.Context` and `brokers libkafka.Brokers` parameters.
   - Drop the `(_, func(), error)` return — just `agentlib.ResultDeliverer`.
   - No error wrapping needed in the body (no I/O, no failure mode).

3. **Update `agent/pr-reviewer/main.go` (~line 295) to own the Kafka connection lifecycle.**

   Replace the `factory.CreateDeliverer(ctx, ...)` call with:

   ```go
   syncProducer, err := libkafka.NewSyncProducerWithName(ctx, cfg.Brokers, "agent-pr-reviewer")
   if err != nil {
       return errors.Wrap(ctx, err, "create kafka sync producer")
   }
   defer func() {
       if err := syncProducer.Close(); err != nil {
           glog.Warningf("close sync producer failed: %v", err)
       }
   }()

   deliverer := factory.CreateDeliverer(
       syncProducer,
       cfg.TaskID,
       cfg.Branch,
       cfg.OriginalContent,
       cfg.CurrentDateTime,
   )
   ```

   - Use the same service name string `"agent-pr-reviewer"` that was hardcoded in the deleted `CreateSyncProducer` (check `serviceName` constant in factory.go; grep `serviceName` for usages before deciding whether to inline or import).
   - Remove the local `cleanup` variable + deferred `cleanup()` call that the old `CreateDeliverer` returned.
   - Update imports: add `libkafka "github.com/bborbe/kafka"` if not already imported in main.go.

4. **Update `agent/pr-reviewer/pkg/factory/factory_test.go` (~line 104) for the new `CreateDeliverer` signature.**

   The test currently calls `_, _, err := factory.CreateDeliverer(ctx, ...)` expecting `(_, func(), error)`. New call returns only `agentlib.ResultDeliverer`:

   ```go
   deliverer := factory.CreateDeliverer(
       fakeSyncProducer,    // pass a Counterfeiter-generated mock of libkafka.SyncProducer
       taskID,
       branch,
       originalContent,
       currentDateTime,
   )
   Expect(deliverer).NotTo(BeNil())
   ```

   If a `libkafka.SyncProducer` mock doesn't exist yet in `mocks/`, generate one. Use the existing counterfeiter pattern in this repo. Add a `//counterfeiter:generate` directive in a suite test file if needed.

   Remove any test setup that constructed `brokers` for `CreateDeliverer` — no longer needed since the test passes the producer directly.

5. **Regenerate mocks if a new `//counterfeiter:generate` was added:**

   ```bash
   cd agent/pr-reviewer && go generate ./pkg/...
   ```

6. **Run `make test`:**

   ```bash
   cd agent/pr-reviewer && make test
   ```

   Fix any compilation errors. Likely culprits: other callers of `CreateSyncProducer` (there should be none — confirm via grep), or import cleanup in factory.go.

7. **Run `make precommit`:**

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/` — specifically `pkg/factory/factory.go`, `main.go`, `pkg/factory/factory_test.go`, and possibly new mocks under `mocks/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass after signature update
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Factory functions must be pure wiring: no I/O, no error returns, no cleanup closures
- Do NOT invent local type aliases like `SyncProducer` or `SaramaClient` — use `libkafka.SyncProducer` everywhere
- The `serviceName` for the producer should match what the deleted `CreateSyncProducer` used (likely the constant `serviceName` in factory.go — keep it accessible or inline `"agent-pr-reviewer"`)
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm CreateSyncProducer deleted:
! grep -n "func CreateSyncProducer" pkg/factory/factory.go

# Confirm CreateDeliverer signature is pure wiring (no error return):
grep -n "func CreateDeliverer" pkg/factory/factory.go

# Confirm main.go owns the connection lifecycle:
grep -n "libkafka.NewSyncProducerWithName\|syncProducer.Close" main.go
</verification>
