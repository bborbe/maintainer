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
		step = pkg.NewReviewStep(runner, instructions)
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
})
