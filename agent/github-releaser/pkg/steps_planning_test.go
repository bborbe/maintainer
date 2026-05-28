// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"errors"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentmocks "github.com/bborbe/maintainer/agent/github-releaser/mocks"
	pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
	githubchangelogmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/githubchangelog/mocks"
)

var _ = Describe("steps_planning", func() {
	Describe("PlanningStep", func() {
		Context("happy path", func() {
			It("ready path: emits ## Plan with outcome=ready and NextPhase=execution", func() {
				fakeFetcher := &githubchangelogmocks.Fetcher{}
				fakeFetcher.FetchReturns(
					[]byte("## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.7.7\n\n- old\n"),
					nil,
				)

				fakeRunner := &agentmocks.ClaudeRunnerMock{}
				fakeRunner.RunReturns(&claudelib.ClaudeResult{
					Result: `{"bump":"minor","reasoning":"feat: stub"}`,
				}, nil)

				step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

				taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-bborbe-maintainer-master-001\n---\n\n# release task\n"

				md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("execution"))

				plan, err := agentlib.ExtractSection[pkg.PlanOutput](
					context.Background(),
					md,
					"## Plan",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(plan.Outcome).To(Equal("ready"))
				Expect(plan.Bump).To(Equal("minor"))
				Expect(plan.CurrentVersion).To(Equal("v1.7.7"))
				Expect(plan.NextVersion).To(Equal("1.8.0"))
				Expect(plan.NextVersionHeader).To(Equal("## v1.8.0"))
				Expect(plan.HeaderPrefixStyle).To(Equal("v"))
				Expect(plan.Bullets).To(ContainElements("feat: add foo", "fix: bar"))
			})
		})

		Context("P1 escalation", func() {
			It(
				"P1 escalation: ## Unreleased not first → outcome=needs_input + assignee cleared",
				func() {
					badChangelog := []byte(
						"# Changelog\n\nIntro text.\n\n## v1.2.6\n\n- old release\n\n## Unreleased\n\n- new bullet\n",
					)
					fakeFetcher := &githubchangelogmocks.Fetcher{}
					fakeFetcher.FetchReturns(badChangelog, nil)
					fakeRunner := &agentmocks.ClaudeRunnerMock{} // not called on escalation

					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.2.6\ntask_identifier: gh-release-bborbe-maintainer-master-001\n---\n\n# release task\n"

					md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
					Expect(err).NotTo(HaveOccurred())

					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
					// NextPhase empty — caller stays in planning per spec 047 Desired Behavior 6.
					Expect(result.NextPhase).To(BeEmpty())

					// Fetcher called, Claude NOT called (escalation short-circuits before claude).
					Expect(fakeRunner.RunCallCount()).To(Equal(0))

					plan, err := agentlib.ExtractSection[pkg.PlanOutput](
						context.Background(),
						md,
						"## Plan",
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(plan.Outcome).To(Equal("needs_input"))
					Expect(plan.PreconditionFailed).To(Equal("P1_unreleased_not_first"))
					Expect(plan.Reason).To(ContainSubstring("not the first ## section"))

					// Frontmatter mutations:
					gotAssignee, _ := md.Frontmatter.String("assignee")
					Expect(gotAssignee).To(Equal(""))
					gotPrevAssignee, _ := md.Frontmatter.String("previous_assignee")
					Expect(gotPrevAssignee).To(Equal("github-releaser-agent"))
					gotStatus, _ := md.Frontmatter.String("status")
					Expect(gotStatus).To(Equal("in_progress"))
					gotPhase, _ := md.Frontmatter.String("phase")
					Expect(gotPhase).To(Equal("planning"))
				},
			)
		})

		Context("missing frontmatter", func() {
			It(
				"missing clone_url → outcome=needs_input + precondition_failed=missing_frontmatter_clone_url",
				func() {
					fakeFetcher := &githubchangelogmocks.Fetcher{}
					fakeRunner := &agentmocks.ClaudeRunnerMock{}
					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-001\n---\n"
					// clone_url intentionally missing

					md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
					Expect(fakeFetcher.FetchCallCount()).To(Equal(0))

					plan, _ := agentlib.ExtractSection[pkg.PlanOutput](
						context.Background(),
						md,
						"## Plan",
					)
					Expect(plan.Outcome).To(Equal("needs_input"))
					Expect(plan.PreconditionFailed).To(Equal("missing_frontmatter_clone_url"))
				},
			)
		})

		Context("fetch error", func() {
			It("fetcher transport error → Status=Failed", func() {
				fakeFetcher := &githubchangelogmocks.Fetcher{}
				fakeFetcher.FetchReturns(nil, errors.New("dial tcp: connection refused"))
				fakeRunner := &agentmocks.ClaudeRunnerMock{}

				step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

				taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-001\n---\n"

				md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.Message).To(ContainSubstring("fetch CHANGELOG.md"))
			})
		})

		Context("claude parse error", func() {
			It("claude returns malformed JSON → Status=Failed", func() {
				fakeFetcher := &githubchangelogmocks.Fetcher{}
				fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: x\n"), nil)
				fakeRunner := &agentmocks.ClaudeRunnerMock{}
				fakeRunner.RunReturns(&claudelib.ClaudeResult{Result: "not-json-at-all"}, nil)

				step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

				taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-001\n---\n"

				md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(result.Message).To(ContainSubstring("parse bump verdict"))
			})
		})

		Context("bad current_version", func() {
			It(
				"malformed current_version → outcome=needs_input + precondition_failed=bad_current_version",
				func() {
					fakeFetcher := &githubchangelogmocks.Fetcher{}
					fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: x\n"), nil)
					fakeRunner := &agentmocks.ClaudeRunnerMock{}
					fakeRunner.RunReturns(
						&claudelib.ClaudeResult{Result: `{"bump":"minor","reasoning":"x"}`},
						nil,
					)

					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: garbage\ntask_identifier: gh-release-001\n---\n"

					md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
					plan, _ := agentlib.ExtractSection[pkg.PlanOutput](
						context.Background(),
						md,
						"## Plan",
					)
					Expect(plan.Outcome).To(Equal("needs_input"))
					Expect(plan.PreconditionFailed).To(Equal("bad_current_version"))
				},
			)
		})

		Context("P2 escalation", func() {
			It(
				"empty Unreleased bullets → outcome=needs_input + precondition_failed=P2_unreleased_empty",
				func() {
					fakeFetcher := &githubchangelogmocks.Fetcher{}
					fakeFetcher.FetchReturns([]byte("## Unreleased\n\n## v1.0.0\n\n- old\n"), nil)
					fakeRunner := &agentmocks.ClaudeRunnerMock{}

					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.0.0\ntask_identifier: gh-release-001\n---\n"

					md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
					plan, _ := agentlib.ExtractSection[pkg.PlanOutput](
						context.Background(),
						md,
						"## Plan",
					)
					Expect(plan.Outcome).To(Equal("needs_input"))
					Expect(plan.PreconditionFailed).To(Equal("P2_unreleased_empty"))
				},
			)
		})

		Context("idempotency", func() {
			It("idempotent: re-running with existing ## Plan replaces it", func() {
				fakeFetcher := &githubchangelogmocks.Fetcher{}
				fakeFetcher.FetchReturns(
					[]byte("## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.7.7\n\n- old\n"),
					nil,
				)
				fakeRunner := &agentmocks.ClaudeRunnerMock{}
				fakeRunner.RunReturns(&claudelib.ClaudeResult{
					Result: `{"bump":"minor","reasoning":"feat: stub"}`,
				}, nil)

				step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

				taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-bborbe-maintainer-master-001\n---\n\n# release task\n\n## Plan\n\n```json\n{\"outcome\":\"stale\"}\n```\n"

				md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

				// After Run, there should be exactly one ## Plan section with fresh outcome
				var planCount int
				for _, sec := range md.Sections {
					if sec.Heading == "## Plan" {
						planCount++
					}
				}
				Expect(planCount).To(Equal(1))

				// And the plan should be fresh (not "stale")
				plan, err := agentlib.ExtractSection[pkg.PlanOutput](
					context.Background(),
					md,
					"## Plan",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(plan.Outcome).To(Equal("ready"))
			})
		})
	})
})
