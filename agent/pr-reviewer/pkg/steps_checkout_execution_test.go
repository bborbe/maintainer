// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"fmt"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/mocks"
	pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

var _ = Describe("checkoutExecutionStep", func() {
	var (
		ctx         context.Context
		repoManager *mocks.RepoManager
		step        agentlib.Step
	)

	BeforeEach(func() {
		ctx = context.Background()
		repoManager = &mocks.RepoManager{}
		step = pkg.NewCheckoutExecutionStep(
			repoManager,
			"",
			"agent",
			"sonnet",
			map[string]string{},
			claudelib.AllowedTools{"Read"},
			"standard",
			nil,
			nil,
		)
	})

	Describe("Name", func() {
		It("returns pr-execute", func() {
			Expect(step.Name()).To(Equal("pr-execute"))
		})
	})

	Describe("ShouldRun", func() {
		DescribeTable("decides based on existing ## Review section",
			func(content string, expected bool) {
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.ShouldRun(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("no review section", "# PR Review\n\nsome text", true),
			Entry("review section present", "# PR Review\n\n## Review\n\n{}", false),
			Entry("empty content", "", true),
		)
	})

	Describe("Run", func() {
		Context("when clone_url is missing from frontmatter", func() {
			It("returns AgentStatusFailed without propagating error", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nref: main\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.Message).To(ContainSubstring("clone_url"))
			})
		})

		Context("when ref is missing from frontmatter", func() {
			It("returns AgentStatusFailed without propagating error", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/example/repo.git\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.Message).To(ContainSubstring("ref"))
			})
		})

		Context("when base_ref is missing from frontmatter", func() {
			It("returns AgentStatusFailed without propagating error", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/example/repo.git\nref: main\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.Message).To(ContainSubstring("base_ref"))
			})
		})

		Context("when EnsureWorktree returns an error", func() {
			It("propagates the error (fail loud)", func() {
				repoManager.EnsureWorktreeReturns("", fmt.Errorf("clone failed: network error"))

				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/example/repo.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, runErr := step.Run(ctx, md)
				Expect(runErr).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(runErr.Error()).To(ContainSubstring("ensure worktree"))
			})
		})

		Context("when EnsureWorktree fails with a git auth-failure error", func() {
			BeforeEach(func() {
				repoManager.EnsureWorktreeReturns(
					"",
					fmt.Errorf(
						"git clone --bare: fatal: could not read Username for 'https://github.com': terminal prompts disabled",
					),
				)
			})

			It("returns AgentStatusNeedsInput", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
			})

			It("diagnostic names host/owner/repo", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, _ := step.Run(ctx, md)
				Expect(result.Message).To(ContainSubstring("github.com/bborbe/trading"))
			})

			It("diagnostic contains GH_TOKEN hint", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, _ := step.Run(ctx, md)
				Expect(result.Message).To(ContainSubstring("GH_TOKEN"))
			})

			It("diagnostic does NOT leak the underlying git error (token non-leakage)", func() {
				// Inject a distinctive fake token into the underlying clone error.
				// The diagnostic uses a fixed template and must not echo err.Error(),
				// so the fake token must NOT appear in result.Message.
				const fakeToken = "FAKE_TOKEN_DO_NOT_LEAK_xyz123" //nolint:gosec // G101: test-only sentinel value, not a real credential
				repoManager.EnsureWorktreeReturns(
					"",
					fmt.Errorf(
						"git clone --bare: fatal: could not read Username for 'https://%s@github.com': terminal prompts disabled",
						fakeToken,
					),
				)
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, runErr := step.Run(ctx, md)
				Expect(runErr).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
				Expect(result.Message).NotTo(ContainSubstring(fakeToken))
			})
		})

		Context("when EnsureWorktree fails with 'Repository not found'", func() {
			// GitHub returns this exact string for unauthenticated requests to private
			// repos. Intentionally classified as auth failure; known false-positive on
			// typo'd public repo URLs (operator can verify URL when re-triggering).
			BeforeEach(func() {
				repoManager.EnsureWorktreeReturns(
					"",
					fmt.Errorf(
						"git clone --bare: remote: Repository not found.\nfatal: repository 'https://github.com/bborbe/private.git/' not found",
					),
				)
			})

			It("returns AgentStatusNeedsInput", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/bborbe/private.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, runErr := step.Run(ctx, md)
				Expect(runErr).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
				Expect(result.Message).To(ContainSubstring("github.com/bborbe/private"))
				Expect(result.Message).To(ContainSubstring("GH_TOKEN"))
			})
		})

		Context("when EnsureWorktree fails with a non-auth error", func() {
			BeforeEach(func() {
				repoManager.EnsureWorktreeReturns(
					"",
					fmt.Errorf(
						"git clone --bare: unable to access 'https://github.com/bborbe/foo.git/': Could not resolve host: github.com",
					),
				)
			})

			It("propagates the error (not NeedsInput)", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nclone_url: https://github.com/bborbe/trading.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, runErr := step.Run(ctx, md)
				Expect(runErr).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		Context("allowlist checks", func() {
			const taskMarkdown = "---\nclone_url: https://github.com/bborbe/maintainer.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n"

			Context("when allowlist is empty", func() {
				It("proceeds to EnsureWorktree (allow-all behavior)", func() {
					stepWithEmpty := pkg.NewCheckoutExecutionStep(
						repoManager,
						"",
						"agent",
						"sonnet",
						map[string]string{},
						claudelib.AllowedTools{"Read"},
						"standard",
						nil,
						nil,
					)
					repoManager.EnsureWorktreeReturns("", fmt.Errorf("stop here"))

					md, err := agentlib.ParseMarkdown(ctx, taskMarkdown)
					Expect(err).NotTo(HaveOccurred())
					_, runErr := stepWithEmpty.Run(ctx, md)
					Expect(repoManager.EnsureWorktreeCallCount()).To(Equal(1))
					Expect(runErr).To(HaveOccurred())
				})
			})

			Context("when allowlist is non-empty and clone_url matches", func() {
				It("proceeds to EnsureWorktree", func() {
					stepWithAllowlist := pkg.NewCheckoutExecutionStep(
						repoManager,
						"",
						"agent",
						"sonnet",
						map[string]string{},
						claudelib.AllowedTools{"Read"},
						"standard",
						[]string{"github.com/bborbe/maintainer"},
						nil,
					)
					repoManager.EnsureWorktreeReturns("", fmt.Errorf("stop here"))

					md, err := agentlib.ParseMarkdown(ctx, taskMarkdown)
					Expect(err).NotTo(HaveOccurred())
					_, runErr := stepWithAllowlist.Run(ctx, md)
					Expect(repoManager.EnsureWorktreeCallCount()).To(Equal(1))
					Expect(runErr).To(HaveOccurred())
				})
			})

			Context("when allowlist is non-empty and clone_url does NOT match", func() {
				It("returns NeedsInput and does not call EnsureWorktree", func() {
					stepWithAllowlist := pkg.NewCheckoutExecutionStep(
						repoManager,
						"",
						"agent",
						"sonnet",
						map[string]string{},
						claudelib.AllowedTools{"Read"},
						"standard",
						[]string{"github.com/bborbe/other-repo"},
						nil,
					)
					const nonMatchingTask = "---\nclone_url: https://github.com/bborbe/maintainer.git\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n"

					md, err := agentlib.ParseMarkdown(ctx, nonMatchingTask)
					Expect(err).NotTo(HaveOccurred())
					result, runErr := stepWithAllowlist.Run(ctx, md)
					Expect(runErr).NotTo(HaveOccurred())
					Expect(result).NotTo(BeNil())
					Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
					Expect(result.Message).To(ContainSubstring("github.com/bborbe/maintainer"))
					Expect(repoManager.EnsureWorktreeCallCount()).To(Equal(0))
				})
			})

			Context("when allowlist is non-empty and clone_url is unparseable", func() {
				It("returns Failed (not NeedsInput) and does not call EnsureWorktree", func() {
					stepWithAllowlist := pkg.NewCheckoutExecutionStep(
						repoManager,
						"",
						"agent",
						"sonnet",
						map[string]string{},
						claudelib.AllowedTools{"Read"},
						"standard",
						[]string{"github.com/bborbe/maintainer"},
						nil,
					)
					const badURLTask = "---\nclone_url: not-a-url\nref: main\nbase_ref: master\ntask_identifier: bd4d883b-0000-0000-0000-000000000001\n---\n# Task\n"

					md, err := agentlib.ParseMarkdown(ctx, badURLTask)
					Expect(err).NotTo(HaveOccurred())
					result, runErr := stepWithAllowlist.Run(ctx, md)
					Expect(runErr).NotTo(HaveOccurred())
					Expect(result).NotTo(BeNil())
					Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
					Expect(result.Message).To(ContainSubstring("failed to parse clone_url"))
					Expect(repoManager.EnsureWorktreeCallCount()).To(Equal(0))
				})
			})
		})
	})

	Describe("posting behavior", func() {
		const (
			prURL      = "https://github.com/bborbe/maintainer/pull/2"
			reviewBody = "LGTM. All checks pass.\n\n{\"verdict\":\"approve\",\"reason\":\"LGTM\"}"
			taskMD     = "---\nref: abc123\ntrigger_count: 1\n---\n\nReview the pull request at " + prURL + ".\n"
		)

		buildMD := func(ctx context.Context, reviewSection string) *agentlib.Markdown {
			md, err := agentlib.ParseMarkdown(ctx, taskMD)
			Expect(err).NotTo(HaveOccurred())
			if reviewSection != "" {
				md.ReplaceSection(agentlib.Section{
					Heading: "## Review",
					Body:    reviewSection,
				})
			}
			return md
		}

		fixedTime := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

		Context("when poster is nil", func() {
			It("advances to ai_review without calling any poster", func() {
				md := buildMD(ctx, reviewBody)
				result, err := pkg.PostAndRouteForTest(ctx, nil, md, prURL, "", fixedTime)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("ai_review"))
			})
		})

		Context("when post succeeds", func() {
			It("advances to ai_review and writes a success diagnostic", func() {
				fakePoster := &mocks.PrPoster{}
				fakePoster.PostReturns(pkg.PostResult{Outcome: "success", ReviewID: 42})

				md := buildMD(ctx, reviewBody)
				result, err := pkg.PostAndRouteForTest(ctx, fakePoster, md, prURL, "", fixedTime)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.NextPhase).To(Equal("ai_review"))
				Expect(fakePoster.PostCallCount()).To(Equal(1))

				diagSection, ok := md.FindSection("## Diagnostics")
				Expect(ok).To(BeTrue())
				Expect(diagSection.Body).To(ContainSubstring("outcome: success"))
				Expect(diagSection.Body).To(ContainSubstring("review_id: 42"))
			})
		})

		Context("when post fails with a transient error", func() {
			It("escalates to human_review and writes a failure diagnostic", func() {
				fakePoster := &mocks.PrPoster{}
				fakePoster.PostReturns(pkg.PostResult{
					Outcome:      "failed",
					Class:        pkg.ErrorClassTransient,
					ErrorMessage: "timeout",
					FailureStep:  "post",
				})

				md := buildMD(ctx, reviewBody)
				result, err := pkg.PostAndRouteForTest(ctx, fakePoster, md, prURL, "", fixedTime)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.NextPhase).To(Equal("human_review"))
				Expect(result.Message).To(ContainSubstring("posting failed"))

				diagSection, ok := md.FindSection("## Diagnostics")
				Expect(ok).To(BeTrue())
				Expect(diagSection.Body).To(ContainSubstring("class: transient"))
			})
		})

		Context("when post returns not-a-failure class (e.g. 422 PR closed)", func() {
			It("advances to ai_review", func() {
				fakePoster := &mocks.PrPoster{}
				fakePoster.PostReturns(pkg.PostResult{
					Outcome: "success",
					Class:   pkg.ErrorClassNotAFailure,
				})

				md := buildMD(ctx, reviewBody)
				result, err := pkg.PostAndRouteForTest(ctx, fakePoster, md, prURL, "", fixedTime)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.NextPhase).To(Equal("ai_review"))
			})
		})

		Context("## Review vault preserved regardless of poster outcome", func() {
			DescribeTable("review body unchanged for every ErrorClass",
				func(class pkg.ErrorClass, outcome string) {
					fakePoster := &mocks.PrPoster{}
					postResult := pkg.PostResult{
						Outcome: outcome,
						Class:   class,
					}
					if outcome == "failed" {
						postResult.ErrorMessage = "test error"
					}
					fakePoster.PostReturns(postResult)

					md := buildMD(ctx, reviewBody)
					_, err := pkg.PostAndRouteForTest(ctx, fakePoster, md, prURL, "", fixedTime)
					Expect(err).NotTo(HaveOccurred())

					reviewSection, ok := md.FindSection("## Review")
					Expect(ok).To(BeTrue())
					Expect(reviewSection.Body).To(Equal(reviewBody))
				},
				Entry("transient failure", pkg.ErrorClassTransient, "failed"),
				Entry("permanent failure", pkg.ErrorClassPermanent, "failed"),
				Entry("unknown failure", pkg.ErrorClassUnknown, "failed"),
				Entry("not-a-failure", pkg.ErrorClassNotAFailure, "success"),
				Entry("soft-warning", pkg.ErrorClassSoftWarning, "success"),
			)
		})

		Context("diagnostic blocks are append-only", func() {
			It("second run appends after the first block", func() {
				fakePoster := &mocks.PrPoster{}
				fakePoster.PostReturns(pkg.PostResult{Outcome: "success", ReviewID: 1})

				md := buildMD(ctx, reviewBody)

				// First posting attempt.
				_, err := pkg.PostAndRouteForTest(ctx, fakePoster, md, prURL, "", fixedTime)
				Expect(err).NotTo(HaveOccurred())

				// Second posting attempt (simulate controller re-spawn).
				fakePoster.PostReturns(pkg.PostResult{Outcome: "success", ReviewID: 2})
				_, err = pkg.PostAndRouteForTest(
					ctx,
					fakePoster,
					md,
					prURL,
					"",
					fixedTime.Add(time.Minute),
				)
				Expect(err).NotTo(HaveOccurred())

				diagSection, ok := md.FindSection("## Diagnostics")
				Expect(ok).To(BeTrue())
				Expect(diagSection.Body).To(ContainSubstring("review_id: 1"))
				Expect(diagSection.Body).To(ContainSubstring("review_id: 2"))
			})
		})
	})
})
