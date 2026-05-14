// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"context"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	"github.com/bborbe/errors"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubauth"
)

// RunConfig is the input to RunAgent — everything the orchestrator needs
// regardless of how the task is read or where the result is delivered.
type RunConfig struct {
	ClaudeConfigDir claudelib.ClaudeConfigDir
	AgentDir        claudelib.AgentDir
	Model           claudelib.ClaudeModel
	GHToken         string
	ReposPath       string
	WorkPath        string
	ReviewMode      string
	RepoAllowlist   []string                // host-qualified repos the agent may clone
	AuthSetup       githubauth.Configurator // pod: real gh-auth-setup; local-CLI: noop
	Phase           domain.TaskPhase
	TaskContent     string
	Deliverer       agentlib.ResultDeliverer
	// Agent overrides the agent used for execution. If nil, CreateAgent is called.
	// Set by main.go after dispatching via CreateAgentProvider. cmd/run-task leaves
	// this nil so CreateAgent is used for backward compatibility.
	Agent *agentlib.Agent
}

// RunAgent performs the shared startup + execution flow for the maintainer-agent-pr-reviewer binary.
// Both entry points (Kafka pod main.go and local CLI cmd/run-task) call this after
// resolving their I/O specifics — task content source and result deliverer.
//
// Steps performed in order:
//  1. Prune stale worktrees from any prior run
//  2. Ensure the bborbe/coding plugin is installed in CLAUDE_CONFIG_DIR
//     (defense-in-depth: pod boot would otherwise rely on an external installer
//     and local CLI runs would silently degrade reviews)
//  3. Build the per-phase agent
//  4. Run the requested phase against the supplied task content
func RunAgent(ctx context.Context, cfg RunConfig) (*agentlib.Result, error) {
	workdirCfg := git.WorkdirConfig{ReposPath: cfg.ReposPath, WorkPath: cfg.WorkPath}
	repoManager := git.NewRepoManager(workdirCfg)
	if err := repoManager.PruneAllWorktrees(ctx); err != nil {
		glog.Warningf("startup worktree prune: %v", err)
	}

	installer := claudelib.NewPluginInstaller(claudelib.NewExecPluginCommander())
	if err := installer.EnsureInstalled(ctx, []claudelib.PluginSpec{
		{Marketplace: "bborbe/coding", Name: "coding"},
	}); err != nil {
		return nil, errors.Wrap(ctx, err, "ensure plugins installed")
	}

	if err := cfg.AuthSetup.Setup(ctx); err != nil {
		return nil, errors.Wrap(ctx, err, "github auth setup failed")
	}

	env := map[string]string{}
	if cfg.GHToken != "" {
		env["GH_TOKEN"] = cfg.GHToken
	}

	agent := cfg.Agent
	if agent == nil {
		agent = CreateAgent(
			cfg.ClaudeConfigDir,
			cfg.AgentDir,
			cfg.Model,
			cfg.GHToken,
			env,
			repoManager,
			cfg.ReviewMode,
			cfg.RepoAllowlist,
		)
	}
	return agent.Run(ctx, cfg.Phase, cfg.TaskContent, cfg.Deliverer)
}
