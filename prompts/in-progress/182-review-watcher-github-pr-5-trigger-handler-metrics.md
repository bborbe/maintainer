---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T22:38:58Z"
---

<summary>
- `SinglePRTriggerHandler` processes PRs via the `/trigger` endpoint but records no metrics for outcomes (create, skipped, error)
- If the trigger endpoint is used directly (without going through `Watcher.Poll`), metrics will silently miss from dashboards and alerts
- The `IncPRPublished("error")` label conflates two distinct failure modes (trust-check failure and Kafka publish failure) into one bucket
</summary>

<objective>
Inject `Metrics` into `singlePRTriggerHandler` and record `IncPRPublished` for each outcome. Also refine the "error" label to distinguish "trust_error" from "kafka_error".
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-prometheus-metrics-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface-based metrics, naming, labels.

Files to read before making changes:
- `watcher/github-pr/pkg/metrics.go` — understand Metrics interface and IncPRPublished
- `watcher/github-pr/pkg/handler/trigger_handler.go` — full file; understand current struct and Handle method
- `watcher/github-pr/pkg/factory/single_pr.go` — full file; understand how handler is wired
</context>

<requirements>

**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Add `Metrics` to `singlePRTriggerHandler` struct in `trigger_handler.go`:**

   ```go
   type singlePRTriggerHandler struct {
       ghClient           GitHubClient
       createSender       task.CreateCommandSender
       taskCreationFilter filter.TaskCreationFilter
       trustDecision      trust.Trust
       stage              string
       maxSlugLen         int
       maxTitleLen        int
       taskSuffix         string
       metrics            Metrics  // NEW: for recording trigger outcomes
   }
   ```

2. **Update `NewSinglePRTriggerHandler` to accept and store `Metrics`:**

   ```go
   func NewSinglePRTriggerHandler(
       ghClient GitHubClient,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       metrics Metrics,  // NEW parameter
   ) SinglePRTriggerHandler {
       return &singlePRTriggerHandler{
           ghClient:           ghClient,
           createSender:       createSender,
           taskCreationFilter: taskCreationFilter,
           trustDecision:      trustDecision,
           stage:              stage,
           maxSlugLen:         maxSlugLen,
           maxTitleLen:        maxTitleLen,
           taskSuffix:         taskSuffix,
           metrics:            metrics,
       }
   }
   ```

3. **Add `IncPRPublished` calls to `Handle` in `trigger_handler.go`:**

   In the `Handle` method, record metrics for each outcome:
   - On filter skip: `h.metrics.IncPRPublished("skipped")`
   - On trust failure: `h.metrics.IncPRPublished("trust_error")` (new label value)
   - On Kafka send failure: `h.metrics.IncPRPublished("kafka_error")` (new label value)
   - On success: `h.metrics.IncPRPublished("create")`

   ```go
   // After filter rejection check:
   if h.taskCreationFilter.Skip(...) {
       h.metrics.IncPRPublished("skipped")
       return errors.Errorf(ctx, "PR skipped by filter")
   }

   // After trust check failure:
   if err != nil {
       h.metrics.IncPRPublished("trust_error")
       return errors.Wrap(ctx, err, "check trust")
   }

   // After Kafka send failure:
   if err := h.createSender.SendCommand(ctx, cmd); err != nil {
       h.metrics.IncPRPublished("kafka_error")
       return errors.Wrap(ctx, err, "send create task command")
   }

   // On success (before returning nil):
   h.metrics.IncPRPublished("create")
   ```

4. **Add `"trust_error"` and `"kafka_error"` to pre-initialized labels in `metrics.go`:**

   In the `init()` function, update the `prPublishedTotal` pre-initialization:
   ```go
   prPublishedTotal.WithLabelValues("create").Add(0)
   prPublishedTotal.WithLabelValues("update_frontmatter").Add(0)
   prPublishedTotal.WithLabelValues("skipped").Add(0)
   prPublishedTotal.WithLabelValues("error").Add(0)
   prPublishedTotal.WithLabelValues("trust_error").Add(0)    // NEW
   prPublishedTotal.WithLabelValues("kafka_error").Add(0)   // NEW
   ```

5. **Update `CreateSinglePRTriggerHandler` in `factory/single_pr.go`:**

   Add `metrics Metrics` parameter and pass it to the constructor:
   ```go
   func CreateSinglePRTriggerHandler(
       httpClient *http.Client,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       metrics Metrics,  // NEW parameter
   ) handler.SinglePRTriggerHandler {
       ghClient := pkg.NewGitHubClient(httpClient)
       return handler.NewSinglePRTriggerHandler(
           ghClient,
           createSender,
           taskCreationFilter,
           trustDecision,
           stage,
           maxSlugLen,
           maxTitleLen,
           taskSuffix,
           metrics,  // NEW
       )
   }
   ```

6. **Update call site in `main.go`:** Pass `pkg.NewMetrics()` when calling `CreateSinglePRTriggerHandler`.

7. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compilation errors.

8. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change `watcher/github-pr/pkg/handler/trigger_handler.go`, `watcher/github-pr/pkg/metrics.go`, `watcher/github-pr/pkg/factory/single_pr.go`, and `watcher/github-pr/main.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Wrapf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- The new label values `"trust_error"` and `"kafka_error"` must be pre-initialized in `init()` to avoid missing Prometheus series
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm metrics field in struct:
grep -n "metrics.*Metrics" watcher/github-pr/pkg/handler/trigger_handler.go

# Confirm IncPRPublished calls:
grep -n "IncPRPublished" watcher/github-pr/pkg/handler/trigger_handler.go

# Confirm new label values pre-init:
grep -n "trust_error\|kafka_error" watcher/github-pr/pkg/metrics.go
</verification>
