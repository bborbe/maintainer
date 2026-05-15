// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	"github.com/bborbe/errors"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/git"
	"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/prompts"
	repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"
)

// githubPRURLPattern matches a GitHub PR URL in arbitrary text.
var githubPRURLPattern = regexp.MustCompile(`https://github\.com/[^/\s]+/[^/\s]+/pull/\d+`)

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
	prPoster        PrPoster // nil = skip posting
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
	prPoster PrPoster,
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
		prPoster:        prPoster,
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
	if repoallowlist.IsAllowed(s.repoAllowlist, repoKey) {
		return nil
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

// ExtractPRURL scans md.Preamble plus every section before the first "## "
// (H2) heading for a GitHub PR URL. agentlib splits sections at both "# " (H1)
// and "## " (H2), so when the watcher writes the task body as:
//
//	# PR Review: <title>
//	<PR URL>
//	## Plan
//
// md.Preamble is always empty and the URL sits inside the H1 section body.
// Stopping at the first H2 prevents matching URLs Claude writes inside ## Review.
func ExtractPRURL(md *agentlib.Markdown) string {
	if u := githubPRURLPattern.FindString(md.Preamble); u != "" {
		return u
	}
	for _, sec := range md.Sections {
		if strings.HasPrefix(sec.Heading, "## ") {
			break // stop at first H2 — Claude-authored body starts here
		}
		if u := githubPRURLPattern.FindString(sec.Heading + "\n" + sec.Body); u != "" {
			return u
		}
	}
	return ""
}

func (s *checkoutExecutionStep) runClaude(
	ctx context.Context,
	md *agentlib.Markdown,
	worktreePath string,
	instructions claudelib.Instructions,
) (*agentlib.Result, error) {
	// Cache PR URL BEFORE any md mutations to avoid matching URLs that Claude
	// writes inside the ## Review section body.
	prURLStr := ExtractPRURL(md)

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

	// Vault-first invariant: write ## Review BEFORE any API call.
	md.ReplaceSection(agentlib.Section{
		Heading: "## Review",
		Body:    runResult.Result,
	})

	return s.postAndRoute(ctx, md, prURLStr, worktreePath, time.Now())
}

// postAndRoute handles the posting sequence after ## Review has been written to
// the vault. Extracted for testability — tests call this directly without Claude.
func (s *checkoutExecutionStep) postAndRoute(
	ctx context.Context,
	md *agentlib.Markdown,
	prURLStr string,
	worktreePath string,
	jobRunTime time.Time,
) (*agentlib.Result, error) {
	// nil poster = skip posting (backward-compatible for cmd/run-task).
	if s.prPoster == nil {
		return &agentlib.Result{
			Status:    agentlib.AgentStatusDone,
			NextPhase: "ai_review",
		}, nil
	}

	// Extract verdict + summary from ## Review (already written to vault).
	var reviewBody string
	if reviewSection, ok := md.FindSection("## Review"); ok && reviewSection != nil {
		reviewBody = reviewSection.Body
	}
	verdict := ParseVerdict(reviewBody)
	summary := StripJSONVerdict(reviewBody)

	prInfo, earlyResult := s.resolvePRInfo(ctx, md, prURLStr, jobRunTime)
	if earlyResult != nil {
		return earlyResult, nil
	}

	// Get head SHA from frontmatter.
	ref, _ := md.Frontmatter.String("ref")

	// Post the review.
	result := s.prPoster.Post(ctx, PostRequest{
		PR:      *prInfo,
		HeadSHA: ref,
		Verdict: verdict.Verdict,
		Summary: summary,
		WorkDir: worktreePath,
	})

	// Always append diagnostic block — one entry per Job run, append-only.
	appendDiagnosticsSection(
		md,
		buildDiagnosticBlock(jobRunTime, md.Frontmatter.TriggerCount(), result),
	)

	// Route based on outcome.
	if result.Outcome == "success" || result.Class == ErrorClassNotAFailure {
		return &agentlib.Result{
			Status:    agentlib.AgentStatusDone,
			NextPhase: "ai_review",
		}, nil
	}

	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: "human_review",
		Message:   "posting failed: " + result.ErrorMessage,
	}, nil
}

// resolvePRInfo validates and parses the PR URL, writes diagnostics on failure,
// and returns either a parsed PRInfo or an early result to return from postAndRoute.
func (s *checkoutExecutionStep) resolvePRInfo(
	ctx context.Context,
	md *agentlib.Markdown,
	prURLStr string,
	jobRunTime time.Time,
) (*PRInfo, *agentlib.Result) {
	tc := md.Frontmatter.TriggerCount()
	if prURLStr == "" {
		appendDiagnosticsSection(md, buildDiagnosticBlock(jobRunTime, tc, PostResult{
			Outcome:      "failed",
			FailureStep:  "pr_url_extraction",
			Class:        ErrorClassPermanent,
			ErrorMessage: "no GitHub PR URL found in task preamble",
		}))
		return nil, &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: "posting skipped: no GitHub PR URL found in task preamble",
		}
	}

	prInfo, parseErr := ParsePRURL(ctx, prURLStr)
	if parseErr != nil {
		appendDiagnosticsSection(md, buildDiagnosticBlock(jobRunTime, tc, PostResult{
			Outcome:      "failed",
			FailureStep:  "pr_url_parse",
			Class:        ErrorClassPermanent,
			ErrorMessage: fmt.Sprintf("failed to parse PR URL %q: %v", prURLStr, parseErr),
		}))
		return nil, &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: fmt.Sprintf("posting skipped: failed to parse PR URL: %v", parseErr),
		}
	}

	if prInfo.Platform != PlatformGitHub {
		glog.Warningf(
			"posting skipped: non-GitHub platform %q for URL %q",
			prInfo.Platform,
			prURLStr,
		)
		appendDiagnosticsSection(md, buildDiagnosticBlock(jobRunTime, tc, PostResult{
			Outcome:     "failed",
			FailureStep: "pr_url_platform",
			Class:       ErrorClassPermanent,
			ErrorMessage: fmt.Sprintf(
				"non-GitHub platform %q is out of scope for posting",
				prInfo.Platform,
			),
		}))
		return nil, &agentlib.Result{
			Status:    agentlib.AgentStatusDone,
			NextPhase: "ai_review",
		}
	}

	return prInfo, nil
}

// appendDiagnosticsSection appends block to the ## Diagnostics section (creates
// it if absent). Append-only — one block per Job run preserves history.
func appendDiagnosticsSection(md *agentlib.Markdown, block string) {
	var existingBody string
	if existing, ok := md.FindSection("## Diagnostics"); ok && existing != nil {
		existingBody = existing.Body
	}
	newBody := strings.TrimLeft(existingBody+"\n"+block, "\n")
	md.ReplaceSection(agentlib.Section{Heading: "## Diagnostics", Body: newBody})
}

// buildDiagnosticBlock formats one posting-attempt entry for ## Diagnostics.
// Success emits a compact one-liner; failure emits a fenced YAML block.
func buildDiagnosticBlock(jobRunTime time.Time, triggerCount int, result PostResult) string {
	if result.Outcome == "success" {
		return fmt.Sprintf("job_run: %s outcome: success review_id: %d\n",
			jobRunTime.UTC().Format(time.RFC3339), result.ReviewID)
	}
	httpStatusStr := "null"
	if result.HTTPStatus != 0 {
		httpStatusStr = fmt.Sprintf("%d", result.HTTPStatus)
	}
	respBody := result.ResponseBody
	if respBody == "" {
		respBody = "<empty>"
	}
	return fmt.Sprintf(
		"```yaml\njob_run: %s\ntrigger_count: %d\noutcome: failed\nfailure_step: %s\nclass: %s\nescalate_hint: %v\nattempt: %d\nhttp_status: %s\nerror_message: %q\nresponse_body: %q\nelapsed_ms: %d\n```\n",
		jobRunTime.UTC().Format(time.RFC3339),
		triggerCount,
		result.FailureStep,
		result.Class,
		result.EscalateHint,
		result.Attempt,
		httpStatusStr,
		result.ErrorMessage,
		respBody,
		result.ElapsedMs,
	)
}
