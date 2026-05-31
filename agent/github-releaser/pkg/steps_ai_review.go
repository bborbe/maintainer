// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"regexp"
	"strings"

	stderrors "errors"
	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	"github.com/golang/glog"
)

// ReviewChecks holds the three boolean verification results.
type ReviewChecks struct {
	TagExists                bool `json:"tag_exists"`
	TagAtExpectedSHA         bool `json:"tag_at_expected_sha"`
	ChangelogHeaderRewritten bool `json:"changelog_header_rewritten"`
}

// ReviewOutput is the typed contract for the `## Review` JSON section the
// ai_review step writes. Round-trips with agentlib.MarshalSectionTyped +
// agentlib.ExtractSection[ReviewOutput].
type ReviewOutput struct {
	Approved bool         `json:"approved"`
	Checks   ReviewChecks `json:"checks"`
	Notes    string       `json:"notes"`
}

// ErrTagNotFound is returned by AIReviewClient.TagExists on a 404 response.
// The step uses errors.Is(err, ErrTagNotFound) to distinguish 404 (verification
// failure → write ## Review approved:false, return Status: failed) from
// 5xx / transport errors (wrap and return; controller retries).
var ErrTagNotFound = stderrors.New("ai_review: tag not found")

// AIReviewClient is the seam for the three GitHub REST API calls.
// Mock it in tests with a counterfeiter-generated mock.
type AIReviewClient interface {
	// TagExists calls GET /repos/{owner}/{repo}/git/ref/tags/{tag} and
	// returns (tagSHA, nil) on 200, or ("", ErrTagNotFound) on 404
	// (the sentinel — step distinguishes 404 → verdict vs 5xx → retry),
	// or ("", wrapped error) on transport / other non-2xx.
	TagExists(ctx context.Context, owner, repo, tag string) (tagSHA string, _ error)

	// ResolveTagCommit calls GET /repos/{owner}/{repo}/git/tags/{sha} and
	// follows annotated tags to their underlying commit SHA. Returns the
	// commit SHA or a wrapped error.
	ResolveTagCommit(ctx context.Context, owner, repo, tagSHA string) (commitSHA string, _ error)

	// FetchChangelog calls GET /repos/{owner}/{repo}/contents/CHANGELOG.md
	// (no ?ref= — relies on API defaulting to the repo's default branch).
	// Returns base64-decoded file bytes or a wrapped error.
	FetchChangelog(ctx context.Context, owner, repo string) ([]byte, error)
}

// aiReviewStep implements agentlib.Step. It performs three remote verification
// checks against the GitHub REST API and writes a ## Review section.
type aiReviewStep struct {
	client  AIReviewClient
	ghToken string
}

// NewAIReviewStep wires the ai_review step with its HTTP client seam and the
// GitHub token (used for authenticated API calls).
func NewAIReviewStep(client AIReviewClient, ghToken string) agentlib.Step {
	return &aiReviewStep{client: client, ghToken: ghToken}
}

// Name implements agentlib.Step.
func (s *aiReviewStep) Name() string { return "github-release-ai-review" }

// ShouldRun always returns true. The step is idempotent at the framework level:
// a re-trigger overwrites the existing ## Review section.
func (s *aiReviewStep) ShouldRun(_ context.Context, _ *agentlib.Markdown) (bool, error) {
	return true, nil
}

// Run executes the three verification checks. On any HTTP/transient error it
// returns a wrapped error so the controller retries. On a 404 for the tag it
// writes ## Review(approved:false) and returns Status:Failed. On any other
// verification failure it writes ## Review(approved:false) and returns
// Status:Failed. On all checks passing it writes ## Review(approved:true)
// and returns Done with NextPhase="done".
func (s *aiReviewStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
	result, err := agentlib.ExtractSection[ResultOutput](ctx, md, "## Result")
	if err != nil || result == nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: extract ## Result section")
	}

	if result.Outcome != ResultOutcomeReleased {
		output := ReviewOutput{
			Approved: true,
			Checks: ReviewChecks{
				TagExists:                true,
				TagAtExpectedSHA:         true,
				ChangelogHeaderRewritten: true,
			},
			Notes: "execution step recorded failure; nothing to verify",
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

	repo, _ := md.Frontmatter.String("repo")
	owner, name, ok := parseOwnerRepo(repo)
	if !ok {
		return nil, errors.Errorf(ctx, "ai_review: read frontmatter repo")
	}

	glog.V(2).Infof("ai_review: starting checks for repo=%s/%s tag=%s commit=%s", owner, name, result.Tag, result.CommitSHA)

	checks := ReviewChecks{
		TagExists:                true,
		TagAtExpectedSHA:         true,
		ChangelogHeaderRewritten: true,
	}

	failVerdict := func(notes string) (*agentlib.Result, error) {
		output := ReviewOutput{
			Approved: false,
			Checks:   checks,
			Notes:    notes,
		}
		section, err := agentlib.MarshalSectionTyped(ctx, "## Review", output)
		if err != nil {
			return nil, errors.Wrapf(ctx, err, "ai_review: marshal ## Review section")
		}
		md.ReplaceSection(section)
		return &agentlib.Result{
			Status:  agentlib.AgentStatusFailed,
			Message: notes,
		}, nil
	}

	// Check 1: tag exists
	tagSHA, err := s.client.TagExists(ctx, owner, name, result.Tag)
	if err != nil {
		if errors.Is(err, ErrTagNotFound) {
			checks.TagExists = false
			glog.V(2).Infof("ai_review: check=TagExists result=false: %v", err)
			return failVerdict("tag " + result.Tag + " not found on remote")
		}
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return nil, errors.Wrapf(ctx, err, "ai_review: TagExists")
	}
	glog.V(2).Infof("ai_review: check=TagExists result=true")

	// Check 2: tag points to expected commit
	commitSHA, err := s.client.ResolveTagCommit(ctx, owner, name, tagSHA)
	if err != nil {
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return nil, errors.Wrapf(ctx, err, "ai_review: ResolveTagCommit")
	}
	if commitSHA != result.CommitSHA {
		checks.TagAtExpectedSHA = false
		glog.V(2).Infof("ai_review: check=TagAtExpectedSHA result=false: tag points to %s, expected %s", commitSHA, result.CommitSHA)
		return failVerdict("tag points to " + commitSHA + ", expected " + result.CommitSHA)
	}
	glog.V(2).Infof("ai_review: check=TagAtExpectedSHA result=true")

	// Check 3: CHANGELOG.md top section rewritten
	changelogBytes, err := s.client.FetchChangelog(ctx, owner, name)
	if err != nil {
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return nil, errors.Wrapf(ctx, err, "ai_review: FetchChangelog")
	}
	if !s.changelogHeaderRewritten(changelogBytes) {
		checks.ChangelogHeaderRewritten = false
		glog.V(2).Infof("ai_review: check=ChangelogHeaderRewritten result=false")
		return failVerdict("CHANGELOG.md top section is still ## Unreleased on default branch")
	}
	glog.V(2).Infof("ai_review: check=ChangelogHeaderRewritten result=true")

	// All checks passed
	output := ReviewOutput{
		Approved: true,
		Checks:   checks,
		Notes:    "all checks passed",
	}
	section, err := agentlib.MarshalSectionTyped(ctx, "## Review", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: marshal ## Review section")
	}
	md.ReplaceSection(section)

	glog.V(2).Infof("ai_review: all checks passed")
	return &agentlib.Result{
		Status:    agentlib.AgentStatusDone,
		NextPhase: "done",
	}, nil
}

// changelogHeaderRewritten returns true if the first ## heading in content
// is NOT "## Unreleased" (i.e. the header has been rewritten to a version).
// Splits on newlines and finds the first line matching ^##\s+.
func (s *aiReviewStep) changelogHeaderRewritten(content []byte) bool {
	lines := strings.Split(string(content), "\n")
	headingRE := regexp.MustCompile(`^##\s+`)
	for _, line := range lines {
		if headingRE.MatchString(line) {
			trimmed := strings.TrimSpace(line)
			return trimmed != "## Unreleased"
		}
	}
	// No ## heading found — treat as not rewritten
	return false
}
