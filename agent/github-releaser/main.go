// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command maintainer-agent-github-releaser is the Kafka entry point for the
// github-releaser agent — spawned as a K8s Job by task/executor with
// TASK_CONTENT + TASK_ID + PHASE + KAFKA_BROKERS env. The agent consumes one
// release task: clones the target repo at `ref`, parses `## Unreleased` from
// CHANGELOG.md, asks Claude to classify the bump (patch / minor / major),
// rewrites the header to `## vX.Y.Z`, commits, tags, pushes. Branch protection
// is handled via PR + auto-merge fallback.
//
// Phase 2 graduation of the validated /github-release-repo slash command (Phase 1).
// See [[GitHub Release Agent Phase 1 Learnings]] for what carries from prototype.
//
// SKELETON — Milestone 1: compiles + make precommit green. Real phase logic
// builds out incrementally in Milestones 2-6.
package main

import (
	"context"
	"os"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	libmetrics "github.com/bborbe/agent/lib/metrics"
	"github.com/bborbe/cqrs/base"
	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

const agentName = "github-releaser-agent"

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	// Task content from agent pipeline (raw markdown injected by task/executor).
	TaskContent string `required:"true" arg:"task-content" env:"TASK_CONTENT" usage:"Raw release task markdown"`

	// Branch for Kafka result delivery (dev / prod).
	Branch base.Branch `required:"true" arg:"branch" env:"BRANCH" usage:"branch"`

	// Phase to run (planning | execution | ai_review). Canonical values; CRD literal match.
	Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"execution"`

	// Kafka delivery (optional — only active when TASK_ID is set).
	KafkaBrokers libkafka.Brokers        `required:"false" arg:"kafka-brokers" env:"KAFKA_BROKERS" usage:"Comma separated list of Kafka brokers"`
	TaskID       agentlib.TaskIdentifier `required:"false" arg:"task-id"       env:"TASK_ID"       usage:"Agent task identifier for publishing results back to task controller"`

	PushgatewayURL string `required:"false" arg:"pushgateway-url" env:"PUSHGATEWAY_URL" usage:"Prometheus PushGateway URL"          default:"http://pushgateway:9090"`
	TaskType       string `required:"false" arg:"task-type"       env:"TASK_TYPE"       usage:"Task type label for metric grouping" default:"unknown"`
}

// Run executes the agent for the configured phase.
//
// SKELETON: returns a placeholder result. Real phase pipeline (clone → parse →
// classify → rewrite → commit → push or PR fallback → verify) builds out in
// follow-up commits per [[Go Agent Implementation Guide]].
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

	result := &agentlib.Result{
		Status:  agentlib.AgentStatusDone,
		Message: "github-releaser skeleton — phase pipeline not implemented yet",
	}
	jobMetrics.RecordRun(result.Status)
	jobMetrics.RecordDuration(time.Since(start))
	return agentlib.PrintResult(ctx, result)
}
