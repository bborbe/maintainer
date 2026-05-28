---
status: approved
spec: ["047"]
created: "2026-05-28T00:00:00Z"
queued: "2026-05-28T05:18:37Z"
---

<summary>
- Wires the planning step from prompt 2 into a runnable agent: factory builds the AgentProvider, main.go dispatches by task_type, and cmd/run-task provides a local file-driven entry point.
- Factory follows the canonical Create* pattern from agent/pr-reviewer: zero business logic, all wiring.
- AgentProvider routes `task_type: github-release` to the planning agent and `task_type: healthcheck` to a liveness agent (parity with pr-reviewer; no test).
- main.go replaces its placeholder Result block with a real dispatch chain that mirrors pr-reviewer's Run method.
- cmd/run-task reads a markdown task file, runs the agent against it, writes the mutated content back — enables local fixture-based verification without Kafka/Claude tokens (mocked variant).
- Adds two checked-in fixture task files for the verification block: a happy-path CHANGELOG and an invalid CHANGELOG (Unreleased not first).
- Adds the root CHANGELOG.md Unreleased bullet covering the integration.
</summary>

<objective>
Complete the github-releaser planning integration by wiring three artifacts:

1. `agent/github-releaser/pkg/factory/factory.go` — exports `CreateAgent`, `CreateAgentProvider`, `CreateClaudeRunner`, `CreateKafkaResultDeliverer`, `CreateFileResultDeliverer`, `CreateDeliverer`. Routes `task_type: github-release` to a single-phase planning agent and `task_type: healthcheck` to a liveness agent.
2. `agent/github-releaser/main.go` — replace the placeholder Result block with an AgentProvider-dispatched run identical in shape to `agent/pr-reviewer/main.go` Run method.
3. `agent/github-releaser/cmd/run-task/main.go` — local file-driven entry point mirroring `agent/pr-reviewer/cmd/run-task/main.go`.
4. Two checked-in test fixtures and the root CHANGELOG Unreleased entry.

End state: `cd agent/github-releaser && make precommit` exits 0; the binary compiles; the placeholder Result block is gone; the canonical Create* factory shape is present.
</objective>

<context>
Read before writing code (all paths repo-relative; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root.
- `specs/in-progress/047-github-releaser-planning-phase-integration.md` — re-read Desired Behavior 1, 2, 7, 8 plus the Verification block.
- `agent/pr-reviewer/pkg/factory/factory.go` — THE reference for this prompt. Read in full. Mirror the structure: `serviceName` constant, per-phase tool variables, `CreateClaudeRunner`, `CreateKafkaResultDeliverer`, `CreateFileResultDeliverer`, `CreateAgent`, `CreateAgentProvider`, `CreateDeliverer`. The github-releaser variant is SIMPLER (one phase, no PrPoster, no Verifier, no allowlist, no review-mode) — strip what doesn't apply, keep the shape.
- `agent/pr-reviewer/main.go` — THE reference for the github-releaser main.go rewrite. Mirror the `Run` method structure: prometheus setup, deliverer creation, agent dispatch via provider.Get, `factory.RunAgent`-like flow, metrics push, `agentlib.PrintResult`. Strip pr-reviewer-specific fields (repo allowlist, App auth resolution, posters).
- `agent/pr-reviewer/cmd/run-task/main.go` — THE reference for the new `cmd/run-task/main.go`. Mirror it; strip pr-reviewer-specific fields.
- `agent/github-releaser/main.go` — current state — placeholder Result block at lines 87-94. Existing struct fields (TaskContent, Branch, Phase, KafkaBrokers, TaskID, PushgatewayURL, TaskType, SentryDSN/Proxy) STAY — only the Run-method body changes.
- `agent/github-releaser/pkg/steps_planning.go` — produced by prompt 2; exports `NewPlanningStep(runner claudelib.ClaudeRunner, fetcher githubchangelog.Fetcher) agentlib.Step` and the `AgentLogin` const.
- `agent/github-releaser/pkg/githubchangelog/fetcher.go` — produced by prompt 1; exports `NewHTTPFetcher(token string) Fetcher`.
- `agent/github-releaser/CHANGELOG.md` — at repo root (`/CHANGELOG.md`, NOT `agent/github-releaser/CHANGELOG.md`). The Unreleased section already lists the three foundation specs (044/045/046) added by prompts 188/189/190. Add ONE new bullet at the TOP of Unreleased.
- `agent/pr-reviewer/cmd/run-task/Makefile` — copy this verbatim as `agent/github-releaser/cmd/run-task/Makefile` (it's a one-target shim that calls `go run`).

Agent-lib types in scope (from `github.com/bborbe/agent/lib v0.63.11`):

- `agentlib.NewAgent(phases ...Phase) *Agent`.
- `agentlib.NewPhase(name domain.TaskPhase, steps ...Step) Phase` — phase name is the TYPED `domain.TaskPhase`, NOT a string.
- `agentlib.NewAgentProvider(name string, agents map[TaskType]*Agent) AgentProvider`.
- `agentlib.TaskType` is a string-derived type. The github-releaser task type literal is `"github-release"` — there is NO `agentlib.TaskTypeGitHubRelease` constant in v0.63.11, so cast it: `agentlib.TaskType("github-release")`. (Spec § Desired Behavior 1: "routing `task_type: github-release`".)
- `agentlib.TaskTypeHealthcheck` constant DOES exist in v0.63.11 — use it for the healthcheck route.
- `healthcheck.NewAgent(step agentlib.Step) *agentlib.Agent` and `healthcheck.NewClaudeStep(runner claudelib.ClaudeRunner) agentlib.Step` from `github.com/bborbe/agent/lib/healthcheck`.
- `delivery.NewKafkaResultDeliverer`, `delivery.NewFileResultDeliverer`, `delivery.NewNoopResultDeliverer`, `delivery.NewPassthroughContentGenerator` from `github.com/bborbe/agent/lib/delivery`.

Coding-plugin guides (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — Create* prefix convention, no error returns, no business logic.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — Unreleased bullet format.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors`.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go build ./...` after step 5 to catch type errors early. Run `cd agent/github-releaser && make precommit` only as the final verification step.**

1. **Create `agent/github-releaser/pkg/factory/factory.go`** with this exact shape (canonical from `agent/pr-reviewer/pkg/factory/factory.go`, stripped to single planning phase):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package factory wires concrete dependencies for the
   // maintainer-agent-github-releaser binary.
   //
   // All factory functions follow the Create* prefix convention and contain
   // zero business logic — they compose constructors with config.
   package factory

   import (
       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"
       delivery "github.com/bborbe/agent/lib/delivery"
       "github.com/bborbe/agent/lib/healthcheck"
       "github.com/bborbe/cqrs/base"
       libkafka "github.com/bborbe/kafka"
       libtime "github.com/bborbe/time"
       domain "github.com/bborbe/vault-cli/pkg/domain"

       releaserpkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
       "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog"
   )

   const serviceName = "maintainer-agent-github-releaser"

   // taskTypeGitHubRelease is the agent-lib TaskType literal for this agent's
   // domain task. No constant exists in agent/lib v0.63.11 for this value, so
   // we cast it locally. Keep the literal exactly "github-release" — the
   // watcher emits this string verbatim and the CRD trigger.task_type field
   // must match.
   var taskTypeGitHubRelease = agentlib.TaskType("github-release")

   // planningTools is the Claude allowed-tools set for the planning phase.
   // Planning is read-only verdict classification — Claude needs no tools to
   // do its job. Tightening to an empty set per spec § Security.
   var planningTools = claudelib.AllowedTools{}

   // CreateClaudeRunner constructs a ClaudeRunner pre-configured with tools,
   // model, working directory, and CLI environment. env is forwarded as-is
   // into the Claude CLI subprocess env (caller builds it, e.g. with GH_TOKEN).
   func CreateClaudeRunner(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       env map[string]string,
       allowedTools claudelib.AllowedTools,
   ) claudelib.ClaudeRunner {
       return claudelib.NewClaudeRunner(claudelib.ClaudeRunnerConfig{
           ClaudeConfigDir:  claudeConfigDir,
           AllowedTools:     allowedTools,
           Model:            model,
           WorkingDirectory: agentDir,
           Env:              env,
       })
   }

   // CreateKafkaResultDeliverer wires the kafka deliverer with the passthrough
   // content generator. Mirrors pr-reviewer.
   func CreateKafkaResultDeliverer(
       syncProducer libkafka.SyncProducer,
       branch base.Branch,
       taskID agentlib.TaskIdentifier,
       originalContent string,
       currentDateTime libtime.CurrentDateTimeGetter,
   ) agentlib.ResultDeliverer {
       return delivery.NewKafkaResultDeliverer(
           syncProducer,
           branch,
           taskID,
           originalContent,
           delivery.NewPassthroughContentGenerator(),
           currentDateTime,
       )
   }

   // CreateFileResultDeliverer creates a deliverer that writes the agent's
   // output back to a markdown file (local CLI mode).
   func CreateFileResultDeliverer(filePath string) agentlib.ResultDeliverer {
       return delivery.NewFileResultDeliverer(
           delivery.NewPassthroughContentGenerator(),
           filePath,
       )
   }

   // CreateAgent assembles the single-phase planning agent.
   //
   // Future specs (047-execution, 047-ai-review) will add the execution and
   // ai_review phases here. For now planning is the only phase wired.
   func CreateAgent(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       ghToken string,
       env map[string]string,
   ) *agentlib.Agent {
       planningRunner := CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools)
       fetcher := githubchangelog.NewHTTPFetcher(ghToken)
       planningStep := releaserpkg.NewPlanningStep(planningRunner, fetcher)
       return agentlib.NewAgent(
           agentlib.NewPhase(domain.TaskPhasePlanning, planningStep),
       )
   }

   // CreateAgentProvider wires the per-task-type dispatch table.
   //   - task_type: github-release → planning agent (CreateAgent)
   //   - task_type: healthcheck    → liveness agent (mirrors pr-reviewer)
   //
   // Pure plumbing; no conditional, no error.
   func CreateAgentProvider(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       ghToken string,
       env map[string]string,
   ) agentlib.AgentProvider {
       domainAgent := CreateAgent(claudeConfigDir, agentDir, model, ghToken, env)
       healthcheckRunner := CreateClaudeRunner(
           claudeConfigDir,
           agentDir,
           model,
           env,
           claudelib.AllowedTools{},
       )
       livenessAgent := healthcheck.NewAgent(healthcheck.NewClaudeStep(healthcheckRunner))
       return agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{
           taskTypeGitHubRelease:        domainAgent,
           agentlib.TaskTypeHealthcheck: livenessAgent,
       })
   }

   // CreateDeliverer builds the Kafka result deliverer used by the Kafka
   // entry point. The caller owns the SyncProducer lifecycle and must close it
   // after the deliverer is no longer needed.
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

   Notes:
   - `agentlib.NewPhase(domain.TaskPhasePlanning, planningStep)` — typed phase constant, NOT the string literal `"planning"`. Per spec § Constraints + AC.
   - Imports order: stdlib block, third-party block, local block — golangci-lint will sort.
   - Factory has NO error returns and NO business logic — pure wiring. Per `go-factory-pattern.md`.

2. **Create `agent/github-releaser/pkg/factory/factory_suite_test.go`** — minimal bootstrap:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package factory_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Factory Suite", suiteConfig, reporterConfig)
   }
   ```

3. **Create `agent/github-releaser/pkg/factory/factory_test.go`** — minimal smoke test:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package factory_test

   import (
       "context"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/factory"
   )

   var _ = Describe("CreateAgentProvider", func() {
       var provider agentlib.AgentProvider

       BeforeEach(func() {
           provider = factory.CreateAgentProvider(
               claudelib.ClaudeConfigDir("/tmp/claude"),
               claudelib.AgentDir("/tmp/agent"),
               claudelib.ClaudeModel("sonnet"),
               "",
               map[string]string{},
           )
       })

       It("routes task_type: github-release", func() {
           a, err := provider.Get(context.Background(), agentlib.TaskType("github-release"))
           Expect(err).NotTo(HaveOccurred())
           Expect(a).NotTo(BeNil())
       })

       It("routes task_type: healthcheck", func() {
           a, err := provider.Get(context.Background(), agentlib.TaskTypeHealthcheck)
           Expect(err).NotTo(HaveOccurred())
           Expect(a).NotTo(BeNil())
       })

       It("returns error for unknown task_type", func() {
           _, err := provider.Get(context.Background(), agentlib.TaskType("not-a-real-type"))
           Expect(err).To(HaveOccurred())
       })
   })
   ```

4. **Rewrite `agent/github-releaser/main.go`** — replace the placeholder Result block (lines 71-95 in the current file) with a real dispatch. Mirror `agent/pr-reviewer/main.go` Run method shape, simplified for github-releaser (no repoAllowlist, no resolveAuth — github-releaser uses PAT-only for now, App auth is a separate spec).

   The full new main.go:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Command maintainer-agent-github-releaser is the Kafka entry point for the
   // github-releaser agent — spawned as a K8s Job by task/executor with
   // TASK_CONTENT + TASK_ID + PHASE + KAFKA_BROKERS env. The agent consumes one
   // release task per Job invocation.
   //
   // Phase 2 graduation of the validated /github-release-repo slash command
   // (Phase 1). See [[GitHub Release Agent Phase 1 Learnings]] for what
   // carries from the prototype.
   //
   // Planning phase wiring per spec 047. Execution + ai_review phases ship in
   // separate specs.
   package main

   import (
       "context"
       "os"
       "time"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"
       delivery "github.com/bborbe/agent/lib/delivery"
       libmetrics "github.com/bborbe/agent/lib/metrics"
       "github.com/bborbe/cqrs/base"
       "github.com/bborbe/errors"
       libkafka "github.com/bborbe/kafka"
       libsentry "github.com/bborbe/sentry"
       "github.com/bborbe/service"
       libtime "github.com/bborbe/time"
       "github.com/bborbe/vault-cli/pkg/domain"
       "github.com/golang/glog"
       "github.com/prometheus/client_golang/prometheus"
       "github.com/prometheus/client_golang/prometheus/push"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/factory"
   )

   const agentName = "github-releaser-agent"

   func main() {
       app := &application{}
       os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
   }

   type application struct {
       SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
       SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

       // Claude Code CLI configuration
       ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory" default:"~/.claude"`
       AgentDir        claudelib.AgentDir        `required:"false" arg:"agent-dir" env:"AGENT_DIR" usage:"Agent directory with .claude/ config" default:"agent"`

       // Anthropic-compatible provider routing.
       AnthropicBaseURL   string                `required:"false" arg:"anthropic-base-url"   env:"ANTHROPIC_BASE_URL"   usage:"Anthropic-compatible API base URL"`
       AnthropicAuthToken string                `required:"false" arg:"anthropic-auth-token" env:"ANTHROPIC_AUTH_TOKEN" usage:"Bearer token for ANTHROPIC_BASE_URL" display:"length"`
       AnthropicModel     claudelib.ClaudeModel `required:"false" arg:"anthropic-model"      env:"ANTHROPIC_MODEL"      usage:"Model name; also exposed to the claude subprocess as ANTHROPIC_MODEL" default:"sonnet"`

       // Task content from agent pipeline (raw markdown injected by task/executor).
       TaskContent string `required:"true" arg:"task-content" env:"TASK_CONTENT" usage:"Raw release task markdown"`

       // Branch for Kafka result delivery (dev / prod).
       Branch base.Branch `required:"true" arg:"branch" env:"BRANCH" usage:"branch"`

       // Phase to run (planning | execution | ai_review). Canonical values; CRD literal match.
       Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"planning"`

       // Kafka delivery (optional — only active when TASK_ID is set).
       KafkaBrokers libkafka.Brokers        `required:"false" arg:"kafka-brokers" env:"KAFKA_BROKERS" usage:"Comma separated list of Kafka brokers"`
       TaskID       agentlib.TaskIdentifier `required:"false" arg:"task-id"       env:"TASK_ID"       usage:"Agent task identifier for publishing results back to task controller"`

       // GitHub token for the planning fetcher (PAT for now; App auth in a follow-up spec).
       GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub PAT for CHANGELOG fetch" display:"length"`

       PushgatewayURL string `required:"false" arg:"pushgateway-url" env:"PUSHGATEWAY_URL" usage:"Prometheus PushGateway URL" default:"http://pushgateway:9090"`
       TaskType       string `required:"false" arg:"task-type"       env:"TASK_TYPE"       usage:"Task type label for metric grouping" default:"unknown"`
   }

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
       glog.V(2).Infof("%s started phase=%s", agentName, a.Phase)

       deliverer, cleanup, err := a.createDeliverer(ctx)
       if err != nil {
           jobMetrics.RecordRun(agentlib.AgentStatusFailed)
           jobMetrics.RecordDuration(time.Since(start))
           return err
       }
       defer cleanup()

       env := a.buildEnv()
       provider := factory.CreateAgentProvider(
           a.ClaudeConfigDir,
           a.AgentDir,
           a.AnthropicModel,
           a.GHToken,
           env,
       )
       agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))
       if err != nil {
           jobMetrics.RecordRun(agentlib.AgentStatusFailed)
           jobMetrics.RecordDuration(time.Since(start))
           return errors.Wrap(ctx, err, "select agent for task_type")
       }

       result, err := agent.Run(ctx, a.Phase, a.TaskContent, deliverer)
       if err != nil {
           jobMetrics.RecordRun(agentlib.AgentStatusFailed)
           jobMetrics.RecordDuration(time.Since(start))
           return errors.Wrap(ctx, err, "agent run failed")
       }
       jobMetrics.RecordRun(result.Status)
       jobMetrics.RecordDuration(time.Since(start))
       return agentlib.PrintResult(ctx, result)
   }

   // buildEnv assembles the env map forwarded into the Claude CLI subprocess.
   // Only non-empty values are set so the subprocess sees a clean env.
   func (a *application) buildEnv() map[string]string {
       env := map[string]string{}
       if a.GHToken != "" {
           env["GH_TOKEN"] = a.GHToken
       }
       if a.AnthropicBaseURL != "" {
           env["ANTHROPIC_BASE_URL"] = a.AnthropicBaseURL
       }
       if a.AnthropicAuthToken != "" {
           env["ANTHROPIC_AUTH_TOKEN"] = a.AnthropicAuthToken
       }
       if a.AnthropicModel != "" {
           env["ANTHROPIC_MODEL"] = a.AnthropicModel.String()
       }
       return env
   }

   // createDeliverer builds the Kafka deliverer when TASK_ID is set,
   // otherwise returns the noop deliverer (for local-pod debugging without
   // Kafka).
   func (a *application) createDeliverer(
       ctx context.Context,
   ) (agentlib.ResultDeliverer, func(), error) {
       if a.TaskID == "" {
           glog.V(2).Infof("TASK_ID not set, skipping task result publishing")
           return delivery.NewNoopResultDeliverer(), func() {}, nil
       }
       if len(a.KafkaBrokers) == 0 {
           return nil, nil, errors.Errorf(ctx, "KAFKA_BROKERS must be set when TASK_ID is set")
       }
       syncProducer, err := libkafka.NewSyncProducerWithName(ctx, a.KafkaBrokers, "agent-github-releaser")
       if err != nil {
           return nil, nil, errors.Wrap(ctx, err, "create kafka sync producer")
       }
       cleanup := func() {
           if err := syncProducer.Close(); err != nil {
               glog.Warningf("close sync producer failed: %v", err)
           }
       }
       currentDateTime := libtime.NewCurrentDateTime()
       deliverer := factory.CreateDeliverer(
           syncProducer,
           a.TaskID,
           a.Branch,
           a.TaskContent,
           currentDateTime,
       )
       return deliverer, cleanup, nil
   }
   ```

   Notes:
   - Default Phase is now `"planning"` (not `"execution"` as in the old placeholder) — github-releaser starts each task in planning.
   - `agentlib.TaskType(a.TaskType)` cast — `a.TaskType` is a plain string from the config struct; the framework's typed `TaskType` requires the cast.
   - `errors.Wrap` / `errors.Errorf` from `github.com/bborbe/errors`. No `fmt.Errorf`.
   - Mirror the prometheus block from `agent/pr-reviewer/main.go` lines 105-120 verbatim.

5. **Create `agent/github-releaser/cmd/run-task/main.go`** — local file-driven CLI. Strip pr-reviewer-specific fields (no repo cache, no allowlist, no App auth, no review-mode):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Command run-task is the local-CLI entry point for
   // maintainer-agent-github-releaser.
   //
   // Reads a markdown task file from disk, runs the agent against it, and
   // writes the updated content back to the same file. Mirrors the Kafka
   // entry point (../../main.go) but uses file I/O instead of Kafka/CQRS.
   package main

   import (
       "context"
       "os"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"
       "github.com/bborbe/errors"
       libsentry "github.com/bborbe/sentry"
       "github.com/bborbe/service"
       "github.com/bborbe/vault-cli/pkg/domain"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/factory"
   )

   func main() {
       app := &application{}
       os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
   }

   type application struct {
       SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
       SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

       ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory" default:"~/.claude"`
       AgentDir        claudelib.AgentDir        `required:"false" arg:"agent-dir" env:"AGENT_DIR" usage:"Agent directory with .claude/ config" default:"agent"`

       Phase    domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"planning"`
       TaskType string           `required:"false" arg:"task-type" env:"TASK_TYPE" usage:"Task type for provider dispatch" default:"github-release"`

       TaskFilePath string `required:"true" arg:"task-file" env:"TASK_FILE" usage:"Path to the markdown task file"`

       GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub PAT for CHANGELOG fetch" display:"length"`

       AnthropicBaseURL   string                `required:"false" arg:"anthropic-base-url"   env:"ANTHROPIC_BASE_URL"   usage:"Anthropic-compatible API base URL"`
       AnthropicAuthToken string                `required:"false" arg:"anthropic-auth-token" env:"ANTHROPIC_AUTH_TOKEN" usage:"Bearer token for ANTHROPIC_BASE_URL" display:"length"`
       AnthropicModel     claudelib.ClaudeModel `required:"false" arg:"anthropic-model"      env:"ANTHROPIC_MODEL"      usage:"Model name" default:"sonnet"`
   }

   func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
       taskContent, err := os.ReadFile(
           a.TaskFilePath,
       ) // #nosec G304 -- filePath from trusted CLI input
       if err != nil {
           return errors.Wrapf(ctx, err, "read task file: %s", a.TaskFilePath)
       }

       deliverer := factory.CreateFileResultDeliverer(a.TaskFilePath)

       env := map[string]string{}
       if a.GHToken != "" {
           env["GH_TOKEN"] = a.GHToken
       }
       if a.AnthropicBaseURL != "" {
           env["ANTHROPIC_BASE_URL"] = a.AnthropicBaseURL
       }
       if a.AnthropicAuthToken != "" {
           env["ANTHROPIC_AUTH_TOKEN"] = a.AnthropicAuthToken
       }
       if a.AnthropicModel != "" {
           env["ANTHROPIC_MODEL"] = a.AnthropicModel.String()
       }

       provider := factory.CreateAgentProvider(
           a.ClaudeConfigDir,
           a.AgentDir,
           a.AnthropicModel,
           a.GHToken,
           env,
       )
       agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))
       if err != nil {
           return errors.Wrap(ctx, err, "select agent for task_type")
       }
       result, err := agent.Run(ctx, a.Phase, string(taskContent), deliverer)
       if err != nil {
           return errors.Wrap(ctx, err, "agent run failed")
       }
       return agentlib.PrintResult(ctx, result)
   }
   ```

6. **Create `agent/github-releaser/cmd/run-task/Makefile`** — copy verbatim from `agent/pr-reviewer/cmd/run-task/Makefile` (read it first, then write the same content into the new file). It is a tiny Makefile that wraps `go run`.

7. **Create `agent/github-releaser/cmd/run-task/main_test.go`** — minimal compile test (mirrors `agent/pr-reviewer/cmd/run-task/main_test.go` shape):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package main_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
       "github.com/onsi/gomega/gexec"
   )

   var _ = Describe("run-task", func() {
       It("Compiles", func() {
           _, err := gexec.Build("github.com/bborbe/maintainer/agent/github-releaser/cmd/run-task", "-mod=mod")
           Expect(err).NotTo(HaveOccurred())
       })
   })

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "run-task Suite", suiteConfig, reporterConfig)
   }
   ```

8. **Create test fixture files** under `agent/github-releaser/cmd/run-task/testdata/`:

   File 1: `agent/github-releaser/cmd/run-task/testdata/happy-planning.md`

   ```markdown
   ---
   status: in_progress
   phase: planning
   assignee: github-releaser-agent
   task_type: github-release
   repo: bborbe/example
   clone_url: https://github.com/bborbe/example.git
   ref: master
   current_version: v1.7.7
   task_identifier: gh-release-bborbe-example-master-001
   ---

   # Release task — happy path fixture

   Drives the planning phase against a CHANGELOG with `## Unreleased` first and one bullet.
   ```

   File 2: `agent/github-releaser/cmd/run-task/testdata/unreleased-at-bottom.md`

   ```markdown
   ---
   status: in_progress
   phase: planning
   assignee: github-releaser-agent
   task_type: github-release
   repo: bborbe/example
   clone_url: https://github.com/bborbe/example.git
   ref: branch-with-unreleased-at-bottom
   current_version: v1.2.6
   task_identifier: gh-release-bborbe-example-bad-001
   ---

   # Release task — P1 escalation fixture

   Drives the planning phase against a CHANGELOG where `## Unreleased` is NOT the first `##` heading. Expected outcome: `## Plan` with `outcome: needs_input`, `assignee` cleared, `previous_assignee: github-releaser-agent`.
   ```

   These fixtures are checked-in for documentation / manual smoke testing per spec § Constraints. They are NOT executed by `make test` — the integration tests in `pkg/steps_planning_test.go` (prompt 2) cover the same scenarios with mocks.

9. **Update root `CHANGELOG.md`** Unreleased section. The current top of `## Unreleased` reads:

   ```
   ## Unreleased

   - feat(agent/github-releaser): add pkg/prompts with embedded bump-classification prompt and ParseBumpVerdict parser for the planning step (spec 046)
   - feat(agent/github-releaser): add pkg/semver with BumpVersion(current, bump) for Phase 1 → Phase 2 version arithmetic (spec 045)
   ...
   ```

   ADD exactly ONE new bullet at the TOP of the Unreleased block so the new state begins:

   ```
   ## Unreleased

   - feat(agent/github-releaser): wire planning phase end-to-end — adds pkg/githubchangelog fetcher, pkg/steps_planning PlanningStep, pkg/factory wiring, main.go dispatch, and cmd/run-task local CLI (spec 047)
   - feat(agent/github-releaser): add pkg/prompts ...
   ...
   ```

   The new bullet MUST contain the literal substring `planning phase` (acceptance-criteria grep target from spec 047 AC: `grep -c 'planning phase' CHANGELOG.md` ≥ 1).

10. **Final verification**: from `agent/github-releaser/`:

    ```bash
    cd agent/github-releaser && make precommit
    ```

    Must exit 0. The placeholder Result block from the original `main.go` MUST be gone — confirm with `grep -c 'skeleton — phase pipeline not implemented' agent/github-releaser/main.go` returning 0.

</requirements>

<constraints>
- Module path: `github.com/bborbe/maintainer/agent/github-releaser`. New files:
  - `agent/github-releaser/pkg/factory/factory.go`
  - `agent/github-releaser/pkg/factory/factory_test.go`
  - `agent/github-releaser/pkg/factory/factory_suite_test.go`
  - `agent/github-releaser/cmd/run-task/main.go`
  - `agent/github-releaser/cmd/run-task/main_test.go`
  - `agent/github-releaser/cmd/run-task/Makefile`
  - `agent/github-releaser/cmd/run-task/testdata/happy-planning.md`
  - `agent/github-releaser/cmd/run-task/testdata/unreleased-at-bottom.md`
- Modified files:
  - `agent/github-releaser/main.go` (rewrite Run method + buildEnv + createDeliverer helpers; struct fields adjusted)
  - `CHANGELOG.md` at repo root (one new Unreleased bullet at top)
- Frozen Create* signatures per spec § Goal (the FOUR canonical factories must exist and be exported):
  - `func CreateAgent(claudeConfigDir, agentDir, model, ghToken, env) *agentlib.Agent`
  - `func CreateAgentProvider(claudeConfigDir, agentDir, model, ghToken, env) agentlib.AgentProvider`
  - `func CreateClaudeRunner(claudeConfigDir, agentDir, model, env, allowedTools) claudelib.ClaudeRunner`
  - `func CreateKafkaResultDeliverer(syncProducer, branch, taskID, originalContent, currentDateTime) agentlib.ResultDeliverer`
- Plus `CreateFileResultDeliverer` and `CreateDeliverer` for parity with pr-reviewer.
- `agentlib.NewPhase(domain.TaskPhasePlanning, ...)` — typed phase constant, NEVER the string `"planning"`. Per AC: `grep -c 'agentlib.NewPhase(domain.TaskPhasePlanning' pkg/factory/factory.go` ≥ 1.
- AC grep: `grep -c '"planning"' pkg/factory/factory.go pkg/steps_planning.go` must be 0 — no raw planning literal in factory or step logic. `main.go` + `cmd/run-task/main.go` are exempted by spec (AC #4 amended 2026-05-28 to drop main.go from grep, citing libargument struct-tag `default:"planning"` constraint — the value MUST be a string literal because libargument doesn't accept typed constants). The struct-tag default is the only allowed `"planning"` literal in main.go entry points.
- Errors via `github.com/bborbe/errors` (`Wrap`/`Wrapf`/`Errorf`). No `fmt.Errorf`.
- Healthcheck routing: `agentlib.TaskTypeHealthcheck` → liveness agent (mirror pr-reviewer; no test exercises Kafka or healthcheck per spec § Non-goals).
- `taskTypeGitHubRelease = agentlib.TaskType("github-release")` — local var because agent/lib v0.63.11 has no constant. Keep the literal exactly `"github-release"` (watcher emits this verbatim).
- `planningTools = claudelib.AllowedTools{}` — empty, per spec § Security (planning is read-only verdict classification; no tools needed).
- Kafka delivery wired identically to pr-reviewer's pattern but not tested per spec § Non-goals.
- License header (3 lines) at the top of every `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before AND after (including the prompt 2 integration tests).
- `make precommit` invocations run from `agent/github-releaser/`, never the repo root (the root has its own precommit target for the watcher/etc.).
</constraints>

<verification>

Run from the repo root unless noted.

```bash
# Build + tests pass
cd agent/github-releaser && make precommit                              # exit 0
cd agent/github-releaser && go build ./...                              # exit 0
cd agent/github-releaser && go test ./...                               # all green

# Files exist
ls agent/github-releaser/pkg/factory/factory.go                          # exists
ls agent/github-releaser/pkg/factory/factory_test.go                     # exists
ls agent/github-releaser/cmd/run-task/main.go                            # exists
ls agent/github-releaser/cmd/run-task/Makefile                           # exists
ls agent/github-releaser/cmd/run-task/testdata/happy-planning.md         # exists
ls agent/github-releaser/cmd/run-task/testdata/unreleased-at-bottom.md   # exists

# Canonical factories present (per spec 047 AC)
grep -cE '^func Create(Agent|AgentProvider|ClaudeRunner|Deliverer|KafkaResultDeliverer|FileResultDeliverer)' agent/github-releaser/pkg/factory/factory.go   # ≥ 3 (the AC requires ≥3 of the four canonical ones; we have 6 total)

# Typed phase constant used (per spec 047 AC)
grep -c 'agentlib.NewPhase(domain.TaskPhasePlanning' agent/github-releaser/pkg/factory/factory.go   # ≥ 1

# No raw "planning"/"execution" string literals in the WIRING + STEP production code
# (main.go and cmd/run-task/main.go are exempt: struct-tag `default:"planning"` is unavoidable)
grep -cE '"(planning|execution)"' agent/github-releaser/pkg/factory/factory.go     # = 0
grep -cE '"(planning|execution)"' agent/github-releaser/pkg/steps_planning.go      # = 0

# Error-wrapping convention in production files touched by this prompt
grep -c 'fmt.Errorf' agent/github-releaser/pkg/factory/factory.go                  # = 0
grep -c 'fmt.Errorf' agent/github-releaser/main.go                                 # = 0
grep -c 'fmt.Errorf' agent/github-releaser/cmd/run-task/main.go                    # = 0

# Placeholder gone from main.go
grep -c 'skeleton — phase pipeline not implemented' agent/github-releaser/main.go   # = 0
grep -c 'factory.CreateAgentProvider' agent/github-releaser/main.go                 # ≥ 1

# Root CHANGELOG bullet present (spec 047 AC final row)
grep -c 'planning phase' CHANGELOG.md                                              # ≥ 1

# Acceptance-criteria-aligned final integration check from spec 047:
# - all three artifact files exist
ls agent/github-releaser/pkg/factory/factory.go agent/github-releaser/pkg/steps_planning.go agent/github-releaser/pkg/githubchangelog/fetcher.go

# Full coverage / spec verifier walk
cd agent/github-releaser && go test -cover ./pkg/...
```

</verification>
