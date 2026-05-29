// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	domain "github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
)

// changelogFileName is the only file the execution step rewrites in the
// cloned target repo. Spec 049 § Non-goals explicitly defers mono-repo
// support (multiple CHANGELOGs in one repo).
const changelogFileName = "CHANGELOG.md"

// workdirPrefix is the os.TempDir-rooted prefix used for ephemeral clone
// workdirs. Full path: <tempdir>/<workdirPrefix><task_identifier>/.
// The directory is removed on every Run exit path via defer.
const workdirPrefix = "github-releaser-"

// executionStep implements agentlib.Step. Dependencies are constructor-injected;
// no global state. Both ops (clone/commit/tag/push) and cloneURLBuilder are
// mockable seams — the integration tests in steps_execution_test.go use a
// counterfeiter GitOps mock and a stub URL builder.
type executionStep struct {
	ops     git.GitOps
	ghToken string
}

// NewExecutionStep wires the execution step with its GitOps seam and the
// GitHub token (used for HTTPS auth URL transformation). Empty ghToken
// means clone goes out anonymously — fine for tests; production always
// supplies a token.
func NewExecutionStep(ops git.GitOps, ghToken string) agentlib.Step {
	return &executionStep{ops: ops, ghToken: ghToken}
}

// Name implements agentlib.Step.
func (s *executionStep) Name() string { return "github-release-execute" }

// ShouldRun returns true. The step is idempotent at the framework level:
// a re-trigger overwrites ## Result. The controller's per-task lock
// prevents concurrent invocations on the same task_identifier.
func (s *executionStep) ShouldRun(_ context.Context, _ *agentlib.Markdown) (bool, error) {
	return true, nil
}

// Run executes the direct-push release pipeline. Sequence:
//  1. Read & validate ## Plan(outcome=ready) + frontmatter
//  2. Create ephemeral workdir under os.TempDir()
//  3. Clone target repo via GitOps
//  4. Read + rewrite CHANGELOG.md (## Unreleased → next header)
//  5. Commit + annotated-tag + push
//  6. Write ## Result(outcome=released) and return Done/NextPhase=ai_review
//
// Failures at any step produce ## Result(outcome=failed) + error_category
// and return Status=Failed (controller retry per its cap).
func (s *executionStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
	plan, err := s.validatePlan(ctx, md)
	if err != nil {
		return s.fail(ctx, md, git.ErrorCategoryUnknown, err)
	}

	cloneURL, ref, taskID, err := s.extractFrontmatter(md)
	if err != nil {
		return s.fail(ctx, md, git.ErrorCategoryUnknown, err)
	}

	workdir := s.setupWorkdir(taskID)
	defer func() {
		if err := os.RemoveAll(workdir); err != nil {
			glog.Warningf("workdir cleanup failed: path=%s err=%v", workdir, err)
		}
	}()

	sha, tagName, failResult := s.executeDirectPush(ctx, md, workdir, plan, cloneURL, ref)
	if failResult != nil {
		return failResult, nil // fail() already called inside executeDirectPush
	}

	output := ResultOutput{
		Outcome:   ResultOutcomeReleased,
		Path:      ResultPathDirectPush,
		CommitSHA: sha,
		Tag:       tagName,
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Result", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "marshal ## Result section")
	}
	md.ReplaceSection(section)

	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: string(domain.TaskPhaseAIReview),
	}, nil
}

// validatePlan extracts and validates the ## Plan section.
func (s *executionStep) validatePlan(
	ctx context.Context,
	md *agentlib.Markdown,
) (*PlanOutput, error) {
	plan, err := agentlib.ExtractSection[PlanOutput](ctx, md, "## Plan")
	if err != nil || plan == nil {
		return nil, errors.Wrapf(ctx, err, "execution invoked but planning did not complete")
	}
	if plan.Outcome != PlanOutcomeReady || plan.NextVersion == "" || plan.NextVersionHeader == "" {
		return nil, errors.Errorf(
			ctx,
			"execution invoked with non-ready plan: outcome=%s next_version=%q next_version_header=%q",
			plan.Outcome,
			plan.NextVersion,
			plan.NextVersionHeader,
		)
	}
	return plan, nil
}

// extractFrontmatter reads the required frontmatter fields.
func (s *executionStep) extractFrontmatter(
	md *agentlib.Markdown,
) (cloneURL, ref, taskID string, _ error) {
	cloneURL, _ = md.Frontmatter.String("clone_url")
	ref, _ = md.Frontmatter.String("ref")
	taskID, _ = md.Frontmatter.String("task_identifier")
	if cloneURL == "" || ref == "" || taskID == "" {
		return "", "", "", errors.Errorf(
			context.Background(),
			"missing frontmatter: clone_url=%q ref=%q task_identifier=%q",
			cloneURL, ref, taskID,
		)
	}
	return cloneURL, ref, taskID, nil
}

// setupWorkdir creates a clean ephemeral workdir under os.TempDir().
func (s *executionStep) setupWorkdir(taskID string) string {
	workdir := filepath.Join(os.TempDir(), workdirPrefix+taskID)
	if err := os.RemoveAll(workdir); err != nil {
		glog.Warningf("remove stale workdir failed: path=%s err=%v", workdir, err)
	}
	return workdir
}

// executeDirectPush runs the clone → rewrite → commit → tag → push sequence.
// Returns (sha, tagName, nil) on success, or ( "", "", failResult) on failure
// where failResult is the result of calling s.fail() with the appropriate error.
func (s *executionStep) executeDirectPush(
	ctx context.Context,
	md *agentlib.Markdown,
	workdir string,
	plan *PlanOutput,
	cloneURL, ref string,
) (sha, tagName string, _ *agentlib.Result) {
	authedURL := s.injectToken(cloneURL)
	if err := s.ops.Clone(ctx, authedURL, ref, workdir); err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}

	changelogPath := filepath.Join(workdir, changelogFileName)
	content, err := os.ReadFile(
		changelogPath,
	) // #nosec G304 -- workdir is os.TempDir-rooted; filename is constant
	if err != nil {
		category := git.ErrorCategoryChangelogMissing
		if !os.IsNotExist(err) {
			category = git.ErrorCategoryUnknown
		}
		result, _ := s.fail(ctx, md, category, errors.Wrapf(ctx, err, "read %s", changelogPath))
		return "", "", result
	}

	rewritten, err := changelog.RewriteUnreleasedHeader(content, plan.NextVersionHeader)
	if err != nil {
		result, _ := s.fail(ctx, md, git.ErrorCategoryUnreleasedNotFound,
			errors.Wrap(ctx, err, "rewrite ## Unreleased"))
		return "", "", result
	}
	if err := os.WriteFile(changelogPath, rewritten, 0o644); err != nil { // #nosec G306,G703 -- standard perms; workdir is os.TempDir-rooted
		result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
			errors.Wrapf(ctx, err, "write %s", changelogPath))
		return "", "", result
	}

	tagName = strings.TrimPrefix(plan.NextVersionHeader, "## ")
	sha, err = s.ops.Commit(ctx, workdir, "release "+tagName, changelogFileName)
	if err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}
	if err := s.ops.Tag(ctx, workdir, tagName, "release "+tagName); err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}
	if err := s.ops.Push(ctx, workdir, "HEAD", "refs/tags/"+tagName); err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}
	return sha, tagName, nil
}

// injectToken transforms an HTTPS GitHub URL into a token-authenticated form.
// https://github.com/owner/repo.git → https://x-access-token:<token>@github.com/owner/repo.git
// Empty token returns the input unchanged (anonymous; fine for tests).
func (s *executionStep) injectToken(cloneURL string) string {
	if s.ghToken == "" {
		return cloneURL
	}
	const prefix = "https://"
	if !strings.HasPrefix(cloneURL, prefix) {
		return cloneURL
	}
	return prefix + "x-access-token:" + s.ghToken + "@" + strings.TrimPrefix(cloneURL, prefix)
}

// fail writes a ## Result(outcome=failed) section with the supplied
// error_category + error string, and returns Status=Failed for controller
// retry. The workdir cleanup defer in Run still runs after this returns.
func (s *executionStep) fail(
	ctx context.Context,
	md *agentlib.Markdown,
	category git.ErrorCategory,
	cause error,
) (*agentlib.Result, error) {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	output := ResultOutput{
		Outcome:       ResultOutcomeFailed,
		Path:          ResultPathDirectPush,
		ErrorCategory: category,
		Error:         msg,
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Result", output)
	if err != nil {
		// Failing to marshal the failure is a real error — surface it so
		// the framework records the panic-equivalent rather than swallowing.
		return nil, errors.Wrapf(ctx, err, "marshal ## Result section (failed)")
	}
	md.ReplaceSection(section)

	glog.V(2).Infof("execution failed: category=%s err=%v", category, cause)
	return &agentlib.Result{
		Status:  agentlib.AgentStatusFailed,
		Message: msg,
	}, nil
}
