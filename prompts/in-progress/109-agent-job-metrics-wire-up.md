---
status: committing
summary: Wired JobMetrics from agent/lib@v0.62.5 into agent/pr-reviewer Run() — registry+pusher init at top, RecordRun+RecordDuration at all 4 return paths, PushgatewayURL/TaskType struct fields added, agentName const added, prometheus/client_golang promoted to direct dep, CHANGELOG updated.
container: maintainer-109-agent-job-metrics-wire-up
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-14T10:10:11Z"
queued: "2026-05-14T10:10:11Z"
started: "2026-05-14T10:13:32Z"
---

<summary>

- Bump `github.com/bborbe/agent/lib` from `v0.62.4` to `v0.62.5` in `agent/pr-reviewer/go.mod`.
- Add `github.com/prometheus/client_golang` (sub-packages `prometheus` and `prometheus/push`) as a direct dep.
- Wire `libmetrics.JobMetrics` from the agent lib into `agent/pr-reviewer/main.go`'s `Run()` method following the frozen pattern already shipped in `agent/{claude,code,gemini}` (see `~/Documents/workspaces/agent/agent/claude/main.go` for the reference shape).
- Binary gains: a file-scope `const agentName = "pr-reviewer-agent"`, two new `application` struct fields (`PushgatewayURL`, `TaskType`), an init block at the top of `Run()` (registry + pusher + deferred PushContext + start time), and `RecordRun`+`RecordDuration` calls at every return path inside `Run()` (4 paths total).
- Single `## Unreleased` entry added to the maintainer repo's root `CHANGELOG.md` describing the wire-up plus env fields.
- No K8s manifest changes. No changes outside `agent/pr-reviewer/main.go` + its `go.mod`/`go.sum` + `CHANGELOG.md`. The helper method `createDeliverer` is NOT instrumented — only `Run()` itself.

</summary>

<objective>

After this prompt completes, the `agent/pr-reviewer` binary pushes per-agent metrics to the cluster's PushGateway at every Job end: `agent_job_run_total{status}`, `agent_job_last_run_timestamp_seconds{status}`, and `agent_job_duration_seconds`, all grouped by `agent="pr-reviewer-agent"` and `task_type` pusher dimensions. PromQL query `agent_job_run_total{agent="pr-reviewer-agent"}` returns non-zero counters after the first Job run in dev.

</objective>

<context>

**Parent context:** The shared `lib/metrics` package was shipped in the agent repo as part of spec 029 (released at `lib/v0.62.5`). The wire-up pattern was validated in `agent/{claude,code,gemini}` (released at `v0.62.5`). This prompt applies the SAME frozen wire-up pattern to the maintainer's `pr-reviewer` binary — design is done, this is pure template application. The exact `Run()` body to produce is inlined in `<requirements>` step 2.d — no external file needs to be consulted.

**Files modified by this prompt:**
- `agent/pr-reviewer/main.go`
- `agent/pr-reviewer/go.mod` + `go.sum`
- `CHANGELOG.md`

**Files NOT modified:**
- Any `k8s/*.yaml` manifest.
- Any `pkg/` subdir under `agent/pr-reviewer/`.
- Any `Dockerfile` or `Makefile`.
- Any binary other than `pr-reviewer`.

**Current state (verify with `grep` before editing):**

| Item | Current value | Target |
|------|---------------|--------|
| `agent/lib` version pin | `v0.62.4` | `v0.62.5` |
| Return paths in `Run()` | 4 (parse allowlist, create deliverer, agent run failed, success) | unchanged in count; each preceded by record calls |
| Helper method `createDeliverer` | exists, NOT instrumented | unchanged |

Verify with:
```bash
grep -n "github.com/bborbe/agent/lib v" agent/pr-reviewer/go.mod
grep -nE "^\s+return " agent/pr-reviewer/main.go
```

**Frozen contracts from spec 029 (do NOT alter):**
- Metric names: `agent_job_run_total`, `agent_job_last_run_timestamp_seconds`, `agent_job_duration_seconds`.
- Constructor signature: `libmetrics.NewJobMetrics(registry *prometheus.Registry, currentDateTime libtime.CurrentDateTime) JobMetrics`.
- `agent` and `task_type` are pusher `Grouping()` keys, NOT metric labels.
- Status values: `done`, `failed`, `needs_input` (from `agentlib.AgentStatusDone/Failed/NeedsInput`).
- `pusher.PushContext(ctx)` in deferred func (NOT `pusher.Push()` which is deprecated).
- `start` is `time.Time` from `libtime.NewCurrentDateTime().Now().Time()`.
- `RecordRun(status)` MUST be called BEFORE `RecordDuration(time.Since(start))` at every return path.

**Reference symbol verification (run AFTER step 1's `go get @v0.62.5` so the module is in the GOPATH cache):**

```bash
grep -n "type JobMetrics\|func NewJobMetrics\|func BuildJobMetricsName\|RecordRun\|RecordDuration" $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.62.5/metrics/metrics.go
```

Expected:
- `type JobMetrics interface` with methods `RecordRun(status agentlib.AgentStatus)` and `RecordDuration(d time.Duration)`.
- `func NewJobMetrics(registry *prometheus.Registry, currentDateTime libtime.CurrentDateTime) JobMetrics`.
- `func BuildJobMetricsName(agentName string) string`.

If symbols are missing AFTER step 1 completes, STOP and report `"status":"failed","blockers":["lib/metrics symbols not found at v0.62.5"]`. The most likely cause is the version tag did not propagate to the proxy — `go build` will fail with a clear error at step 4.

**Imports needed in main.go** (add to existing import block; reuse the existing `agentlib` and `libtime` aliases — both are already imported):

```go
import (
    "time"

    libmetrics "github.com/bborbe/agent/lib/metrics"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/push"
)
```

NOTE: `agentlib "github.com/bborbe/agent/lib"` and `libtime "github.com/bborbe/time"` are ALREADY imported — do NOT re-add them.

**Return-path topology in `Run()`** (4 paths total, all in the top-level `Run()` method):

| Line (current) | Trigger | Status to record |
|----------------|---------|------------------|
| `return err` after `ParseRepoAllowlist` | infra error | `agentlib.AgentStatusFailed` |
| `return err` after `a.createDeliverer(ctx)` | infra error | `agentlib.AgentStatusFailed` |
| `return errors.Wrap(ctx, err, "agent run failed")` | agent error | `agentlib.AgentStatusFailed` |
| `return agentlib.PrintResult(result)` | success | `result.Status` |

The `createDeliverer` helper method itself has 4 internal return paths (TaskID-empty → noop+nil; KAFKA_BROKERS empty → error; factory.CreateDeliverer error → error wrap; success → values), NONE of which are instrumented. Its return values flow up to `Run()`, which records at the `return err` site.

</context>

<requirements>

## 1. Bump `agent/lib` to v0.62.5 + add prometheus deps

```bash
cd agent/pr-reviewer
go get github.com/bborbe/agent/lib@v0.62.5
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/push
go mod tidy
```

Verify:
```bash
grep "github.com/bborbe/agent/lib v0.62.5\|prometheus/client_golang" agent/pr-reviewer/go.mod | grep -v indirect
```
Expected: `agent/lib v0.62.5` and `prometheus/client_golang` both listed as direct deps.

## 2. Update `agent/pr-reviewer/main.go`

Read the full file before editing.

**a. Add `const agentName` after the import block, before `func main()`:**
```go
const agentName = "pr-reviewer-agent"
```

**b. Append two new fields to the `application` struct body** (after the last existing field, before closing brace):
```go
PushgatewayURL string `required:"false" arg:"pushgateway-url" env:"PUSHGATEWAY_URL" usage:"Prometheus PushGateway URL"          default:"http://pushgateway:9090"`
TaskType       string `required:"false" arg:"task-type"       env:"TASK_TYPE"       usage:"Task type label for metric grouping" default:"unknown"`
```

**c. Add imports** to the existing import block — `agentlib` and `libtime` are already imported; only add `"time"`, `libmetrics`, and the two `prometheus/client_golang` paths:

```go
"time"

libmetrics "github.com/bborbe/agent/lib/metrics"
"github.com/prometheus/client_golang/prometheus"
"github.com/prometheus/client_golang/prometheus/push"
```

**d. Rewrite the `Run()` method body** — insert metrics init at the top (before existing `glog.V(2).Infof(...)`), and add `RecordRun`+`RecordDuration` before each of the four return paths:

```go
func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	registry := prometheus.NewRegistry()
	jobMetrics := libmetrics.NewJobMetrics(registry, libtime.NewCurrentDateTime())
	pusher := push.New(a.PushgatewayURL, libmetrics.BuildJobMetricsName(agentName)).
		Grouping("agent", agentName).
		Grouping("task_type", a.TaskType).
		Collector(registry)
	defer func() {
		if err := pusher.PushContext(ctx); err != nil {
			glog.Warningf("prometheus push failed: %v", err)
			return
		}
		glog.V(2).Infof("prometheus push completed")
	}()
	start := libtime.NewCurrentDateTime().Now().Time()

	glog.V(2).Infof("maintainer-agent-pr-reviewer started phase=%s", a.Phase)

	repoAllowlist, err := prpkg.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return err
	}
	glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))

	deliverer, cleanup, err := a.createDeliverer(ctx)
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return err
	}
	defer cleanup()

	authSetup := githubauth.NewGhAuthSetupGit(a.GHToken)
	result, err := factory.RunAgent(ctx, factory.RunConfig{
		ClaudeConfigDir: a.ClaudeConfigDir,
		AgentDir:        a.AgentDir,
		Model:           a.Model,
		GHToken:         a.GHToken,
		ReposPath:       a.ReposPath,
		WorkPath:        a.WorkPath,
		ReviewMode:      a.ReviewMode,
		RepoAllowlist:   repoAllowlist,
		AuthSetup:       authSetup,
		Phase:           a.Phase,
		TaskContent:     a.TaskContent,
		Deliverer:       deliverer,
	})
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "agent run failed")
	}
	jobMetrics.RecordRun(result.Status)
	jobMetrics.RecordDuration(time.Since(start))
	return agentlib.PrintResult(result)
}
```

**Critical:** the helper method `createDeliverer` at the bottom of the file is NOT modified — its 2 internal return paths are NOT instrumented. Recording happens only in `Run()` at the call site.

Verify every return path in `Run()` has both record calls:
```bash
grep -nE "RecordRun|RecordDuration|^\s+return " agent/pr-reviewer/main.go
```
Expected: each of the 4 `return` lines INSIDE `Run()` is preceded by `jobMetrics.RecordRun` and `jobMetrics.RecordDuration`. The 2 returns inside `createDeliverer` (after `// createDeliverer builds...`) are NOT instrumented.

Verify exactly 4 RecordRun calls:
```bash
grep -c "RecordRun" agent/pr-reviewer/main.go
```
Expected: 4.

## 3. Add CHANGELOG bullet

Ensure root `CHANGELOG.md` has a `## Unreleased` section. Append exactly one bullet describing the wire-up:

```markdown
- feat(agent/pr-reviewer): wire `JobMetrics` from `github.com/bborbe/agent/lib/metrics@v0.62.5` into `Run()` — constructs a fresh registry + pusher at startup, defers `PushContext` for end-of-run metric delivery, records run outcome and duration at every return path; adds `PUSHGATEWAY_URL` (default `http://pushgateway:9090`) and `TASK_TYPE` (default `unknown`) env fields; bumps `agent/lib` from `v0.62.4` to `v0.62.5`
```

Verify:
```bash
grep -A 5 "^## Unreleased" CHANGELOG.md | head -10
```
Expected: the new bullet present, no duplicate `## Unreleased` headers.

## 4. Run precommit

```bash
cd agent/pr-reviewer && make precommit
```
Must exit 0.

**Common compile errors to expect:**
- Local variable `metrics` shadowing import — use `jobMetrics` as the local variable name (consistent with the wire-up in `agent/{claude,code,gemini}`).
- `agentlib.AgentStatusFailed` — confirm the existing `agentlib` import alias.
- If `go mod tidy` removes `prometheus/client_golang` from direct deps, check that `prometheus.NewRegistry()` and `push.New(...)` are actually referenced inside `Run()`.

</requirements>

<constraints>

- **Precondition:** `github.com/bborbe/agent/lib@v0.62.5` must exist on the module proxy. Verify via the `grep` in `<context>` against the GOPATH module cache. If absent, run `GOPROXY=direct go list -m github.com/bborbe/agent/lib@v0.62.5` to force-fetch; if still absent, STOP and report failure.
- Only these files are modified: `agent/pr-reviewer/main.go`, `agent/pr-reviewer/go.mod`, `agent/pr-reviewer/go.sum`, and `CHANGELOG.md`. No K8s manifests, Dockerfiles, Makefiles, or `pkg/` subdirs are touched.
- The two new struct field names are EXACTLY `PushgatewayURL` and `TaskType` with the env names `PUSHGATEWAY_URL` and `TASK_TYPE` and the defaults `"http://pushgateway:9090"` and `"unknown"`. No other struct fields are added or modified.
- `const agentName` value: EXACTLY `"pr-reviewer-agent"` — this string drives operator dashboards via the `agent` grouping label.
- The metrics init block is inserted at the TOP of `Run()`, BEFORE the existing `glog.V(2).Infof(...)` log line.
- No new top-level helper functions are extracted from `main.go`. The metrics + pusher setup lives inline in `Run()`.
- No `init()` functions are added.
- The helper method `createDeliverer` is NOT instrumented — its internal returns flow up to `Run()`, which handles recording at the call site.
- `push.New(url, jobName string)` from `github.com/prometheus/client_golang/prometheus/push` is used directly. Do NOT use `bborbemetrics.NewPusher()` — its wrapper does not expose `Grouping`.
- `pusher.PushContext(ctx)` is used in the deferred func, NOT `pusher.Push()`.
- `start` is captured via `libtime.NewCurrentDateTime().Now().Time()` returning a `time.Time`. `time.Since(start)` returns the elapsed duration.
- **Recording call order is frozen:** `jobMetrics.RecordRun(status)` MUST be called BEFORE `jobMetrics.RecordDuration(time.Since(start))` at every return path inside `Run()`.
- Every return path inside `Run()` records — infrastructure errors use `agentlib.AgentStatusFailed`; the success path uses `result.Status`. No return path through `Run()` exits without recording.
- Error wrapping uses `github.com/bborbe/errors` (already used in the binary). NEVER use `fmt.Errorf`.
- Local variable in `Run()` is named `jobMetrics` (not `metrics`) to avoid shadowing any package import.
- Do NOT commit — dark-factory handles git.
- All existing tests must still pass.
- `make precommit` must exit 0 in `agent/pr-reviewer/`.

</constraints>

<verification>

Verify the lib version is pinned correctly:
```bash
grep -n "github.com/bborbe/agent/lib v0.62.5" agent/pr-reviewer/go.mod
```
Expected: one match.

Verify direct deps:
```bash
grep "prometheus/client_golang" agent/pr-reviewer/go.mod | grep -v indirect
```
Expected: at least one direct entry.

Verify `agentName` constant:
```bash
grep -n "const agentName" agent/pr-reviewer/main.go
```
Expected: one match — `"pr-reviewer-agent"`.

Verify new struct fields:
```bash
grep -n "PushgatewayURL\|TaskType\|PUSHGATEWAY_URL\|TASK_TYPE" agent/pr-reviewer/main.go
```
Expected: both fields present.

Verify metrics init block:
```bash
grep -n "prometheus.NewRegistry\|libmetrics.NewJobMetrics\|push.New\|PushContext\|libtime.NewCurrentDateTime" agent/pr-reviewer/main.go
```
Expected: file contains all five constructs.

Verify RecordRun call count (4 paths in Run, 0 in createDeliverer):
```bash
grep -c "RecordRun" agent/pr-reviewer/main.go
```
Expected: 4.

Verify CHANGELOG updated:
```bash
grep -n "pr-reviewer.*JobMetrics\|PUSHGATEWAY_URL\|agent-job-metrics" CHANGELOG.md | head -3
```
Expected: at least one match under `## Unreleased`.

Verify precommit passed (already required in step 4).

</verification>
