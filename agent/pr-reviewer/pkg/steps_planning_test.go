// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"encoding/json"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/pr-reviewer/mocks"
	pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

var _ = Describe("planningStep", func() {
	var (
		ctx      context.Context
		runner   *mocks.ClaudeRunnerMock
		prPoster *mocks.PrPoster
		step     agentlib.Step
		botLogin string
	)

	BeforeEach(func() {
		ctx = context.Background()
		runner = &mocks.ClaudeRunnerMock{}
		prPoster = &mocks.PrPoster{}
		botLogin = "ben-s-pull-request-reviewer-dev[bot]"
		step = pkg.NewPlanningStep(
			runner,
			claudelib.Instructions{},
			prPoster,
			botLogin,
		)
	})

	Describe("Name", func() {
		It("returns pr-plan", func() {
			Expect(step.Name()).To(Equal("pr-plan"))
		})
	})

	Describe("ShouldRun", func() {
		DescribeTable("decides based on existing ## Plan section",
			func(content string, expected bool) {
				md, err := agentlib.ParseMarkdown(ctx, content)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.ShouldRun(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("no plan section", "# PR Review\n\nsome text", true),
			Entry("plan section present", "# PR Review\n\n## Plan\n\n{}", false),
			Entry("empty content", "", true),
		)
	})

	Describe("Run — empty concerns path (LGTM)", func() {
		var md *agentlib.Markdown

		BeforeEach(func() {
			var err error
			md, err = agentlib.ParseMarkdown(ctx, `---
ref: abc123
task_identifier: 00000000-0000-0000-0000-000000000001
---
# PR Review

https://github.com/bborbe/maintainer/pull/14
`)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when ## Plan has concerns: [] and POST succeeds", func() {
			BeforeEach(func() {
				planBody, _ := json.Marshal(map[string]interface{}{
					"pr_url":        "https://github.com/bborbe/maintainer/pull/14",
					"pr_title":      "test PR",
					"base_branch":   "main",
					"head_branch":   "feat/test",
					"files_changed": []string{"README.md"},
					"scope":         "docs",
					"focus_areas":   []string{"docs"},
					"concerns":      []interface{}{},
				})
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "```json\n" + string(planBody) + "\n```",
				}, nil)
				prPoster.PostLGTMReturns(pkg.PostResult{
					Outcome:     "success",
					ReviewID:    12345,
					PostedEvent: "COMMENT",
				})
			})

			It("calls PrPoster.PostLGTM with correct arguments", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(prPoster.PostLGTMCallCount()).To(Equal(1))
				_, prArg, headSHAArg, workDirArg, botLoginArg := prPoster.PostLGTMArgsForCall(0)
				Expect(prArg.Owner).To(Equal("bborbe"))
				Expect(prArg.Repo).To(Equal("maintainer"))
				Expect(prArg.Number).To(Equal(14))
				Expect(headSHAArg).To(Equal("abc123"))
				Expect(workDirArg).To(Equal(""))
				Expect(botLoginArg).To(Equal(botLogin))
			})

			It("returns status done with NextPhase done", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))
			})

			It("writes ## Plan section with the LLM output", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				planSection, exists := md.FindSection("## Plan")
				Expect(exists).To(BeTrue())
				Expect(planSection.Body).To(ContainSubstring("concerns"))
			})

			It("writes ## Verdict section naming review id and COMMENT", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				verdictSection, exists := md.FindSection("## Verdict")
				Expect(exists).To(BeTrue())
				Expect(verdictSection.Body).To(ContainSubstring("review_id: 12345"))
				Expect(verdictSection.Body).To(ContainSubstring("event: COMMENT"))
			})

			It("appends a success diagnostics one-liner", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				diagSection, exists := md.FindSection("## Diagnostics")
				Expect(exists).To(BeTrue())
				Expect(diagSection.Body).To(ContainSubstring("outcome: success"))
				Expect(diagSection.Body).To(ContainSubstring("review_id: 12345"))
			})
		})

		Context("when ## Plan has concerns: [] and POST returns failure", func() {
			BeforeEach(func() {
				planBody, _ := json.Marshal(map[string]interface{}{
					"pr_url":        "https://github.com/bborbe/maintainer/pull/14",
					"pr_title":      "test PR",
					"base_branch":   "main",
					"head_branch":   "feat/test",
					"files_changed": []string{"README.md"},
					"scope":         "docs",
					"focus_areas":   []string{"docs"},
					"concerns":      []interface{}{},
				})
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "```json\n" + string(planBody) + "\n```",
				}, nil)
				prPoster.PostLGTMReturns(pkg.PostResult{
					Outcome:      "failed",
					FailureStep:  "POST /pulls/N/reviews",
					Class:        pkg.ErrorClassTransient,
					ErrorMessage: "network timeout",
					HTTPStatus:   500,
				})
			})

			It("returns status done with NextPhase human_review", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("human_review"))
				Expect(result.Message).To(ContainSubstring("LGTM POST failed"))
			})

			It("appends a failure diagnostic block", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				diagSection, exists := md.FindSection("## Diagnostics")
				Expect(exists).To(BeTrue())
				Expect(diagSection.Body).To(ContainSubstring("outcome: failed"))
				Expect(diagSection.Body).To(ContainSubstring("network timeout"))
			})

			It("does NOT write ## Verdict section", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				_, exists := md.FindSection("## Verdict")
				Expect(exists).To(BeFalse())
			})
		})

		Context("when prPoster is nil (cmd/run-task mode)", func() {
			BeforeEach(func() {
				step = pkg.NewPlanningStep(runner, claudelib.Instructions{}, nil, botLogin)
				planBody, _ := json.Marshal(map[string]interface{}{
					"pr_url":        "https://github.com/bborbe/maintainer/pull/14",
					"pr_title":      "test PR",
					"base_branch":   "main",
					"head_branch":   "feat/test",
					"files_changed": []string{"README.md"},
					"scope":         "docs",
					"focus_areas":   []string{"docs"},
					"concerns":      []interface{}{},
				})
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "```json\n" + string(planBody) + "\n```",
				}, nil)
			})

			It("returns done without calling PostLGTM", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("done"))
			})
		})
	})

	Describe("Run — non-empty concerns path (execution)", func() {
		var md *agentlib.Markdown

		BeforeEach(func() {
			var err error
			md, err = agentlib.ParseMarkdown(ctx, `---
ref: abc123
task_identifier: 00000000-0000-0000-0000-000000000001
---
# PR Review

https://github.com/bborbe/maintainer/pull/14
`)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when ## Plan has non-empty concerns", func() {
			BeforeEach(func() {
				planBody, _ := json.Marshal(map[string]interface{}{
					"pr_url":        "https://github.com/bborbe/maintainer/pull/14",
					"pr_title":      "test PR",
					"base_branch":   "main",
					"head_branch":   "feat/test",
					"files_changed": []string{"pkg/auth/handler.go"},
					"scope":         "feature",
					"focus_areas":   []string{"security"},
					"concerns": []map[string]string{
						{
							"area": "security",
							"file": "pkg/auth/handler.go",
							"note": "missing rate limit",
						},
					},
				})
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "```json\n" + string(planBody) + "\n```",
				}, nil)
			})

			It("returns status done with NextPhase in_progress", func() {
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("in_progress"))
			})

			It("does NOT call PostLGTM", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(prPoster.PostLGTMCallCount()).To(Equal(0))
			})

			It("does NOT write ## Verdict section", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				_, exists := md.FindSection("## Verdict")
				Expect(exists).To(BeFalse())
			})

			It("does NOT append diagnostics", func() {
				_, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				_, exists := md.FindSection("## Diagnostics")
				Expect(exists).To(BeFalse())
			})
		})
	})

	Describe("Run — error cases", func() {
		Context("when ## Plan JSON is malformed", func() {
			BeforeEach(func() {
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "not valid json at all",
				}, nil)
			})

			It("routes to human_review", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"# PR Review\n\nhttps://github.com/bborbe/maintainer/pull/14\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("human_review"))
				Expect(result.Message).To(ContainSubstring("parse ## Plan JSON"))
			})
		})

		Context("when Claude runner returns an error", func() {
			BeforeEach(func() {
				runner.RunReturns(nil, context.DeadlineExceeded)
			})

			It("returns AgentStatusFailed", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"# PR Review\n\nhttps://github.com/bborbe/maintainer/pull/14\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
			})
		})

		Context("when PR URL is absent from task", func() {
			BeforeEach(func() {
				planBody, _ := json.Marshal(map[string]interface{}{
					"pr_url":        "https://github.com/bborbe/maintainer/pull/14",
					"pr_title":      "test PR",
					"base_branch":   "main",
					"head_branch":   "feat/test",
					"files_changed": []string{"README.md"},
					"scope":         "docs",
					"focus_areas":   []string{"docs"},
					"concerns":      []interface{}{},
				})
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "```json\n" + string(planBody) + "\n```",
				}, nil)
			})

			It("returns human_review when PR URL missing", func() {
				md, err := agentlib.ParseMarkdown(ctx, "# PR Review\n")
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.NextPhase).To(Equal("human_review"))
				Expect(result.Message).To(ContainSubstring("no GitHub PR URL"))
			})
		})

		Context("when non-GitHub platform", func() {
			BeforeEach(func() {
				planBody, _ := json.Marshal(map[string]interface{}{
					"pr_url":        "https://bitbucket.org/bborbe/maintainer/pull/14",
					"pr_title":      "test PR",
					"base_branch":   "main",
					"head_branch":   "feat/test",
					"files_changed": []string{"README.md"},
					"scope":         "docs",
					"focus_areas":   []string{"docs"},
					"concerns":      []interface{}{},
				})
				runner.RunReturns(&claudelib.ClaudeResult{
					Result: "```json\n" + string(planBody) + "\n```",
				}, nil)
			})

			It("skips posting and returns done", func() {
				md, err := agentlib.ParseMarkdown(
					ctx,
					"# PR Review\n\nhttps://bitbucket.org/bborbe/maintainer/pull/14\n",
				)
				Expect(err).NotTo(HaveOccurred())
				result, err := step.Run(ctx, md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.NextPhase).To(Equal("done"))
				Expect(prPoster.PostLGTMCallCount()).To(Equal(0))
			})
		})
	})

	Describe("parsePlanningConcerns", func() {
		DescribeTable(
			"extracts concerns array from various JSON wrapping",
			func(body, want string) {
				concerns, err := pkg.ParsePlanningConcernsForTest(body)
				if want == "error" {
					Expect(err).To(HaveOccurred())
					return
				}
				Expect(err).NotTo(HaveOccurred())
				if want == "empty" {
					Expect(concerns).To(BeEmpty())
				} else {
					Expect(concerns).NotTo(BeEmpty())
				}
			},
			Entry("bare JSON array", `{"concerns":[]}`, "empty"),
			Entry("json fence", "```json\n{\"concerns\":[]}\n```", "empty"),
			Entry(
				"non-empty concerns",
				"```json\n{\"concerns\":[{\"area\":\"security\"}]}\n```",
				"non-empty",
			),
			Entry("malformed JSON", "not json at all", "error"),
		)
	})
})
