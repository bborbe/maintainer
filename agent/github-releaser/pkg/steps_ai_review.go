// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"regexp"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	"github.com/golang/glog"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubreview"
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

// changelogHeadingRE matches a level-2 markdown heading. Compiled once at
// package load — the regex is invariant.
var changelogHeadingRE = regexp.MustCompile(`^##\s+`)

// NewAIReviewStep wires the ai_review step with its GitHub REST API client
// and the GitHub token (used for authenticated API calls).
func NewAIReviewStep(client githubreview.Client, ghToken string) agentlib.Step {
	return &aiReviewStep{client: client, ghToken: ghToken}
}

// aiReviewStep implements agentlib.Step. It performs three remote verification
// checks against the GitHub REST API and writes a ## Review section.
type aiReviewStep struct {
	client  githubreview.Client
	ghToken string
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
		return s.writeReviewSection(ctx, md, ReviewOutput{
			Approved: true,
			Checks: ReviewChecks{
				TagExists:                true,
				TagAtExpectedSHA:         true,
				ChangelogHeaderRewritten: true,
			},
			Notes: "execution step recorded failure; nothing to verify",
		})
	}

	repo, _ := md.Frontmatter.String("repo")
	owner, name, ok := parseOwnerRepo(repo)
	if !ok {
		return nil, errors.Errorf(ctx, "ai_review: read frontmatter repo")
	}

	glog.V(2).
		Infof("ai_review: starting checks for repo=%s/%s tag=%s commit=%s", owner, name, result.Tag, result.CommitSHA)

	checks := ReviewChecks{TagExists: true, TagAtExpectedSHA: true, ChangelogHeaderRewritten: true}

	var tagSHA string
	if tagSHA, err = s.verifyTagExists(ctx, owner, name, result.Tag, &checks); err != nil {
		if errors.Is(err, githubreview.ErrTagNotFound) {
			return s.writeReviewSection(
				ctx,
				md,
				ReviewOutput{
					Approved: false,
					Checks:   checks,
					Notes:    "tag " + result.Tag + " not found on remote",
				},
			)
		}
		return nil, errors.Wrapf(ctx, err, "ai_review: TagExists")
	}

	if err := s.verifyTagAtExpectedCommit(ctx, owner, name, tagSHA, result.CommitSHA, &checks); err != nil {
		return s.writeReviewSection(
			ctx,
			md,
			ReviewOutput{Approved: false, Checks: checks, Notes: err.Error()},
		)
	}

	if err := s.verifyChangelogHeaderRewritten(ctx, owner, name, &checks); err != nil {
		return s.writeReviewSection(
			ctx,
			md,
			ReviewOutput{Approved: false, Checks: checks, Notes: err.Error()},
		)
	}

	return s.writeReviewSection(ctx, md, ReviewOutput{
		Approved: true,
		Checks:   checks,
		Notes:    "all checks passed",
	})
}

// writeReviewSection marshals output as a ## Review section and replaces it in md.
func (s *aiReviewStep) writeReviewSection(
	ctx context.Context,
	md *agentlib.Markdown,
	output ReviewOutput,
) (*agentlib.Result, error) {
	section, err := agentlib.MarshalSectionTyped(ctx, "## Review", output)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "ai_review: marshal ## Review section")
	}
	md.ReplaceSection(section)
	if output.Approved {
		return &agentlib.Result{Status: agentlib.AgentStatusDone, NextPhase: "done"}, nil
	}
	return &agentlib.Result{Status: agentlib.AgentStatusFailed, Message: output.Notes}, nil
}

// verifyTagExists calls TagExists API and records the result in checks.
// Returns the tagSHA on success, githubreview.ErrTagNotFound if the tag is
// missing (caller handles the verdict), or a wrapped error for transient
// failures.
func (s *aiReviewStep) verifyTagExists(
	ctx context.Context,
	owner, repo, tag string,
	checks *ReviewChecks,
) (string, error) {
	tagSHA, err := s.client.TagExists(ctx, owner, repo, tag)
	if err != nil {
		if errors.Is(err, githubreview.ErrTagNotFound) {
			checks.TagExists = false
			glog.V(2).Infof("ai_review: check=TagExists result=false: %v", err)
			return "", githubreview.ErrTagNotFound
		}
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return "", errors.Wrapf(ctx, err, "ai_review: TagExists")
	}
	glog.V(2).Infof("ai_review: check=TagExists result=true")
	return tagSHA, nil
}

// verifyTagAtExpectedCommit calls ResolveTagCommit and checks the returned SHA
// matches expectedCommit. Records the result in checks. Returns an error with a
// descriptive message if the SHA mismatch cannot be retried.
//
// Length mismatch tolerance: the execution step writes Result.CommitSHA via
// `git rev-parse --short HEAD` (7 chars by default — pkg/git/os_exec_git_ops.go),
// while the GitHub API always returns a full 40-char SHA. A naive `==` compare
// would false-positive every release. We accept either string as a prefix of
// the other to handle both directions (short stored vs full stored).
func (s *aiReviewStep) verifyTagAtExpectedCommit(
	ctx context.Context,
	owner, name, tagSHA, expectedCommit string,
	checks *ReviewChecks,
) error {
	commitSHA, err := s.client.ResolveTagCommit(ctx, owner, name, tagSHA)
	if err != nil {
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return errors.Wrapf(ctx, err, "ai_review: ResolveTagCommit")
	}
	if !commitSHAMatches(commitSHA, expectedCommit) {
		checks.TagAtExpectedSHA = false
		glog.V(2).
			Infof("ai_review: check=TagAtExpectedSHA result=false: tag points to %s, expected %s", commitSHA, expectedCommit)
		return errors.Errorf(ctx, "tag points to %s, expected %s", commitSHA, expectedCommit)
	}
	glog.V(2).Infof("ai_review: check=TagAtExpectedSHA result=true")
	return nil
}

// verifyChangelogHeaderRewritten calls FetchChangelog and checks that the top
// heading is NOT "## Unreleased". Records the result in checks.
func (s *aiReviewStep) verifyChangelogHeaderRewritten(
	ctx context.Context,
	owner, repo string,
	checks *ReviewChecks,
) error {
	changelogBytes, err := s.client.FetchChangelog(ctx, owner, repo)
	if err != nil {
		glog.V(2).Infof("ai_review: GitHub API error: %v", err)
		return errors.Wrapf(ctx, err, "ai_review: FetchChangelog")
	}
	if !s.changelogHeaderRewritten(changelogBytes) {
		checks.ChangelogHeaderRewritten = false
		glog.V(2).Infof("ai_review: check=ChangelogHeaderRewritten result=false")
		return errors.Errorf(
			ctx,
			"CHANGELOG.md top section is still ## Unreleased on default branch",
		)
	}
	glog.V(2).Infof("ai_review: check=ChangelogHeaderRewritten result=true")
	return nil
}

// changelogHeaderRewritten returns true if the first ## heading in content
// is NOT "## Unreleased" (i.e. the header has been rewritten to a version).
// Splits on newlines and finds the first line matching ^##\s+.
func (s *aiReviewStep) changelogHeaderRewritten(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		if changelogHeadingRE.MatchString(line) {
			return strings.TrimSpace(line) != "## Unreleased"
		}
	}
	return false
}

// commitSHAMatches returns true when one SHA is a prefix of the other. This
// handles the short-vs-full length asymmetry between the execution step's
// `git rev-parse --short HEAD` output (7 chars by default) and GitHub's API
// which always returns the full 40-char SHA. Both directions are accepted so
// the comparison is correct regardless of which side is shorter. An empty
// string never matches (avoids vacuous-true on missing data).
func commitSHAMatches(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}
