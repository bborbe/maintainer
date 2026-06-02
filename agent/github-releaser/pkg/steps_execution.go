// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	domain "github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/plugin"
)

// changelogFileName is the only file the execution step rewrites in the
// cloned target repo. Spec 049 § Non-goals explicitly defers mono-repo
// support (multiple CHANGELOGs in one repo).
const changelogFileName = "CHANGELOG.md"

// workdirPrefix is the os.TempDir-rooted prefix used for ephemeral clone
// workdirs. Full path: <tempdir>/<workdirPrefix><task_identifier>/.
// On the happy path the directory is INTENTIONALLY preserved past Run's
// return so the next phase (ai_review) can read `git log -1 --name-only`
// against it. The ai-review step owns workdir lifecycle for terminal
// transitions (Approved+push done → Done, or human_review exit). The
// failure-path defer below still removes it.
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

// Run executes the local-release pipeline. Sequence:
//  1. Read & validate ## Plan(outcome=ready) + frontmatter
//  2. Create ephemeral workdir under os.TempDir()
//  3. Clone target repo via GitOps
//  4. Read + rewrite CHANGELOG.md (apply plan.RewrittenUnreleased body if
//     plan.RewriteNeeded, then rename the header to plan.NextVersionHeader)
//  5. Commit + annotated-tag
//  6. Write ## Result(outcome=released) and return Done/NextPhase=ai_review
//
// Note: the network push happens in the ai_review step (spec 058 prompt 3),
// not here. The local clone + tag are preserved past Run's return so
// ai_review can read them.
//
// Failures at any step produce ## Result(outcome=failed) + error_category
// and return Status=Failed (controller retry per its cap). The workdir is
// removed on every failure path; the happy path keeps the workdir alive
// for ai_review.
func (s *executionStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
	plan, err := s.validatePlan(ctx, md)
	if err != nil {
		return s.fail(ctx, md, git.ErrorCategoryUnknown, err)
	}

	cloneURL, ref, taskID, err := s.extractFrontmatter(ctx, md)
	if err != nil {
		return s.fail(ctx, md, git.ErrorCategoryUnknown, err)
	}

	workdir := s.setupWorkdir(taskID)
	// Conditional cleanup: only remove the workdir on the failure path.
	// The happy path leaves it in place so ai_review (next phase) can
	// read it via result.Workdir.
	releaseSuccess := false
	defer func() {
		if !releaseSuccess {
			if err := os.RemoveAll(workdir); err != nil {
				glog.Warningf("workdir cleanup failed: path=%s err=%v", workdir, err)
			}
		}
	}()

	sha, tagName, failResult := s.executeLocalRelease(ctx, md, workdir, plan, cloneURL, ref)
	if failResult != nil {
		return failResult, nil // fail() already called inside executeLocalRelease
	}

	output := ResultOutput{
		Outcome:   ResultOutcomeReleased,
		Path:      ResultPathDirectPush,
		CommitSHA: sha,
		Tag:       tagName,
		Workdir:   workdir,
		LocalTag:  tagName,
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Result", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "marshal ## Result section")
	}
	md.ReplaceSection(section)

	// Mark success: defer above will skip RemoveAll so the workdir (with
	// the local commit + tag) survives until ai_review finishes.
	releaseSuccess = true
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
	ctx context.Context,
	md *agentlib.Markdown,
) (cloneURL, ref, taskID string, _ error) {
	cloneURL, _ = md.Frontmatter.String("clone_url")
	ref, _ = md.Frontmatter.String("ref")
	taskID, _ = md.Frontmatter.String("task_identifier")
	if cloneURL == "" || ref == "" || taskID == "" {
		return "", "", "", errors.Errorf(
			ctx,
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

// executeLocalRelease runs the clone → (optional body rewrite) → header
// rename → commit → tag sequence. The network push happens in the ai_review
// step (spec 058 prompt 3), not here.
//
// Returns (sha, tagName, nil) on success, or ( "", "", failResult) on failure
// where failResult is the result of calling s.fail() with the appropriate error.
//
//nolint:gocognit,funlen // multi-stage release pipeline with branching error paths
func (s *executionStep) executeLocalRelease(
	ctx context.Context,
	md *agentlib.Markdown,
	workdir string,
	plan *PlanOutput,
	cloneURL, ref string,
) (sha, tagName string, _ *agentlib.Result) {
	normalizedURL := normalizeCloneURLToHTTPS(cloneURL)
	authedURL := s.injectToken(normalizedURL)
	if err := s.ops.Clone(ctx, authedURL, ref, workdir); err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}

	// Detect plugin manifests BEFORE any writes.
	detectedManifests, err := plugin.DetectManifests(ctx, workdir)
	if err != nil {
		result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
			errors.Wrapf(ctx, err, "detect plugin manifests in %s", workdir))
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

	// Optional body rewrite: only when planning flagged the body as
	// non-conformant. Done in-memory BEFORE the header rename so the
	// final commit is a single atomic "rewrite + rename" change.
	rewritten := content
	if plan.RewriteNeeded {
		rewritten, err = changelog.ReplaceUnreleasedBody(
			ctx,
			content,
			plan.RewrittenUnreleased,
		)
		if err != nil {
			result, _ := s.fail(ctx, md, git.ErrorCategoryUnreleasedNotFound,
				errors.Wrap(ctx, err, "replace ## Unreleased body"))
			return "", "", result
		}
	}

	rewritten, err = changelog.RewriteUnreleasedHeader(
		ctx,
		rewritten,
		plan.NextVersionHeader,
	)
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

	// Bump and write detected plugin manifests.
	unprefixedVersion := deriveUnprefixedVersion(plan.NextVersionHeader)
	for _, manifestPath := range detectedManifests {
		manifestAbsPath := filepath.Join(workdir, manifestPath)
		manifestContent, err := os.ReadFile(
			manifestAbsPath,
		) // #nosec G304 -- workdir is os.TempDir-rooted
		if err != nil {
			result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
				errors.Wrapf(ctx, err, "read %s", manifestAbsPath))
			return "", "", result
		}

		var rewrittenManifest []byte
		if strings.HasSuffix(manifestPath, "plugin.json") {
			rewrittenManifest, err = plugin.BumpPluginJSON(ctx, manifestContent, unprefixedVersion)
		} else if strings.HasSuffix(manifestPath, "marketplace.json") {
			rewrittenManifest, err = plugin.BumpMarketplaceJSON(ctx, manifestContent, unprefixedVersion)
		} else {
			result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
				errors.Errorf(ctx, "unsupported manifest type: %s", manifestPath))
			return "", "", result
		}
		if err != nil {
			result, _ := s.fail(ctx, md, git.ErrorCategoryPluginManifestInvalid,
				errors.Wrapf(ctx, err, "bump %s", manifestPath))
			return "", "", result
		}

		if err := os.WriteFile(manifestAbsPath, rewrittenManifest, 0o644); err != nil { // #nosec G306,G703 -- standard perms; workdir is os.TempDir-rooted
			result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
				errors.Wrapf(ctx, err, "write %s", manifestAbsPath))
			return "", "", result
		}
	}

	tagName = strings.TrimPrefix(plan.NextVersionHeader, "## ")
	// Build the full commit path list: changelog + detected manifests (in that order).
	commitPaths := append([]string{changelogFileName}, detectedManifests...)
	sha, err = s.ops.Commit(ctx, workdir, "release "+tagName, commitPaths...)
	if err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}
	// Pre-push guard: the release commit must touch exactly the files we
	// rewrote (changelog + detected manifests). Fail closed BEFORE tag
	// if anything else slipped in — the release trust model depends on
	// this commit being changelog+manifests-only. Push is no longer in
	// this step (moved to ai_review in spec 058), but the guard still
	// runs here so a non-conformant commit never gets a tag.
	expectedFiles := append([]string{changelogFileName}, detectedManifests...)
	if failResult := s.guardCommittedFiles(ctx, md, workdir, expectedFiles); failResult != nil {
		return "", "", failResult
	}
	if err := s.ops.Tag(ctx, workdir, tagName, "release "+tagName); err != nil {
		result, _ := s.fail(ctx, md, git.ClassifyError(err), err)
		return "", "", result
	}
	return sha, tagName, nil
}

// guardCommittedFiles asserts the HEAD (release) commit changed exactly
// the expected files. On any deviation it writes a ## Result with
// error_category=unexpected_diff and returns a failed Result — the caller
// must abort before tag/push. Returns nil when the commit changed only
// the expected files.
func (s *executionStep) guardCommittedFiles(
	ctx context.Context,
	md *agentlib.Markdown,
	workdir string,
	expectedFiles []string,
) *agentlib.Result {
	files, err := s.ops.CommittedFiles(ctx, workdir)
	if err != nil {
		result, _ := s.fail(ctx, md, git.ErrorCategoryUnknown,
			errors.Wrap(ctx, err, "inspect committed files"))
		return result
	}
	if !sameStringSet(files, expectedFiles) {
		result, _ := s.fail(ctx, md, git.ErrorCategoryUnexpectedDiff,
			errors.Errorf(ctx,
				"release commit must change only %v, got %v", expectedFiles, files))
		return result
	}
	return nil
}

// normalizeCloneURLToHTTPS converts the common GitHub clone-URL forms to
// canonical HTTPS so the installation-token auth in injectToken always
// applies. The github-releaser authenticates with a GitHub App installation
// token (HTTPS only) and the runtime image has no ssh client, so an SSH
// clone URL can never succeed — it must be rewritten to HTTPS before
// injectToken runs.
//
//	git@github.com:owner/repo.git        → https://github.com/owner/repo.git
//	ssh://git@github.com/owner/repo.git  → https://github.com/owner/repo.git
//	https://github.com/owner/repo.git    → unchanged
//	https://github.com/owner/repo        → unchanged (no .git is fine)
//
// Any form it does not recognize is returned unchanged so the failure
// surfaces loudly downstream rather than being silently mangled.
func normalizeCloneURLToHTTPS(raw string) string {
	const (
		scpPrefix = "git@github.com:"
		sshPrefix = "ssh://git@github.com/"
		httpsBase = "https://github.com/"
	)
	switch {
	case strings.HasPrefix(raw, scpPrefix):
		return httpsBase + strings.TrimPrefix(raw, scpPrefix)
	case strings.HasPrefix(raw, sshPrefix):
		return httpsBase + strings.TrimPrefix(raw, sshPrefix)
	default:
		return raw
	}
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

// deriveUnprefixedVersion strips "## " prefix and "v" prefix from
// a version header to produce the unprefixed semver string.
// "## v0.10.0" → "0.10.0"
// "## 0.10.0" → "0.10.0"
// "0.10.0" → "0.10.0"
func deriveUnprefixedVersion(header string) string {
	header = strings.TrimPrefix(header, "## ")
	header = strings.TrimPrefix(header, "v")
	return header
}

// sameStringSet reports whether a and b contain the same elements,
// ignoring order. It never mutates its inputs (callers reuse them
// — e.g. expectedFiles is rendered into the failure message).
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := slices.Clone(a)
	bc := slices.Clone(b)
	slices.Sort(ac)
	slices.Sort(bc)
	return slices.Equal(ac, bc)
}
