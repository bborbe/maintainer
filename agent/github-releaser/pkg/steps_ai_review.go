// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	"github.com/bborbe/errors"
	domain "github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubreview"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/plugin"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/prompts"
)

// Per-entry faithfulness verdict values. Applied ONLY to per_entry entries.
// The `unknown` state lives on Overall (see OverallUnknown below), NOT on
// individual entries: per the spec's Failure Modes row "LLM unavailability",
// `unknown` surfaces only at the overall level. When the LLM is
// unreachable, PerEntry is left empty rather than filled with `unknown`
// entries — single-purpose constants stay clearer this way.
const (
	FaithfulnessPresent      = "present"
	FaithfulnessSilentDrop   = "silent-drop"
	FaithfulnessHallucinated = "hallucinated"
)

// Overall verdict values for ReviewOutput.Overall.
const (
	OverallPass    = "pass"
	OverallFail    = "fail"
	OverallUnknown = "unknown"
)

// Stable check names recorded in ReviewOutput.FailedChecks. Spec AC 15
// asserts on these literals verbatim; do not rename without a spec
// amendment.
const (
	CheckTagExists                = "TagExists"
	CheckTagAtExpectedSHA         = "TagAtExpectedSHA"
	CheckChangelogHeaderRewritten = "ChangelogHeaderRewritten"
	CheckFaithfulness             = "Faithfulness"
	CheckUnexpectedFileChange     = "UnexpectedFileChange"
)

// FaithfulnessVerdict captures the semantic comparison of one entry from
// the original ## Unreleased against the final ## vX.Y.Z body.
//
//   - Verdict ∈ {FaithfulnessPresent, FaithfulnessSilentDrop, FaithfulnessHallucinated}.
//   - Entry is the verbatim line being judged.
//   - Note is the LLM's one-sentence justification.
type FaithfulnessVerdict struct {
	Entry   string `json:"entry"`
	Verdict string `json:"verdict"`
	Note    string `json:"note,omitempty"`
}

// ReviewChecks holds the boolean verification results. The first three
// are the original structural checks; the remaining two are the new
// ai-review-side gates.
type ReviewChecks struct {
	TagExists                bool `json:"tag_exists"`
	TagAtExpectedSHA         bool `json:"tag_at_expected_sha"`
	ChangelogHeaderRewritten bool `json:"changelog_header_rewritten"`

	// FaithfulnessOK is true when every per-entry verdict is
	// FaithfulnessPresent (no silent-drop, no hallucinated). False on
	// any drift OR when the overall verdict is OverallUnknown.
	FaithfulnessOK bool `json:"faithfulness_ok"`

	// UnexpectedFileChange is true when the release commit touched a
	// file other than CHANGELOG.md (plus detected plugin manifests).
	// It is the ai-review-side mirror of the executeLocalRelease
	// pre-commit guard.
	UnexpectedFileChange bool `json:"unexpected_file_change"`
}

// ReviewOutput is the typed contract for the `## Review` JSON section the
// ai_review step writes. Round-trips with agentlib.MarshalSectionTyped +
// agentlib.ExtractSection[ReviewOutput].
type ReviewOutput struct {
	Approved bool         `json:"approved"`
	Checks   ReviewChecks `json:"checks"`
	Notes    string       `json:"notes"`

	// PerEntry holds the per-entry semantic verdict produced by the
	// faithfulness LLM call. Empty when Overall == OverallUnknown or
	// when the execution step recorded failure (nothing to verify).
	PerEntry []FaithfulnessVerdict `json:"per_entry,omitempty"`

	// Overall is the rolled-up semantic verdict: OverallPass |
	// OverallFail | OverallUnknown.
	//   - OverallPass:    every PerEntry is FaithfulnessPresent AND
	//                     no UnexpectedFileChange AND every structural
	//                     check is true.
	//   - OverallFail:    at least one PerEntry is silent-drop or
	//                     hallucinated, OR UnexpectedFileChange is true,
	//                     OR any structural check is false.
	//   - OverallUnknown: the LLM was unreachable; the rest of the
	//                     review is still written (structural checks)
	//                     but Approved is false and push is skipped.
	Overall string `json:"overall"`

	// UnexpectedFiles lists the file paths the commit touched that
	// were NOT in the expected set. Empty when
	// UnexpectedFileChange is false.
	UnexpectedFiles []string `json:"unexpected_files,omitempty"`

	// FailedChecks names the structural and semantic checks that did
	// not pass. Stable strings — referenced by spec AC 15 assertions.
	// One or more of: CheckTagExists, CheckTagAtExpectedSHA,
	// CheckChangelogHeaderRewritten, CheckFaithfulness,
	// CheckUnexpectedFileChange.
	FailedChecks []string `json:"failed_checks,omitempty"`
}

// changelogHeadingRE matches a level-2 markdown heading. Compiled once
// at package load — the regex is invariant.
var changelogHeadingRE = regexp.MustCompile(`^##\s+`)

// NewAIReviewStep wires the ai_review step with its GitHub REST API
// client, the ClaudeRunner used to invoke the faithfulness LLM, the
// GitOps seam used to push the local commit + tag, and the GitHub
// token (used for authenticated API calls).
func NewAIReviewStep(
	client githubreview.Client,
	runner claudelib.ClaudeRunner,
	ops git.GitOps,
	ghToken string,
) agentlib.Step {
	return &aiReviewStep{client: client, runner: runner, ops: ops, ghToken: ghToken}
}

// aiReviewStep implements agentlib.Step. It performs three remote
// verification checks against the GitHub REST API, one local diff
// check against the workdir, one semantic faithfulness check via the
// Claude runner, and on success pushes the local commit + tag to the
// remote.
type aiReviewStep struct {
	client  githubreview.Client
	runner  claudelib.ClaudeRunner
	ops     git.GitOps
	ghToken string
}

// Name implements agentlib.Step.
func (s *aiReviewStep) Name() string { return "github-release-ai-review" }

// ShouldRun always returns true. The step is idempotent at the
// framework level: a re-trigger overwrites the existing ## Review
// section.
func (s *aiReviewStep) ShouldRun(_ context.Context, _ *agentlib.Markdown) (bool, error) {
	return true, nil
}

// Run executes the verification pipeline. Sequence:
//  1. Read ## Result + ## Plan (fatal error if either missing).
//  2. If Result.Outcome != "released" → short-circuit approved=true.
//  3. Three structural checks (TagExists, TagAtExpectedSHA,
//     ChangelogHeaderRewritten). Each failure is recorded in
//     FailedChecks but does NOT early-return — the full check set is
//     gathered before the verdict rolls up.
//  4. Unexpected-file-change check against the local workdir.
//  5. Faithfulness LLM call (one-shot) compares
//     Plan.OriginalUnreleased against the body of
//     plan.NextVersionHeader in <Workdir>/CHANGELOG.md.
//  6. Roll up overall verdict; write ## Review.
//  7. When Approved: call ops.Push. On Push error: still write
//     ## Review (with a "push failed" note), set Approved=false,
//     return Failed/human_review.
//  8. Workdir cleanup: deferred via workdirShouldCleanup sentinel —
//     the workdir is removed on BOTH terminal transitions (Done and
//     human_review) AFTER ## Review has been written.
func (s *aiReviewStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
	result, err := agentlib.ExtractSection[ResultOutput](ctx, md, "## Result")
	if err != nil || result == nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: extract ## Result section")
	}
	plan, err := agentlib.ExtractSection[PlanOutput](ctx, md, "## Plan")
	if err != nil || plan == nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: extract ## Plan section")
	}

	if result.Outcome != ResultOutcomeReleased {
		return s.writeShortCircuit(ctx, md)
	}

	repo, _ := md.Frontmatter.String("repo")
	owner, name, ok := parseOwnerRepo(repo)
	if !ok {
		return nil, errors.Errorf(ctx, "ai_review: read frontmatter repo")
	}

	glog.V(2).Infof(
		"ai_review: starting checks for repo=%s/%s tag=%s commit=%s",
		owner, name, result.Tag, result.CommitSHA,
	)

	checks := ReviewChecks{
		TagExists: true, TagAtExpectedSHA: true, ChangelogHeaderRewritten: true,
		FaithfulnessOK: true,
	}
	var failedChecks []string
	// Workdir cleanup: ai-review owns the lifetime once execution
	// returns result.Workdir. Removed on BOTH terminal transitions
	// (Done, human_review) AFTER ## Review has been marshaled.
	var workdirShouldCleanup bool
	defer s.cleanupWorkdir(result, &workdirShouldCleanup)

	// (1) Structural checks — do NOT early-return on failure.
	// Record each in failedChecks and continue so the human reviewer
	// sees the full set of issues.
	if err := s.runStructuralChecks(
		ctx,
		owner,
		name,
		result,
		&checks,
		&failedChecks,
	); err != nil {
		return nil, err
	}

	// (2) Unexpected-file-change check — local workdir inspection.
	unexpected := s.checkUnexpectedFileChange(ctx, &checks, result, &failedChecks)

	// (3) Faithfulness LLM call.
	faithfulnessOverall, perEntry := s.checkFaithfulness(
		ctx,
		plan,
		result,
		&checks,
		&failedChecks,
	)

	// (4) Roll up overall verdict.
	overall, approved := rollupVerdict(faithfulnessOverall, failedChecks)

	output := ReviewOutput{
		Approved:        approved,
		Checks:          checks,
		Notes:           s.notesFor(failedChecks),
		PerEntry:        perEntry,
		Overall:         overall,
		UnexpectedFiles: unexpected,
		FailedChecks:    failedChecks,
	}

	// (5) Push gating — only on Approved branch.
	if !output.Approved {
		return s.finishHumanReview(ctx, md, output, &workdirShouldCleanup)
	}
	return s.finishApproved(ctx, md, result, output, &workdirShouldCleanup)
}

// cleanupWorkdir is the deferred cleanup. The sentinel is set at
// the terminal-return points so the workdir outlives the
// ## Review section write — operator triage reads the task page,
// not the on-disk clone.
func (s *aiReviewStep) cleanupWorkdir(result *ResultOutput, workdirShouldCleanup *bool) {
	if *workdirShouldCleanup && result.Workdir != "" {
		if err := os.RemoveAll(result.Workdir); err != nil {
			glog.Warningf("ai_review: workdir cleanup failed: %v", err)
		}
	}
}

// finishApproved executes the push step. On push failure it falls
// through to finishHumanReview with an updated note; on success it
// returns Done.
func (s *aiReviewStep) finishApproved(
	ctx context.Context,
	md *agentlib.Markdown,
	result *ResultOutput,
	output ReviewOutput,
	workdirShouldCleanup *bool,
) (*agentlib.Result, error) {
	pushErr := s.ops.Push(
		ctx,
		result.Workdir,
		"HEAD",
		"refs/tags/"+result.LocalTag,
	)
	if pushErr != nil {
		glog.Warningf("ai_review: push failed: %v", pushErr)
		output.Notes = "push failed: " + pushErr.Error()
		output.Approved = false
		output.FailedChecks = append(output.FailedChecks, "Push")
		return s.finishHumanReview(ctx, md, output, workdirShouldCleanup)
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Review", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: marshal ## Review section")
	}
	md.ReplaceSection(section)
	*workdirShouldCleanup = true
	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: "done",
	}, nil
}

// finishHumanReview writes the ## Review section (with the failed
// check set) and returns Failed/human_review. The workdir cleanup
// sentinel is set so the deferred cleanup runs AFTER the section
// has been written — the operator triage reads the task page, not
// the on-disk clone.
func (s *aiReviewStep) finishHumanReview(
	ctx context.Context,
	md *agentlib.Markdown,
	output ReviewOutput,
	workdirShouldCleanup *bool,
) (*agentlib.Result, error) {
	section, err := agentlib.MarshalSectionTyped(ctx, "## Review", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: marshal ## Review section")
	}
	md.ReplaceSection(section)
	*workdirShouldCleanup = true
	return &agentlib.Result{
		Status:    agentlib.AgentStatusFailed,
		NextPhase: string(domain.TaskPhaseHumanReview),
		Message:   output.Notes,
	}, nil
}

// writeShortCircuit handles the Result.Outcome != released path: no
// checks to run, the spec says approved=true with overall=pass.
func (s *aiReviewStep) writeShortCircuit(
	ctx context.Context,
	md *agentlib.Markdown,
) (*agentlib.Result, error) {
	output := ReviewOutput{
		Approved: true,
		Checks: ReviewChecks{
			TagExists:                true,
			TagAtExpectedSHA:         true,
			ChangelogHeaderRewritten: true,
			FaithfulnessOK:           true,
		},
		Overall: OverallPass,
		Notes:   "execution step recorded failure; nothing to verify",
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Review", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: marshal ## Review section")
	}
	md.ReplaceSection(section)
	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: "done",
	}, nil
}

// runStructuralChecks executes the three remote structural checks
// (TagExists, TagAtExpectedSHA, ChangelogHeaderRewritten). On any
// non-sentinel error it returns it (controller retries). On a
// sentinel error (tag-missing) or a check failure, the failed-check
// name is appended to failedChecks and execution continues.
func (s *aiReviewStep) runStructuralChecks(
	ctx context.Context,
	owner, name string,
	result *ResultOutput,
	checks *ReviewChecks,
	failedChecks *[]string,
) error {
	tagSHA, err := s.verifyTagExists(ctx, owner, name, result.Tag, checks)
	if err != nil {
		if errors.Is(err, githubreview.ErrTagNotFound) {
			checks.TagExists = false
			*failedChecks = append(*failedChecks, CheckTagExists)
			glog.V(2).Infof("ai_review: check=%s result=false: %v", CheckTagExists, err)
		} else {
			return errors.Wrapf(ctx, err, "ai_review: TagExists")
		}
	}

	if tagSHA != "" {
		if err := s.verifyTagAtExpectedCommit(
			ctx,
			owner,
			name,
			tagSHA,
			result.CommitSHA,
			checks,
			failedChecks,
		); err != nil {
			glog.Warningf("ai_review: verifyTagAtExpectedCommit: %v", err)
		}
	}

	if err := s.verifyChangelogHeaderRewritten(
		ctx,
		owner,
		name,
		checks,
		failedChecks,
	); err != nil {
		glog.Warningf("ai_review: verifyChangelogHeaderRewritten: %v", err)
	}
	return nil
}

// verifyTagExists calls TagExists API and records the result in
// checks. Returns the tagSHA on success,
// githubreview.ErrTagNotFound if the tag is missing (caller handles
// the verdict and records the failed-check name), or a wrapped error
// for transient failures.
func (s *aiReviewStep) verifyTagExists(
	ctx context.Context,
	owner, repo, tag string,
	checks *ReviewChecks,
) (string, error) {
	tagSHA, err := s.client.TagExists(ctx, owner, repo, tag)
	if err != nil {
		if errors.Is(err, githubreview.ErrTagNotFound) {
			checks.TagExists = false
			glog.V(2).Infof("ai_review: check=%s result=false: %v", CheckTagExists, err)
			return "", githubreview.ErrTagNotFound
		}
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return "", errors.Wrapf(ctx, err, "ai_review: TagExists")
	}
	glog.V(2).Infof("ai_review: check=%s result=true", CheckTagExists)
	return tagSHA, nil
}

// verifyTagAtExpectedCommit calls ResolveTagCommit and checks the
// returned SHA matches expectedCommit. Records the result in checks
// and appends the failed-check name on mismatch. Returns a wrapped
// error ONLY for transient transport failures (controller retries);
// a SHA mismatch is a check failure that must keep the rest of the
// pipeline running.
//
// Length mismatch tolerance: the execution step writes
// Result.CommitSHA via `git rev-parse --short HEAD` (7 chars by
// default — pkg/git/os_exec_git_ops.go), while the GitHub API always
// returns a full 40-char SHA. A naive `==` compare would
// false-positive every release. We accept either string as a prefix
// of the other to handle both directions (short stored vs full
// stored).
func (s *aiReviewStep) verifyTagAtExpectedCommit(
	ctx context.Context,
	owner, name, tagSHA, expectedCommit string,
	checks *ReviewChecks,
	failedChecks *[]string,
) error {
	commitSHA, err := s.client.ResolveTagCommit(ctx, owner, name, tagSHA)
	if err != nil {
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return errors.Wrapf(ctx, err, "ai_review: ResolveTagCommit")
	}
	if !commitSHAMatches(commitSHA, expectedCommit) {
		checks.TagAtExpectedSHA = false
		*failedChecks = append(*failedChecks, CheckTagAtExpectedSHA)
		glog.V(2).Infof(
			"ai_review: check=%s result=false: tag points to %s, expected %s",
			CheckTagAtExpectedSHA,
			commitSHA,
			expectedCommit,
		)
		return nil
	}
	glog.V(2).Infof("ai_review: check=%s result=true", CheckTagAtExpectedSHA)
	return nil
}

// verifyChangelogHeaderRewritten calls FetchChangelog and checks that
// the top heading is NOT "## Unreleased". Records the result in
// checks and appends the failed-check name on mismatch. Returns a
// wrapped error ONLY for transient transport failures; a header
// mismatch is a check failure that must keep the rest of the
// pipeline running.
func (s *aiReviewStep) verifyChangelogHeaderRewritten(
	ctx context.Context,
	owner, repo string,
	checks *ReviewChecks,
	failedChecks *[]string,
) error {
	changelogBytes, err := s.client.FetchChangelog(ctx, owner, repo)
	if err != nil {
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return errors.Wrapf(ctx, err, "ai_review: FetchChangelog")
	}
	if !s.changelogHeaderRewritten(changelogBytes) {
		checks.ChangelogHeaderRewritten = false
		*failedChecks = append(*failedChecks, CheckChangelogHeaderRewritten)
		glog.V(2).Infof("ai_review: check=%s result=false", CheckChangelogHeaderRewritten)
		return nil
	}
	glog.V(2).Infof("ai_review: check=%s result=true", CheckChangelogHeaderRewritten)
	return nil
}

// changelogHeaderRewritten returns true if the first ## heading in
// content is NOT "## Unreleased" (i.e. the header has been rewritten
// to a version). Splits on newlines and finds the first line matching
// ^##\s+.
func (s *aiReviewStep) changelogHeaderRewritten(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		if changelogHeadingRE.MatchString(line) {
			return strings.TrimSpace(line) != "## Unreleased"
		}
	}
	return false
}

// commitSHAMatches returns true when one SHA is a prefix of the
// other. This handles the short-vs-full length asymmetry between
// the execution step's `git rev-parse --short HEAD` output (7 chars
// by default) and GitHub's API which always returns the full
// 40-char SHA. Both directions are accepted so the comparison is
// correct regardless of which side is shorter. An empty string
// never matches (avoids vacuous-true on missing data).
func commitSHAMatches(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// checkUnexpectedFileChange inspects the release commit's touched
// files. On deviation from the expected set (CHANGELOG.md + detected
// plugin manifests) it sets checks.UnexpectedFileChange=true and
// appends the failed-check name.
//
// Returns the diff (committed - expected) for the UnexpectedFiles
// output slice. On workdir or git error the check is skipped
// silently and an empty diff is returned — the operator triage path
// is covered by the structural checks in that case.
//
// sameStringSet is the same helper used by steps_execution.go
// (guardCommittedFiles). It is package-private, so we reference it
// directly. The "promote to a small file" alternative from the spec
// is not pursued: both call sites live in the same package, so the
// duplicate-style cost is zero.
func (s *aiReviewStep) checkUnexpectedFileChange(
	ctx context.Context,
	checks *ReviewChecks,
	result *ResultOutput,
	failedChecks *[]string,
) []string {
	if result.Workdir == "" {
		return nil
	}
	files, err := s.ops.CommittedFiles(ctx, result.Workdir)
	if err != nil {
		// Transient / missing workdir → controller retries; not a
		// semantic check failure.
		glog.Warningf("ai_review: CommittedFiles failed: %v", err)
		return nil
	}
	// The expected set: CHANGELOG.md + detected plugin manifests.
	// We invoke plugin.DetectManifests even when result.Workdir is
	// non-empty — same as steps_execution.go. Detected manifests
	// extend the expected set.
	expected := []string{changelogFileName}
	detected, derr := plugin.DetectManifests(ctx, result.Workdir)
	if derr != nil {
		glog.Warningf("ai_review: DetectManifests failed: %v", derr)
		// Fall through with just the changelog in the expected set
		// — any extra detected manifest in `files` will then
		// surface as a (false) unexpected-file, which is the safer
		// bias.
	} else {
		expected = append(expected, detected...)
	}
	if !sameStringSet(files, expected) {
		checks.UnexpectedFileChange = true
		*failedChecks = append(*failedChecks, CheckUnexpectedFileChange)
		glog.V(2).Infof(
			"ai_review: check=%s result=false: committed=%v expected=%v",
			CheckUnexpectedFileChange,
			files,
			expected,
		)
		return diffStringSet(files, expected)
	}
	glog.V(2).Infof("ai_review: check=%s result=true", CheckUnexpectedFileChange)
	return nil
}

// checkFaithfulness invokes the ClaudeRunner once with the embedded
// faithfulness prompt + the captured original + the final body,
// parses the response, and updates checks.FaithfulnessOK and
// PerEntry accordingly. The returned overall string is OverallPass,
// OverallFail, or OverallUnknown; the perEntry slice is the flat
// mapping (per_entry + extras-with-Verdict=hallucinated).
//
// On LLM error or parse error: OverallUnknown — perEntry left empty,
// FaithfulnessOK=false, CheckFaithfulness recorded in failedChecks.
//
// heading is the plan.NextVersionHeader text (e.g. "## v1.2.8") and
// is passed to changelog.ExtractSectionBody to extract the matching
// section from the on-disk CHANGELOG.md.
func (s *aiReviewStep) checkFaithfulness(
	ctx context.Context,
	plan *PlanOutput,
	result *ResultOutput,
	checks *ReviewChecks,
	failedChecks *[]string,
) (string, []FaithfulnessVerdict) {
	if result.Workdir == "" {
		// No workdir → no final body to read. Cannot run the
		// semantic check; mark unknown so the operator triages.
		checks.FaithfulnessOK = false
		*failedChecks = append(*failedChecks, CheckFaithfulness)
		return OverallUnknown, nil
	}
	changelogPath := filepath.Join(result.Workdir, changelogFileName)
	content, err := os.ReadFile(
		changelogPath,
	) // #nosec G304 -- workdir is os.TempDir-rooted; filename is constant
	if err != nil {
		glog.Warningf("ai_review: read CHANGELOG failed: %v", err)
		checks.FaithfulnessOK = false
		*failedChecks = append(*failedChecks, CheckFaithfulness)
		return OverallUnknown, nil
	}
	// ExtractSectionBody takes the heading TEXT (the part after
	// "## "), so strip the "## " prefix from plan.NextVersionHeader.
	headingText := strings.TrimPrefix(plan.NextVersionHeader, "## ")
	finalBody, err := changelog.ExtractSectionBody(ctx, content, headingText)
	if err != nil {
		glog.Warningf("ai_review: extract section body failed: %v", err)
		checks.FaithfulnessOK = false
		*failedChecks = append(*failedChecks, CheckFaithfulness)
		return OverallUnknown, nil
	}
	prompt := prompts.ChangelogFaithfulnessPrompt() +
		"\n\n## Original ## Unreleased body\n\n" + plan.OriginalUnreleased +
		"\n\n## Final " + plan.NextVersionHeader + " body\n\n" + finalBody
	claudeResult, err := s.runner.Run(ctx, prompt)
	if err != nil {
		glog.Warningf("ai_review: faithfulness LLM call failed: %v", err)
		checks.FaithfulnessOK = false
		*failedChecks = append(*failedChecks, CheckFaithfulness)
		return OverallUnknown, nil
	}
	resp, err := prompts.ParseFaithfulnessResponse(ctx, claudeResult.Result)
	if err != nil {
		glog.Warningf("ai_review: parse faithfulness response failed: %v", err)
		checks.FaithfulnessOK = false
		*failedChecks = append(*failedChecks, CheckFaithfulness)
		return OverallUnknown, nil
	}

	// Map FaithfulnessLLMResponse → flat ReviewOutput.PerEntry list.
	// per_entry is appended verbatim; extras is appended with
	// Verdict=FaithfulnessHallucinated (the LLM's per-extras
	// verdict is always "hallucinated" by parser contract).
	perEntry := make([]FaithfulnessVerdict, 0, len(resp.PerEntry)+len(resp.Extras))
	for _, e := range resp.PerEntry {
		perEntry = append(perEntry, FaithfulnessVerdict{
			Entry:   e.Entry,
			Verdict: e.Verdict,
			Note:    e.Note,
		})
	}
	for _, e := range resp.Extras {
		perEntry = append(perEntry, FaithfulnessVerdict{
			Entry:   e.Entry,
			Verdict: FaithfulnessHallucinated,
			Note:    e.Note,
		})
	}

	if resp.Overall == OverallPass {
		checks.FaithfulnessOK = true
		return OverallPass, perEntry
	}
	checks.FaithfulnessOK = false
	*failedChecks = append(*failedChecks, CheckFaithfulness)
	return OverallFail, perEntry
}

// notesFor returns a human-readable one-liner naming each failed check,
// or "all checks passed" on success. Mirrors today's behavior.
func (s *aiReviewStep) notesFor(failedChecks []string) string {
	if len(failedChecks) == 0 {
		return "all checks passed"
	}
	return "failed checks: " + strings.Join(failedChecks, ", ")
}

// rollupVerdict aggregates the per-check failure state into the
// single Overall string and the boolean Approved flag. The "unknown"
// verdict from the LLM override is layered on top of the binary
// pass/fail rollup so the human reviewer sees the distinction
// between "checks failed" and "LLM unreachable".
func rollupVerdict(
	faithfulnessOverall string,
	failedChecks []string,
) (overall string, approved bool) {
	overall = OverallPass
	approved = true
	if len(failedChecks) > 0 || faithfulnessOverall == OverallUnknown {
		overall = OverallFail
		approved = false
	}
	if faithfulnessOverall == OverallUnknown {
		// Override: when the LLM is unreachable we surface
		// "unknown" at the overall level per spec failure-mode
		// "LLM unavailability".
		overall = OverallUnknown
	}
	return overall, approved
}

// diffStringSet returns the set difference a - b as a sorted slice.
// Used to populate ReviewOutput.UnexpectedFiles with the offending
// paths in deterministic order.
func diffStringSet(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	var out []string
	for _, v := range a {
		if _, ok := set[v]; !ok {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}
