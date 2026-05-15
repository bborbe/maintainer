// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command run-task is the local-CLI entry point for maintainer-agent-pr-reviewer.
//
// Reads a markdown task file from disk, runs the agent against it, and
// writes the updated content back to the same file. Mirrors the Kafka
// entry point (../../main.go) but uses file I/O instead of Kafka/CQRS.
package main

import (
	"context"
	"os"
	"path/filepath"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/factory"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubauth"
	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
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

	// Workdir paths for bare-clone cache and per-task worktrees (default: ~/.cache/maintainer/pr-reviewer/*)
	ReposPath string `required:"false" arg:"repos-path" env:"REPOS_PATH" usage:"Root path for bare-clone cache (default: ~/.cache/maintainer/pr-reviewer/repos)"`
	WorkPath  string `required:"false" arg:"work-path"  env:"WORK_PATH"  usage:"Root path for per-task worktrees (default: ~/.cache/maintainer/pr-reviewer/work)"`

	// Review depth passed to /coding:pr-review (short | standard | full)
	ReviewMode string `required:"false" arg:"review-mode" env:"REVIEW_MODE" usage:"Review depth: short | standard | full" default:"standard"`

	// Environment
	Branch base.Branch `required:"true" arg:"branch" env:"BRANCH" usage:"branch" default:"dev"`

	// Phase to run (framework requires explicit phase)
	Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | in_progress | ai_review" default:"in_progress"`

	// Task file for local development
	TaskFilePath string `required:"true" arg:"task-file" env:"TASK_FILE" usage:"Path to the markdown task file"`

	// GitHub token forwarded to the Claude CLI subprocess as GH_TOKEN for gh auth.
	// cmd/run-task uses NoopAuthSetup — the developer's existing gh auth login handles git credentials.
	GHToken string `required:"false" arg:"gh-token" env:"GH_TOKEN" usage:"GitHub token for gh CLI auth" display:"length"`

	// Repo allowlist — comma-separated host/owner/repo entries; empty means allow-all.
	RepoAllowlist string `required:"false" arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo); empty means allow-all"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	repoAllowlist, err := prpkg.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return err
	}
	// Warn on malformed entries; allow-all and wildcard semantics handled by IsAllowed at match time.
	if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
		glog.Warningf(
			"REPO_ALLOWLIST contains malformed entries (will be ignored at match time): %v",
			validationErr,
		)
	}
	glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))

	taskContent, err := os.ReadFile(
		a.TaskFilePath,
	) // #nosec G304 -- filePath from trusted CLI input
	if err != nil {
		return errors.Wrapf(ctx, err, "read task file: %s", a.TaskFilePath)
	}

	reposPath, workPath, err := a.resolveCachePaths(ctx)
	if err != nil {
		return err
	}

	deliverer := factory.CreateFileResultDeliverer(a.TaskFilePath)

	authSetup := githubauth.NewNoopAuthSetup()
	result, err := factory.RunAgent(ctx, factory.RunConfig{
		ClaudeConfigDir: a.ClaudeConfigDir,
		AgentDir:        a.AgentDir,
		Model:           a.Model,
		GHToken:         a.GHToken,
		ReposPath:       reposPath,
		WorkPath:        workPath,
		ReviewMode:      a.ReviewMode,
		RepoAllowlist:   repoAllowlist,
		AuthSetup:       authSetup,
		Phase:           a.Phase,
		TaskContent:     string(taskContent),
		Deliverer:       deliverer,
	})
	if err != nil {
		return errors.Wrap(ctx, err, "agent run failed")
	}
	return agentlib.PrintResult(result)
}

// resolveCachePaths fills in defaults for ReposPath/WorkPath when unset
// (~/.cache/maintainer/pr-reviewer/{repos,work}). The pod entry point requires
// explicit /repos and /work mounts, but local CLI usage benefits from a default.
func (a *application) resolveCachePaths(ctx context.Context) (string, string, error) {
	reposPath := a.ReposPath
	workPath := a.WorkPath
	if reposPath != "" && workPath != "" {
		return reposPath, workPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", errors.Wrap(ctx, err, "resolve user home dir")
	}
	if reposPath == "" {
		reposPath = filepath.Join(home, ".cache", "maintainer", "pr-reviewer", "repos")
	}
	if workPath == "" {
		workPath = filepath.Join(home, ".cache", "maintainer", "pr-reviewer", "work")
	}
	return reposPath, workPath, nil
}
