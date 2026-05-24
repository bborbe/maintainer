---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `CreateSyncProducer` opens a TCP connection to Kafka at construction time — violates zero-I/O factory rule
- `CreateDeliverer` calls `CreateSyncProducer` (which does I/O) and returns a cleanup closure with side effects
- Both functions should be refactored so factories do pure wiring only; I/O moves to `runner.go` where lifecycle is managed
</summary>

<objective>
Refactor `CreateSyncProducer` and `CreateDeliverer` in `agent/pr-reviewer/pkg/factory/factory.go` to eliminate I/O at construction time. Factories must be pure dependency wiring — no network calls, no file I/O, no side-effect closures.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory pattern rules, zero-business-logic constraint.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks.

Files to read before making changes:
- `agent/pr-reviewer/pkg/factory/factory.go` — full file; understand `CreateSyncProducer` (~line 94), `CreateDeliverer` (~line 247), and how `SyncProducer` is used by `CreateKafkaResultDeliverer`
- `agent/pr-reviewer/pkg/factory/runner.go` — full file; understand `RunAgent` (~line 61) where Kafka connection lifecycle is currently managed after factory returns
- `agent/pr-reviewer/pkg/factory/factory_test.go` — full file; understand existing factory tests
</context>

<requirements>
**Execute steps in order. Run `make test` after step 2. Run `make precommit` only at the final step.**

1. **Refactor `CreateSyncProducer` in `agent/pr-reviewer/pkg/factory/factory.go`**

   Change `CreateSyncProducer` to accept a `*libkafka.Brokers` and return a `SyncProducerProvider` interface (new), with I/O moved to `runner.go`:

   ```go
   // SyncProducerProvider creates a SyncProducer on demand (I/O happens at call time, not construction).
   type SyncProducerProvider interface {
       Client(ctx context.Context) (SaramaClient, error)
       Producer(ctx context.Context) (SyncProducer, error)
   }

   // counterfeiter:generate -o ../mocks/sync-producer-provider.go --fake-name SyncProducerProvider . SyncProducerProvider
   ```

   Rename the existing `CreateSyncProducer` to `CreateSyncProducerProvider`:
   ```go
   func CreateSyncProducerProvider(
       brokers libkafka.Brokers,
   ) SyncProducerProvider {
       return &syncProducerProvider{brokers: brokers}
   }

   type syncProducerProvider struct {
       brokers libkafka.Brokers
   }

   func (s *syncProducerProvider) Client(ctx context.Context) (SaramaClient, error) {
       return libkafka.NewSyncProducerWithName(ctx, s.brokers, serviceName)
   }

   func (s *syncProducerProvider) Producer(ctx context.Context) (SyncProducer, error) {
       client, err := s.Client(ctx)
       if err != nil {
           return nil, err
       }
       return client.SyncProducer()
   }
   ```

   Add to the import block: `sync "github.com/IBM/sarama"` (or the actual sarama package used by `libkafka` — confirm via grep).

   Note: `libkafka.NewSyncProducerWithName` may return a `*sarama.SyncProducer` or a wrapper. Adapt to the actual return type after reading `factory.go` and `libkafka` source.

2. **Refactor `CreateDeliverer` in `agent/pr-reviewer/pkg/factory/factory.go`**

   Change `CreateDeliverer` to accept a `SyncProducer` (already-connected) as a parameter and return only `(agentlib.ResultDeliverer, error)` — no cleanup closure:

   ```go
   func CreateDeliverer(
       syncProducer SyncProducer,
       taskID agentlib.TaskIdentifier,
       branch base.Branch,
       originalContent string,
       currentDateTime libtime.CurrentDateTimeGetter,
   ) (agentlib.ResultDeliverer, error) {
       return CreateKafkaResultDeliverer(syncProducer, branch, taskID, originalContent, currentDateTime)
   }
   ```

   The `syncProducer.Close()` lifecycle moves to `runner.go` where it is managed via `defer` alongside the existing `defer producer.Close()` pattern.

3. **Update `runner.go` to wire the new signatures**

   In `RunAgent` (`runner.go:61`), move the Kafka connection creation (currently inside `CreateSyncProducer`) to happen at the top of `RunAgent` using the new provider, and pass the connected `SyncProducer` to `CreateDeliverer`:

   ```go
   func RunAgent(ctx context.Context, cfg RunConfig) (*agentlib.Result, error) {
       // ... existing pruning and plugin installation ...

       // Kafka connection lifecycle — I/O happens here, not in factory
       syncProducerProvider := factory.CreateSyncProducerProvider(cfg.Brokers)
       saramaClient, err := syncProducerProvider.Client(ctx)
       if err != nil {
           return nil, errors.Wrap(ctx, err, "create kafka client failed")
       }
       defer saramaClient.Close()

       deliverer, err := factory.CreateDeliverer(
           syncProducerProvider, // pass provider for producer creation
           cfg.TaskID,
           cfg.Branch,
           cfg.OriginalContent,
           cfg.CurrentDateTime,
       )
       if err != nil {
           return nil, errors.Wrap(ctx, err, "create deliverer failed")
       }
       // ... rest unchanged ...
   }
   ```

   Adapt the `CreateDeliverer` signature to accept the provider (which can create a `SyncProducer` on demand) rather than an already-connected `SyncProducer`, since `NewKafkaResultDeliverer` may need the producer interface specifically.

4. **Regenerate mocks**

   ```bash
   cd agent/pr-reviewer && go generate ./pkg/...
   ```

5. **Run `make test`** to verify compilation and tests pass:

   ```bash
   cd agent/pr-reviewer && make test
   ```

6. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/pkg/factory/` and `agent/pr-reviewer/pkg/factory/runner.go`
- Do NOT commit — dark-factory handles git
- Factory functions must have zero I/O — wiring only, no network calls, no file I/O
- `CreateDeliverer` must NOT return a cleanup closure — lifecycle management belongs in `runner.go`
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
