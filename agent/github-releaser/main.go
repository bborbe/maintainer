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
	ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory"         default:"~/.claude"`
	AgentDir        claudelib.AgentDir        `required:"false" arg:"agent-dir"         env:"AGENT_DIR"         usage:"Agent directory with .claude/ config" default:"agent"`

	// Anthropic-compatible provider routing.
	AnthropicBaseURL   string                `required:"false" arg:"anthropic-base-url"   env:"ANTHROPIC_BASE_URL"   usage:"Anthropic-compatible API base URL"`
	AnthropicAuthToken string                `required:"false" arg:"anthropic-auth-token" env:"ANTHROPIC_AUTH_TOKEN" usage:"Bearer token for ANTHROPIC_BASE_URL"                                  display:"length"`
	AnthropicModel     claudelib.ClaudeModel `required:"false" arg:"anthropic-model"      env:"ANTHROPIC_MODEL"      usage:"Model name; also exposed to the claude subprocess as ANTHROPIC_MODEL"                  default:"sonnet"`

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

	PushgatewayURL string `required:"false" arg:"pushgateway-url" env:"PUSHGATEWAY_URL" usage:"Prometheus PushGateway URL"          default:"http://pushgateway:9090"`
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
	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		a.KafkaBrokers,
		"agent-github-releaser",
	)
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
