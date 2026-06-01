// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentlib "github.com/bborbe/agent/lib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/mocks"
	pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/githubreview"
)

var _ = Describe("AIReviewStep", func() {

	var (
		fakeClient *mocks.ReviewClient
		token      string
		step       agentlib.Step
	)

	BeforeEach(func() {
		fakeClient = &mocks.ReviewClient{}
		token = "test-token"
		step = pkg.NewAIReviewStep(fakeClient, token)
	})

	// taskWithResult builds a task markdown with ## Result section.
	// Backticks cannot appear in Go raw string literals, so we build the
	// fenced JSON blocks via string concatenation.
	taskWithResult := func(commitSHA, tag, outcome string) string {
		const fm = "---\n" +
			"status: in_progress\n" +
			"phase: ai_review\n" +
			"assignee: github-releaser-agent\n" +
			"task_type: github-release\n" +
			"repo: bborbe/example\n" +
			"task_identifier: gh-release-001\n" +
			"---\n\n"
		plan := "## Plan\n\n" +
			"```json\n" +
			`{"outcome":"ready","next_version":"1.0.0","next_version_header":"## v1.0.0"}` + "\n" +
			"```\n\n"
		result := "## Result\n\n" +
			"```json\n" +
			fmt.Sprintf(
				`{"outcome":%q,"path":"direct-push","commit_sha":%q,"tag":%q}`,
				outcome,
				commitSHA,
				tag,
			) + "\n" +
			"```\n"
		return fm + plan + result
	}

	taskWithFailedResult := func() string {
		const fm = "---\n" +
			"status: in_progress\n" +
			"phase: ai_review\n" +
			"assignee: github-releaser-agent\n" +
			"task_type: github-release\n" +
			"repo: bborbe/example\n" +
			"task_identifier: gh-release-001\n" +
			"---\n\n"
		result := "## Result\n\n" +
			"```json\n" +
			`{"outcome":"failed","error_category":"unknown","error":"clone failed"}` + "\n" +
			"```\n"
		return fm + result
	}

	taskWithMalformedResult := func() string {
		const fm = "---\n" +
			"status: in_progress\n" +
			"phase: ai_review\n" +
			"assignee: github-releaser-agent\n" +
			"task_type: github-release\n" +
			"repo: bborbe/example\n" +
			"task_identifier: gh-release-001\n" +
			"---\n\n"
		result := "## Result\n\n" +
			"```json\n" +
			`{"outcome": "released", "invalid-json` + "\n" +
			"```\n"
		return fm + result
	}

	taskWithoutResult := func() string {
		return "---\n" +
			"status: in_progress\n" +
			"phase: ai_review\n" +
			"assignee: github-releaser-agent\n" +
			"task_type: github-release\n" +
			"repo: bborbe/example\n" +
			"task_identifier: gh-release-001\n" +
			"---\n\n" +
			"## Plan\n\n" +
			"```json\n" +
			`{"outcome":"ready"}` + "\n" +
			"```\n"
	}

	taskWithoutRepo := func(outcome string) string {
		const fm = "---\n" +
			"status: in_progress\n" +
			"phase: ai_review\n" +
			"assignee: github-releaser-agent\n" +
			"task_type: github-release\n" +
			"task_identifier: gh-release-001\n" +
			"---\n\n"
		result := "## Result\n\n" +
			"```json\n" +
			fmt.Sprintf(
				`{"outcome":%q,"path":"direct-push","commit_sha":"abc123","tag":"v1.0.0"}`,
				outcome,
			) + "\n" +
			"```\n"
		return fm + result
	}

	// runStep is used for tests where the step is expected to return (result, nil).
	runStep := func(taskMD string) (*agentlib.Result, *agentlib.Markdown) {
		md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
		Expect(err).NotTo(HaveOccurred())
		result, err := step.Run(context.Background(), md)
		Expect(err).NotTo(HaveOccurred())
		return result, md
	}

	// extractReview calls ExtractSection and returns the pointer result.
	// Fails the test if the section is missing or malformed.
	extractReview := func(md *agentlib.Markdown) *pkg.ReviewOutput {
		review, err := agentlib.ExtractSection[pkg.ReviewOutput](
			context.Background(),
			md,
			"## Review",
		)
		Expect(err).NotTo(HaveOccurred())
		return review
	}

	Describe("Name", func() {
		It("returns github-release-ai-review", func() {
			Expect(step.Name()).To(Equal("github-release-ai-review"))
		})
	})

	Describe("ShouldRun", func() {
		It("returns true, nil (always runs, idempotent overwrite)", func() {
			md, err := agentlib.ParseMarkdown(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())
			shouldRun, err := step.ShouldRun(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())
			Expect(shouldRun).To(BeTrue())
		})
	})

	Describe("Run", func() {
		Context("7a. Happy path", func() {
			It("all three checks pass → approved:true, status:done, next_phase:done", func() {
				fakeClient.TagExistsReturns("abc123", nil)
				fakeClient.ResolveTagCommitReturns("abc123", nil)
				fakeClient.FetchChangelogReturns(
					[]byte("## v1.0.0\n\n- feat\n\n## Unreleased\n\n- old"),
					nil,
				)

				result, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))

				review := extractReview(md)
				Expect(review.Approved).To(BeTrue())
				Expect(review.Checks.TagExists).To(BeTrue())
				Expect(review.Checks.TagAtExpectedSHA).To(BeTrue())
				Expect(review.Checks.ChangelogHeaderRewritten).To(BeTrue())
				Expect(review.Notes).To(ContainSubstring("passed"))
			})
		})

		Context("7b. Tag missing (404)", func() {
			It("ErrTagNotFound → approved:false, status:failed, next_phase:empty", func() {
				fakeClient.TagExistsReturns("", githubreview.ErrTagNotFound)

				result, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				Expect(fakeClient.ResolveTagCommitCallCount()).To(Equal(0))
				Expect(fakeClient.FetchChangelogCallCount()).To(Equal(0))

				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.NextPhase).To(BeEmpty())

				review := extractReview(md)
				Expect(review.Approved).To(BeFalse())
				Expect(review.Checks.TagExists).To(BeFalse())
				// Other checks vacuously true (struct initialised all-true)
				Expect(review.Checks.TagAtExpectedSHA).To(BeTrue())
				Expect(review.Checks.ChangelogHeaderRewritten).To(BeTrue())
				Expect(review.Notes).To(ContainSubstring("not found"))
			})
		})

		Context("7b-bis. Tag check returns transient 5xx error", func() {
			It("non-sentinel error → step returns wrapped error; no ## Review written", func() {
				fakeClient.TagExistsReturns("",
					errors.New("TagExists: status 500: Server Error"))

				md, err := agentlib.ParseMarkdown(context.Background(),
					taskWithResult("abc123", "v1.0.0", "released"))
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(context.Background(), md)

				// 5xx → step returns nil result with wrapped error (controller retries)
				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				// Error chain contains the wrap message
				Expect(err.Error()).To(ContainSubstring("ai_review: TagExists"))

				// No ## Review section written on retry-able error path
				review, err := agentlib.ExtractSection[pkg.ReviewOutput](
					context.Background(), md, "## Review")
				Expect(err).To(HaveOccurred())
				Expect(review).To(BeNil())
			})
		})

		Context("7c. Annotated tag SHA mismatch", func() {
			It("tag exists but points to different commit → tag_at_expected_sha:false", func() {
				fakeClient.TagExistsReturns("tag-sha-annotated", nil)
				fakeClient.ResolveTagCommitReturns("different-commit-sha", nil)

				result, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				// FetchChangelog not called (short-circuits after SHA mismatch)
				Expect(fakeClient.FetchChangelogCallCount()).To(Equal(0))

				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				review := extractReview(md)
				Expect(review.Approved).To(BeFalse())
				Expect(review.Checks.TagExists).To(BeTrue())
				Expect(review.Checks.TagAtExpectedSHA).To(BeFalse())
				Expect(review.Checks.ChangelogHeaderRewritten).To(BeTrue())
			})
		})

		Context("7d. Lightweight tag SHA mismatch", func() {
			It("lightweight tag points to different commit → tag_at_expected_sha:false", func() {
				fakeClient.TagExistsReturns("tag-sha-lightweight", nil)
				fakeClient.ResolveTagCommitReturns("different-commit-sha", nil)

				result, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				review := extractReview(md)
				Expect(review.Approved).To(BeFalse())
				Expect(review.Checks.TagAtExpectedSHA).To(BeFalse())
			})
		})

		Context("7d-bis. Short-vs-full SHA equivalence (regression for prod-1)", func() {
			// Bug observed in prod 2026-06-01: execution step writes
			// Result.CommitSHA via `git rev-parse --short HEAD` (7 chars),
			// GitHub API returns 40 chars. Naive == compare false-positived
			// every release. Fix: bidirectional strings.HasPrefix match.
			It("short stored vs full from API → matches → approved:true", func() {
				short := "dcd3195"
				full := "dcd3195e3cca37862f4e612a7b14c4e00af6b935"
				fakeClient.TagExistsReturns("tag-sha", nil)
				fakeClient.ResolveTagCommitReturns(full, nil)
				fakeClient.FetchChangelogReturns(
					[]byte("## v0.9.0\n\n- feat"), nil,
				)

				result, md := runStep(taskWithResult(short, "v0.9.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))

				review := extractReview(md)
				Expect(review.Approved).To(BeTrue())
				Expect(review.Checks.TagAtExpectedSHA).To(BeTrue())
			})

			It("full stored vs short from API → matches → approved:true", func() {
				short := "dcd3195"
				full := "dcd3195e3cca37862f4e612a7b14c4e00af6b935"
				fakeClient.TagExistsReturns("tag-sha", nil)
				fakeClient.ResolveTagCommitReturns(short, nil)
				fakeClient.FetchChangelogReturns(
					[]byte("## v0.9.0\n\n- feat"), nil,
				)

				result, md := runStep(taskWithResult(full, "v0.9.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))

				review := extractReview(md)
				Expect(review.Approved).To(BeTrue())
				Expect(review.Checks.TagAtExpectedSHA).To(BeTrue())
			})

			It("short prefix that does NOT match full → fails via tag-points-to error path", func() {
				short := "dcd3195"
				// 40-char hex SHA (GitHub never returns anything else); the
				// 7-char prefix `abc1234` does NOT match `dcd3195`.
				full := "abc1234ffff37862f4e612a7b14c4e00af6b935"
				fakeClient.TagExistsReturns("tag-sha", nil)
				fakeClient.ResolveTagCommitReturns(full, nil)

				result, md := runStep(taskWithResult(short, "v0.9.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				review := extractReview(md)
				Expect(review.Approved).To(BeFalse())
				Expect(review.Checks.TagAtExpectedSHA).To(BeFalse())
				// Assert the SHA-mismatch error path executed (not a different
				// failure mode) — the prod regression was silent execution of
				// the WRONG path; this assertion proves the right one fired.
				Expect(review.Notes).To(ContainSubstring("tag points to"))
				Expect(review.Notes).To(ContainSubstring(full))
				Expect(review.Notes).To(ContainSubstring(short))
			})
		})

		Context("7e. CHANGELOG still has ## Unreleased as top heading", func() {
			It("changelog_header_rewritten:false → status:failed", func() {
				fakeClient.TagExistsReturns("abc123", nil)
				fakeClient.ResolveTagCommitReturns("abc123", nil)
				fakeClient.FetchChangelogReturns(
					[]byte("## Unreleased\n\n- new\n\n## v0.9.0\n\n- old"),
					nil,
				)

				result, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				review := extractReview(md)
				Expect(review.Approved).To(BeFalse())
				Expect(review.Checks.TagExists).To(BeTrue())
				Expect(review.Checks.TagAtExpectedSHA).To(BeTrue())
				Expect(review.Checks.ChangelogHeaderRewritten).To(BeFalse())
			})
		})

		Context("7f. CHANGELOG top heading is a version (pass case)", func() {
			It("header rewritten to version → approved:true, status:done", func() {
				fakeClient.TagExistsReturns("abc123", nil)
				fakeClient.ResolveTagCommitReturns("abc123", nil)
				fakeClient.FetchChangelogReturns(
					[]byte("## v1.0.0\n\n- feat\n\n## Unreleased\n\n- old"),
					nil,
				)

				result, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				review := extractReview(md)
				Expect(review.Approved).To(BeTrue())
			})
		})

		Context("7g. Short-circuit: Result.outcome != released", func() {
			It("no HTTP calls, approved:true, status:done, next_phase:done", func() {
				result, md := runStep(taskWithFailedResult())

				Expect(fakeClient.TagExistsCallCount()).To(Equal(0))
				Expect(fakeClient.ResolveTagCommitCallCount()).To(Equal(0))
				Expect(fakeClient.FetchChangelogCallCount()).To(Equal(0))

				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))

				review := extractReview(md)
				Expect(review.Approved).To(BeTrue())
				Expect(review.Checks.TagExists).To(BeTrue())
				Expect(review.Checks.TagAtExpectedSHA).To(BeTrue())
				Expect(review.Checks.ChangelogHeaderRewritten).To(BeTrue())
				Expect(review.Notes).To(ContainSubstring("nothing to verify"))
			})
		})

		Context("7h. Malformed ## Result JSON", func() {
			It("step returns wrapped error, no ## Review written", func() {
				md, err := agentlib.ParseMarkdown(context.Background(), taskWithMalformedResult())
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(context.Background(), md)

				// Malformed JSON → step returns wrapped error (controller retries)
				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("ai_review: extract ## Result section"))

				// No ## Review section written
				review, err := agentlib.ExtractSection[pkg.ReviewOutput](
					context.Background(), md, "## Review")
				Expect(err).To(HaveOccurred())
				Expect(review).To(BeNil())
			})
		})

		Context("7i. Missing ## Result section", func() {
			It("step returns wrapped error, no ## Review", func() {
				md, err := agentlib.ParseMarkdown(context.Background(), taskWithoutResult())
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(context.Background(), md)

				// Missing section → step returns wrapped error (controller retries)
				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("ai_review: extract ## Result section"))

				// No ## Review section written
				review, err := agentlib.ExtractSection[pkg.ReviewOutput](
					context.Background(), md, "## Review")
				Expect(err).To(HaveOccurred())
				Expect(review).To(BeNil())
			})
		})

		Context("7j. Missing frontmatter repo", func() {
			It("step returns wrapped error mentioning 'read frontmatter repo'", func() {
				md, err := agentlib.ParseMarkdown(context.Background(),
					taskWithoutRepo("released"))
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(context.Background(), md)

				// Missing repo → step returns error
				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("read frontmatter repo"))
			})
		})

		Context("7k. Bearer token never in error strings", func() {
			It("error string does not contain test-token", func() {
				fakeClient.TagExistsReturns("",
					errors.New("TagExists: status 500: Server Error"))

				md, err := agentlib.ParseMarkdown(context.Background(),
					taskWithResult("abc123", "v1.0.0", "released"))
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(context.Background(), md)

				// 5xx error returns nil result with wrapped error
				Expect(result).To(BeNil())
				Expect(err).NotTo(BeNil())
				Expect(strings.Contains(err.Error(), "test-token")).To(BeFalse())
			})
		})

		Context("7l. Step does NOT write ## Failure section", func() {
			It("failure case has no ## Failure section in markdown", func() {
				fakeClient.TagExistsReturns("", githubreview.ErrTagNotFound)

				_, md := runStep(taskWithResult("abc123", "v1.0.0", "released"))

				fullMarkdown, err := md.Marshal(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(fullMarkdown).NotTo(ContainSubstring("## Failure"))
			})
		})
	})
})
