// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command agent-pr-reviewer is the Kafka entry point for the PR-review
// agent — spawned as a K8s Job by task/executor with TASK_CONTENT +
// TASK_ID + PHASE + KAFKA_BROKERS env. For local CLI mode (file-based),
// see cmd/run-task/main.go.
package main

import (
	"context"
	"os"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	delivery "github.com/bborbe/agent/lib/delivery"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/factory"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubauth"
)

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	// Claude Code CLI configuration
	ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory" default:"~/.claude"`

	// Agent directory (contains .claude/ with CLAUDE.md and commands)
	AgentDir claudelib.AgentDir `required:"false" arg:"agent-dir" env:"AGENT_DIR" usage:"Agent directory with .claude/ config" default:"agent"`

	// Model selection
	Model claudelib.ClaudeModel `required:"false" arg:"model" env:"MODEL" usage:"Claude model to use (sonnet, opus)" default:"sonnet"`

	// Workdir paths for bare-clone cache and per-task worktrees
	ReposPath string `required:"false" arg:"repos-path" env:"REPOS_PATH" usage:"Root path for bare-clone cache"   default:"/repos"`
	WorkPath  string `required:"false" arg:"work-path"  env:"WORK_PATH"  usage:"Root path for per-task worktrees" default:"/work"`

	// Review depth passed to /coding:pr-review (short | standard | full)
	ReviewMode string `required:"false" arg:"review-mode" env:"REVIEW_MODE" usage:"Review depth: short | standard | full" default:"standard"`

	// Task content from agent pipeline
	TaskContent string `required:"true" arg:"task-content" env:"TASK_CONTENT" usage:"Raw task markdown from vault"`

	// Branch for Kafka result delivery
	Branch base.Branch `required:"true" arg:"branch" env:"BRANCH" usage:"branch"`

	// Phase to run (framework requires explicit phase)
	Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | in_progress | ai_review" default:"in_progress"`

	// Kafka delivery (optional — only active when TASK_ID is set)
	KafkaBrokers libkafka.Brokers        `required:"false" arg:"kafka-brokers" env:"KAFKA_BROKERS" usage:"Comma separated list of Kafka brokers"`
	TaskID       agentlib.TaskIdentifier `required:"false" arg:"task-id"       env:"TASK_ID"       usage:"Agent task identifier for publishing results back to task controller"`

	// GitHub token forwarded to the Claude CLI subprocess as GH_TOKEN for gh auth.
	// Also used by the real GitHubAuthSetup to configure git credential helper at pod startup.
	GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub token for gh CLI auth and git credential helper at pod startup" display:"length"`

	// Repo allowlist — comma-separated host/owner/repo entries; empty means allow-all.
	RepoAllowlist string `required:"false" arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); empty means allow-all"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	glog.V(2).Infof("agent-pr-reviewer started phase=%s", a.Phase)

	repoAllowlist, err := prpkg.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return err
	}
	glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))

	deliverer, cleanup, err := a.createDeliverer(ctx)
	if err != nil {
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
		return errors.Wrap(ctx, err, "agent run failed")
	}
	return agentlib.PrintResult(result)
}

// createDeliverer builds the Kafka result deliverer when TASK_ID is set,
// otherwise returns a noop deliverer (for local-pod debugging without Kafka).
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
	currentDateTime := libtime.NewCurrentDateTime()
	deliverer, cleanup, err := factory.CreateDeliverer(
		ctx,
		a.TaskID,
		a.KafkaBrokers,
		a.Branch,
		a.TaskContent,
		currentDateTime,
	)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create deliverer")
	}
	return deliverer, cleanup, nil
}
