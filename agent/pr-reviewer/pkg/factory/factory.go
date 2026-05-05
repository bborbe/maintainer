// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package factory wires concrete dependencies for the agent-pr-reviewer binary.
//
// All factory functions follow the Create* prefix convention and contain
// zero business logic — they compose constructors with config.
package factory

import (
	"context"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	delivery "github.com/bborbe/agent/lib/delivery"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/prompts"
)

const serviceName = "agent-pr-reviewer"

// AgentRunner is the minimal interface satisfied by *agentlib.Agent.
type AgentRunner interface {
	Run(
		ctx context.Context,
		phaseName domain.TaskPhase,
		taskContent string,
		deliverer agentlib.ResultDeliverer,
	) (*agentlib.Result, error)
}

// Per-phase tool scopes. Principle: each phase gets the smallest set that
// lets it do its job. Planning + Review are read-only inspection. Execution
// gets broader git/gh access for cross-file reads but still cannot post
// (no `gh pr comment` / `gh pr review`) — posting happens out-of-band
// after the human approves the verdict.
var (
	planningTools = claudelib.AllowedTools{
		"Read", "Grep", "Glob",
		"Bash(git diff:*)", "Bash(git log:*)", "Bash(git show:*)",
		"Bash(gh pr view:*)", "Bash(gh pr diff:*)", "Bash(gh pr list:*)",
	}
	// Sub-agent allowlist audit (spec-011 + inline-plugin fix): the inlined
	// /coding:pr-review content (assembled per-task by the execution step)
	// dispatches specialist sub-agents via Task. Each coding:* sub-agent
	// declares its own read-only tools (Read, Grep, Glob, restricted Bash;
	// no Write, Edit, curl, wget, nc). Verified by inspecting plugin agent
	// definitions under
	// ~/.claude/plugins/marketplaces/coding/agents/*.md. The tool set below
	// mirrors the plugin's `allowed-tools` frontmatter so the inlined body
	// has the same capabilities as a real /coding:pr-review invocation.
	executionTools = claudelib.AllowedTools{
		"Task",
		"Bash(git diff:*)",
		"Bash(git log:*)",
		"Bash(git status:*)",
		"Bash(git ls-files:*)",
		"Bash(git fetch:*)",
		"Bash(git worktree:*)",
		"Bash(git branch:*)",
		"Bash(rm -rf:*)",
	}
	reviewTools = claudelib.AllowedTools{
		"Read", "Grep",
		"Bash(gh pr view:*)", "Bash(gh pr diff:*)",
	}
)

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

// CreateSyncProducer creates a Kafka sync producer.
func CreateSyncProducer(
	ctx context.Context,
	brokers libkafka.Brokers,
) (libkafka.SyncProducer, error) {
	producer, err := libkafka.NewSyncProducerWithName(ctx, brokers, serviceName)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "create sync producer failed")
	}
	return producer, nil
}

// CreateKafkaResultDeliverer creates a ResultDeliverer that publishes task
// updates to Kafka via CQRS commands. Uses the passthrough content generator
// — the agent framework's StepRunner already produces the full marshaled
// task in result.Output; the deliverer publishes it as-is.
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

// CreateFileResultDeliverer creates a ResultDeliverer that writes the agent's
// output back to a markdown file (local CLI mode).
func CreateFileResultDeliverer(filePath string) agentlib.ResultDeliverer {
	return delivery.NewFileResultDeliverer(
		delivery.NewPassthroughContentGenerator(),
		filePath,
	)
}

// CreateAgent assembles the full 3-phase pr-reviewer agent with per-phase
// tool scopes and per-phase prompts:
//
//   - planning: read-only diff inspection → ## Plan (JSON)
//   - in_progress: read + cross-file inspection → ## Review (JSON)
//   - ai_review: minimal read-only fresh-context verifier → ## Verdict (JSON);
//     verdict=pass → done, otherwise → human_review
func CreateAgent(
	claudeConfigDir claudelib.ClaudeConfigDir,
	agentDir claudelib.AgentDir,
	model claudelib.ClaudeModel,
	ghToken string,
	env map[string]string,
	repoManager git.RepoManager,
	reviewMode string,
	repoAllowlist []string,
) AgentRunner {
	tokenCheck := prpkg.NewGHTokenCheckStep(ghToken)
	planningStep := claudelib.NewAgentStep(claudelib.AgentStepConfig{
		Name:          "pr-plan",
		Runner:        CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools),
		Instructions:  prompts.BuildPlanningInstructions(),
		OutputSection: "## Plan",
		NextPhase:     "in_progress",
	})
	executionStep := prpkg.NewCheckoutExecutionStep(
		repoManager,
		claudeConfigDir,
		agentDir,
		model,
		env,
		executionTools,
		reviewMode,
		repoAllowlist,
	)
	reviewStep := prpkg.NewReviewStep(
		CreateClaudeRunner(claudeConfigDir, agentDir, model, env, reviewTools),
		prompts.BuildReviewInstructions(),
	)
	return agentlib.NewAgent(
		agentlib.NewPhase("planning", tokenCheck, planningStep),
		agentlib.NewPhase("in_progress", tokenCheck, executionStep),
		agentlib.NewPhase("ai_review", tokenCheck, reviewStep),
	)
}

// CreateDeliverer builds the Kafka result deliverer used by the Kafka
// entry point. Requires non-empty taskID and brokers — callers must
// guard these preconditions before calling.
func CreateDeliverer(
	ctx context.Context,
	taskID agentlib.TaskIdentifier,
	brokers libkafka.Brokers,
	branch base.Branch,
	originalContent string,
	currentDateTime libtime.CurrentDateTimeGetter,
) (agentlib.ResultDeliverer, func(), error) {
	syncProducer, err := CreateSyncProducer(ctx, brokers)
	if err != nil {
		return nil, nil, errors.Wrap(ctx, err, "create sync producer failed")
	}
	deliverer := CreateKafkaResultDeliverer(
		syncProducer,
		branch,
		taskID,
		originalContent,
		currentDateTime,
	)
	cleanup := func() {
		if err := syncProducer.Close(); err != nil {
			glog.Warningf("close sync producer failed: %v", err)
		}
	}
	return deliverer, cleanup, nil
}
