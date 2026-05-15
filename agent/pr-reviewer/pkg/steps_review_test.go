// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"fmt"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/mocks"
	pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

var _ = Describe("ExtractVerdict", func() {
	DescribeTable("parses verdict from various LLM response shapes",
		func(input, wantVerdict, wantReason string, wantOK bool) {
			got, err := pkg.ExtractVerdictForTest(input)
			if !wantOK {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Verdict).To(Equal(wantVerdict))
			Expect(got.Reason).To(Equal(wantReason))
		},

		Entry("raw JSON object",
			`{"verdict":"pass","reason":"all good"}`,
			"pass", "all good", true),

		Entry("JSON with leading + trailing whitespace",
			"\n\n  {\"verdict\":\"fail\",\"reason\":\"bad\"}  \n",
			"fail", "bad", true),

		Entry("JSON wrapped in ```json fence",
			"```json\n{\"verdict\":\"pass\",\"reason\":\"x\"}\n```",
			"pass", "x", true),

		Entry("JSON wrapped in plain ``` fence",
			"```\n{\"verdict\":\"fail\",\"reason\":\"y\"}\n```",
			"fail", "y", true),

		Entry(
			"prose before JSON (Claude commentary)",
			"All three checks pass:\n\n1. Concerns addressed\n2. No hallucinations\n3. Consistent\n\n{\"verdict\":\"pass\",\"reason\":\"all good\"}",
			"pass",
			"all good",
			true,
		),

		Entry("prose before AND after JSON",
			"Reasoning here.\n\n{\"verdict\":\"pass\",\"reason\":\"ok\"}\n\nFurther explanation.",
			"pass", "ok", true),

		Entry("multiple JSON-like fragments — picks the last balanced block",
			"Ignored: {\"foo\":\"bar\"}\n\nFinal: {\"verdict\":\"fail\",\"reason\":\"z\"}",
			"fail", "z", true),

		Entry("nested objects in the verdict JSON are preserved",
			"```json\n{\"verdict\":\"fail\",\"reason\":\"nested\",\"detail\":{\"a\":1}}\n```",
			"fail", "nested", true),

		Entry("empty string fails",
			"", "", "", false),

		Entry("prose only without any JSON fails",
			"This is just prose with no braces.", "", "", false),

		Entry("malformed JSON with unbalanced braces fails",
			"oops {{{", "", "", false),
	)
})

var _ = Describe("reviewStep", func() {
	var (
		ctx          context.Context
		runner       *mocks.ClaudeRunnerMock
		step         agentlib.Step
		instructions claudelib.Instructions
	)

	BeforeEach(func() {
		ctx = context.Background()
		runner = &mocks.ClaudeRunnerMock{}
		instructions = claudelib.Instructions{}
		step = pkg.NewReviewStep(runner, instructions, nil, "", "")
	})

	Describe("Name", func() {
		It("returns the step name", func() {
			Expect(step.Name()).To(Equal("pr-ai-review"))
		})
	})

	Describe("ShouldRun", func() {
		DescribeTable("decides based on existing ## Verdict section",
			func(content string, expected bool) {
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.ShouldRun(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("no verdict section", "# PR Review\n\nsome text", true),
			Entry("verdict section present", "# PR Review\n\n## Verdict\n\npass", false),
			Entry("empty content", "", true),
		)
	})

	Describe("Run", func() {
		var md *agentlib.Markdown

		BeforeEach(func() {
			var err error
			md, err = agentlib.ParseMarkdown(ctx, "# Task\n\nsome content")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when Claude runner returns an error", func() {
			BeforeEach(func() {
				runner.RunReturns(nil, fmt.Errorf("claude CLI failed"))
			})

			It("returns AgentStatusFailed result without propagating the error", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
			})
		})

		Context("when Claude runner returns unparseable output", func() {
			BeforeEach(func() {
				runner.RunReturns(&claudelib.ClaudeResult{Result: "this is not json at all"}, nil)
			})

			It("returns AgentStatusDone with NextPhase human_review", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("human_review"))
			})
		})

		Context("when Claude runner returns verdict: pass", func() {
			BeforeEach(func() {
				runner.RunReturns(
					&claudelib.ClaudeResult{Result: `{"verdict":"pass","reason":"looks good"}`},
					nil,
				)
			})

			It("returns AgentStatusDone with NextPhase done", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))
				Expect(result.Message).To(Equal("looks good"))
			})
		})

		Context("when Claude runner returns verdict: fail", func() {
			BeforeEach(func() {
				runner.RunReturns(
					&claudelib.ClaudeResult{Result: `{"verdict":"fail","reason":"issues found"}`},
					nil,
				)
			})

			It("returns AgentStatusDone with NextPhase human_review", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("human_review"))
			})
		})
	})

	Describe("verification behavior", func() {
		const prURL = "https://github.com/bborbe/maintainer/pull/2"
		const passVerdict = `{"verdict":"pass","reason":"all checks pass"}`

		var verifier *mocks.ReviewVerifier

		BeforeEach(func() {
			verifier = &mocks.ReviewVerifier{}
			step = pkg.NewReviewStep(runner, instructions, verifier, "test-token", "test-bot")
			runner.RunReturns(&claudelib.ClaudeResult{Result: passVerdict}, nil)
		})

		Context("skip verification when ## Review is absent", func() {
			It("does not call verifier; meta-verdict routes normally", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"---\nref: abc123\n---\n\nReview the PR at "+prURL+"\n\nsome content",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(verifier.VerifyReviewCallCount()).To(Equal(0))
			})
		})

		Context("skip verification when Diagnostics shows class: permanent", func() {
			It("does not call verifier", func() {
				diagBody := "```yaml\nclass: permanent\n```\n"
				content := "---\nref: abc123\n---\n\nReview the PR at " + prURL + "\n\n" +
					"## Review\n\nsome content\n\n" +
					"## Diagnostics\n\n" + diagBody
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				_, err = step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(verifier.VerifyReviewCallCount()).To(Equal(0))
			})
		})

		Context("skip verification when Diagnostics shows class: unknown", func() {
			It("does not call verifier", func() {
				diagBody := "```yaml\nclass: unknown\n```\n"
				content := "---\nref: abc123\n---\n\nReview the PR at " + prURL + "\n\n" +
					"## Review\n\nsome content\n\n" +
					"## Diagnostics\n\n" + diagBody
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				_, err = step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(verifier.VerifyReviewCallCount()).To(Equal(0))
			})
		})

		Context("verification runs and succeeds", func() {
			BeforeEach(func() {
				verifier.VerifyReviewReturns(pkg.VerifyResult{
					Found:      true,
					Outcome:    "success",
					FoundState: "APPROVED",
				})
			})

			It("calls verifier once and routes based on meta-verdict", func() {
				diagBody := "```yaml\nclass: transient\n```\n"
				content := "---\nref: abc123\n---\n\nReview the PR at " + prURL + "\n\n" +
					"## Review\n\nsome content\n\n" +
					"## Diagnostics\n\n" + diagBody
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(verifier.VerifyReviewCallCount()).To(Equal(1))
				// meta-verdict is "pass" → done
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))
			})
		})

		Context("verification runs and fails", func() {
			BeforeEach(func() {
				verifier.VerifyReviewReturns(pkg.VerifyResult{
					Found:        false,
					Outcome:      "failed",
					Class:        pkg.ErrorClassTransient,
					EscalateHint: false,
					HTTPStatus:   0,
					ErrorMessage: "review not found",
				})
			})

			It("exits with AgentStatusFailed and writes ai_review verify diagnostic", func() {
				content := "---\nref: abc123\n---\n\nReview the PR at " + prURL + "\n\n" +
					"## Review\n\nsome content\n"
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.Message).To(ContainSubstring("post verification failed"))
				// Diagnostics section should contain the ai_review verify line
				diagSection, exists := md.FindSection("## Diagnostics")
				Expect(exists).To(BeTrue())
				Expect(diagSection).NotTo(BeNil())
				Expect(diagSection.Body).To(ContainSubstring("ai_review verify:"))
				Expect(diagSection.Body).To(ContainSubstring("review not found"))
			})
		})

		Context("nil verifier skips verification without panic", func() {
			It("routes normally", func() {
				step = pkg.NewReviewStep(runner, instructions, nil, "", "")
				content := "---\nref: abc123\n---\n\nReview the PR at " + prURL + "\n\n" +
					"## Review\n\nsome content\n"
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
			})
		})
	})
})

var _ = Describe("shouldVerifyPost", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	const prURL = "https://github.com/bborbe/maintainer/pull/2"

	taskWithReview := func(diagBody string) *agentlib.Markdown {
		content := "---\nref: abc123\n---\n\nReview the PR at " + prURL + "\n\n## Review\n\nsome content\n"
		if diagBody != "" {
			content += "\n## Diagnostics\n\n" + diagBody
		}
		md, err := agentlib.ParseMarkdown(context.Background(), content)
		Expect(err).NotTo(HaveOccurred())
		return md
	}

	Describe("handles ## Diagnostics absent", func() {
		It("returns true — verification should run", func() {
			md := taskWithReview("")
			result, err := pkg.ShouldVerifyPostForTest(ctx, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Describe("selects the MOST RECENT diagnostic block when multiple exist", func() {
		It(
			"returns true when last block has class: transient despite older class: permanent",
			func() {
				diagBody := "```yaml\ntrigger_count: 0\nclass: permanent\n```\n\n" +
					"```yaml\ntrigger_count: 1\nclass: transient\n```\n"
				md := taskWithReview(diagBody)
				result, err := pkg.ShouldVerifyPostForTest(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			},
		)
	})

	Describe("diagnostic format round-trip with prompt 2's exact output", func() {
		It(
			"parses class: permanent from buildDiagnosticBlock output and skips verification",
			func() {
				// Exact format produced by buildDiagnosticBlock for a failure:
				// fmt.Sprintf("```yaml\njob_run: %s\ntrigger_count: %d\n...class: %s\n...```\n", ...)
				diagBody := "```yaml\n" +
					"job_run: 2026-01-01T00:00:00Z\n" +
					"trigger_count: 1\n" +
					"outcome: failed\n" +
					"failure_step: POST /pulls/2/reviews\n" +
					"class: permanent\n" +
					"escalate_hint: true\n" +
					"attempt: 1\n" +
					"http_status: 403\n" +
					"error_message: \"forbidden\"\n" +
					"response_body: \"{}\"\n" +
					"elapsed_ms: 100\n" +
					"```\n"
				md := taskWithReview(diagBody)
				result, err := pkg.ShouldVerifyPostForTest(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			},
		)
	})

	Describe("returns false when ## Review is absent", func() {
		It("skips verification without error", func() {
			md, err := agentlib.ParseMarkdown(ctx, "# Task\n\nsome content")
			Expect(err).NotTo(HaveOccurred())
			result, err := pkg.ShouldVerifyPostForTest(ctx, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Describe("returns false when last block has class: unknown", func() {
		It("skips verification", func() {
			diagBody := "```yaml\nclass: unknown\n```\n"
			md := taskWithReview(diagBody)
			result, err := pkg.ShouldVerifyPostForTest(ctx, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Describe("returns true when Diagnostics has only success one-liners", func() {
		It("runs verification (no yaml block means no skip condition)", func() {
			diagBody := "job_run: 2026-01-01T00:00:00Z outcome: success review_id: 12345\n"
			md := taskWithReview(diagBody)
			result, err := pkg.ShouldVerifyPostForTest(ctx, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})
})
