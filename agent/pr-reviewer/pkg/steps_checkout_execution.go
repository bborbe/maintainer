// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	"github.com/bborbe/errors"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/prompts"
)

// checkoutExecutionStep is the execution phase step that checks out the
// target ref as an on-disk worktree and runs Claude against the real files.
type checkoutExecutionStep struct {
	repoManager     git.RepoManager
	claudeConfigDir claudelib.ClaudeConfigDir
	agentDir        claudelib.AgentDir
	model           claudelib.ClaudeModel
	env             map[string]string
	allowedTools    claudelib.AllowedTools
	reviewMode      string
	repoAllowlist   []string
}

// NewCheckoutExecutionStep constructs the execution-phase step that wires
// RepoManager checkout into the Claude runner working directory.
func NewCheckoutExecutionStep(
	repoManager git.RepoManager,
	claudeConfigDir claudelib.ClaudeConfigDir,
	agentDir claudelib.AgentDir,
	model claudelib.ClaudeModel,
	env map[string]string,
	allowedTools claudelib.AllowedTools,
	reviewMode string,
	repoAllowlist []string,
) agentlib.Step {
	return &checkoutExecutionStep{
		repoManager:     repoManager,
		claudeConfigDir: claudeConfigDir,
		agentDir:        agentDir,
		model:           model,
		env:             env,
		allowedTools:    allowedTools,
		reviewMode:      reviewMode,
		repoAllowlist:   repoAllowlist,
	}
}

// Name implements agentlib.Step.
func (s *checkoutExecutionStep) Name() string { return "pr-execute" }

// ShouldRun returns false if ## Review already exists (idempotent).
func (s *checkoutExecutionStep) ShouldRun(_ context.Context, md *agentlib.Markdown) (bool, error) {
	_, exists := md.FindSection("## Review")
	return !exists, nil
}

// Run checks out the target ref as a worktree, then runs Claude in that
// directory to produce the ## Review section.
func (s *checkoutExecutionStep) Run(
	ctx context.Context,
	md *agentlib.Markdown,
) (*agentlib.Result, error) {
	cloneURL, _ := md.Frontmatter.String("clone_url")
	ref, _ := md.Frontmatter.String("ref")
	taskID, _ := md.Frontmatter.String("task_identifier")
	baseRef, _ := md.Frontmatter.String("base_ref")

	if cloneURL == "" {
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "execution step: clone_url is missing from task frontmatter",
		}, nil
	}
	if ref == "" {
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "execution step: ref is missing from task frontmatter",
		}, nil
	}
	if baseRef == "" {
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "execution step: base_ref is missing from task frontmatter",
		}, nil
	}

	// Pre-parse clone_url to extract host/owner/repo for allowlist and
	// auth-failure diagnostics. A parse failure is a hard error — the URL is
	// malformed and no clone can proceed.
	parts, parseErr := git.ParseCloneURLParts(ctx, cloneURL)
	if parseErr != nil {
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: fmt.Sprintf("execution step: failed to parse clone_url: %v", parseErr),
		}, nil
	}
	repoKey := parts.Host + "/" + parts.Owner + "/" + parts.Repo

	if result := s.checkAllowlist(ctx, cloneURL); result != nil {
		return result, nil
	}

	worktreePath, err := s.repoManager.EnsureWorktree(ctx, cloneURL, ref, taskID)
	if err != nil {
		if git.IsGitAuthFailure(err) {
			// Underlying git error is intentionally NOT included in the diagnostic
			// (it could in theory echo credential-bearing strings). Operators dig
			// into pod logs at glog v(2) for the raw git stderr.
			glog.V(2).
				Infof("clone auth failure repo=%s ref=%s task_id=%s err=%v", repoKey, ref, taskID, err)
			return &agentlib.Result{
				Status: agentlib.AgentStatusNeedsInput,
				Message: fmt.Sprintf(
					"execution step: clone failed for %s: authentication required (set GH_TOKEN and re-trigger)",
					repoKey,
				),
			}, nil
		}
		return nil, errors.Wrapf(
			ctx,
			err,
			"ensure worktree repo=%s ref=%s task_id=%s",
			repoKey,
			ref,
			taskID,
		)
	}

	instructions, err := prompts.BuildExecutionInstructions(
		ctx,
		s.claudeConfigDir,
		s.reviewMode,
		baseRef,
	)
	if err != nil {
		return nil, errors.Wrapf(
			ctx,
			err,
			"build execution instructions base_ref=%s mode=%s",
			baseRef,
			s.reviewMode,
		)
	}

	return s.runClaude(ctx, md, worktreePath, instructions)
}

// checkAllowlist returns a non-nil Result if the cloneURL is blocked by the
// allowlist or fails to parse. Returns nil when the clone is permitted.
// Must be called before EnsureWorktree — cloning is the trust boundary.
func (s *checkoutExecutionStep) checkAllowlist(
	ctx context.Context,
	cloneURL string,
) *agentlib.Result {
	if len(s.repoAllowlist) == 0 {
		return nil
	}
	parts, parseErr := git.ParseCloneURLParts(ctx, cloneURL)
	if parseErr != nil {
		return &agentlib.Result{
			Status: agentlib.AgentStatusFailed,
			Message: fmt.Sprintf(
				"execution step: failed to parse clone_url for allowlist check: %v",
				parseErr,
			),
		}
	}
	repoKey := parts.Host + "/" + parts.Owner + "/" + parts.Repo
	for _, entry := range s.repoAllowlist {
		if entry == repoKey {
			return nil
		}
	}
	return &agentlib.Result{
		Status: agentlib.AgentStatusNeedsInput,
		Message: fmt.Sprintf(
			"execution step: repo %q is not on the allowlist (%d entries); task routed to human review without clone",
			repoKey,
			len(s.repoAllowlist),
		),
	}
}

func (s *checkoutExecutionStep) runClaude(
	ctx context.Context,
	md *agentlib.Markdown,
	worktreePath string,
	instructions claudelib.Instructions,
) (*agentlib.Result, error) {
	runner := claudelib.NewClaudeRunner(claudelib.ClaudeRunnerConfig{
		ClaudeConfigDir:  s.claudeConfigDir,
		AllowedTools:     s.allowedTools,
		Model:            s.model,
		WorkingDirectory: claudelib.AgentDir(worktreePath),
		Env:              s.env,
	})

	taskContent, err := md.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "execution marshal task")
	}

	prompt := claudelib.BuildPrompt(instructions.String(), nil, taskContent)
	runResult, runErr := runner.Run(ctx, prompt)
	if runErr != nil {
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: fmt.Sprintf("execution claude run failed: %v", runErr),
		}, nil
	}

	md.ReplaceSection(agentlib.Section{
		Heading: "## Review",
		Body:    runResult.Result,
	})

	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: "ai_review",
	}, nil
}
