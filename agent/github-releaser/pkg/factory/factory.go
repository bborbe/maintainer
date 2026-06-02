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
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubreview"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/maintainerconfig"
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

// CreateGitOps returns the production GitOps implementation, wired with
// the Phase 1 verbatim bot identity. Pure plumbing.
func CreateGitOps() git.GitOps {
	return git.NewOSExecGitOps()
}

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

// aiReviewTools is the Claude allowed-tools set for the ai-review phase.
// Like the planning phase, ai-review is read-only verdict classification —
// the LLM needs no tools. Mirrors planningTools for consistency.
var aiReviewTools = claudelib.AllowedTools{}

// CreateAgent assembles the planning + execution + ai_review agent.
//
// The three phases advance in order: planning writes ## Plan(outcome=ready),
// execution reads it, clones the target repo, rewrites ## Unreleased,
// commits + tags + pushes, and writes ## Result(outcome=released),
// then ai_review verifies the tag and CHANGELOG.md header and drives
// terminal completion or escalation via ## Review.
func CreateAgent(
	claudeConfigDir claudelib.ClaudeConfigDir,
	agentDir claudelib.AgentDir,
	model claudelib.ClaudeModel,
	ghToken string,
	env map[string]string,
) *agentlib.Agent {
	planningRunner := CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools)
	fetcher := githubchangelog.NewHTTPFetcher(ghToken)
	maintainerConfigFetcher := maintainerconfig.NewHTTPFetcher(ghToken)
	planningStep := releaserpkg.NewPlanningStep(planningRunner, fetcher, maintainerConfigFetcher)

	executionOps := CreateGitOps()
	executionStep := releaserpkg.NewExecutionStep(executionOps, ghToken)

	reviewClient := githubreview.NewHTTPClient(ghToken)
	// The ai-review LLM is read-only — same tool policy as planning.
	aiReviewRunner := CreateClaudeRunner(
		claudeConfigDir,
		agentDir,
		model,
		env,
		aiReviewTools,
	)
	// Reuse the same GitOps seam as the execution step so the push of
	// the local commit + tag goes out via the same authenticated path.
	reviewStep := releaserpkg.NewAIReviewStep(
		reviewClient,
		aiReviewRunner,
		executionOps,
		ghToken,
	)

	return agentlib.NewAgent(
		agentlib.NewPhase(domain.TaskPhasePlanning, planningStep),
		agentlib.NewPhase(domain.TaskPhaseExecution, executionStep),
		agentlib.NewPhase(domain.TaskPhaseAIReview, reviewStep),
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
