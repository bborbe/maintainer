// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	agentlib "github.com/bborbe/agent/lib"
	claudelib "github.com/bborbe/agent/lib/claude"
	delivery "github.com/bborbe/agent/lib/delivery"
	domain "github.com/bborbe/vault-cli/pkg/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/mocks"
	pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
	"github.com/bborbe/maintainer/agent/github-releaser/pkg/factory"
)

var _ = Describe("steps_planning", func() {
	Describe("PlanningStep", func() {
		Context("happy path", func() {
			It("ready path: emits ## Plan with outcome=ready and NextPhase=execution", func() {
				fakeFetcher := &mocks.Fetcher{}
				fakeFetcher.FetchReturns(
					[]byte("## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.7.7\n\n- old\n"),
					nil,
				)

				fakeRunner := &mocks.ClaudeRunnerMock{}
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
				Expect(fakeFetcher.FetchCallCount()).To(Equal(1))

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
					fakeFetcher := &mocks.Fetcher{}
					fakeFetcher.FetchReturns(badChangelog, nil)
					fakeRunner := &mocks.ClaudeRunnerMock{} // not called on escalation

					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.2.6\ntask_identifier: gh-release-bborbe-maintainer-master-001\n---\n\n# release task\n"

					md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
					Expect(err).NotTo(HaveOccurred())

					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
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
					fakeFetcher := &mocks.Fetcher{}
					fakeRunner := &mocks.ClaudeRunnerMock{}
					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nref: master\ncurrent_version: v1.7.7\ntask_identifier: gh-release-001\n---\n"
					// clone_url intentionally missing

					md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
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
				fakeFetcher := &mocks.Fetcher{}
				fakeFetcher.FetchReturns(nil, errors.New("dial tcp: connection refused"))
				fakeRunner := &mocks.ClaudeRunnerMock{}

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
				fakeFetcher := &mocks.Fetcher{}
				fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: x\n"), nil)
				fakeRunner := &mocks.ClaudeRunnerMock{}
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
					fakeFetcher := &mocks.Fetcher{}
					fakeFetcher.FetchReturns([]byte("## Unreleased\n\n- feat: x\n"), nil)
					fakeRunner := &mocks.ClaudeRunnerMock{}
					fakeRunner.RunReturns(
						&claudelib.ClaudeResult{Result: `{"bump":"minor","reasoning":"x"}`},
						nil,
					)

					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: garbage\ntask_identifier: gh-release-001\n---\n"

					md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
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
					fakeFetcher := &mocks.Fetcher{}
					fakeFetcher.FetchReturns([]byte("## Unreleased\n\n## v1.0.0\n\n- old\n"), nil)
					fakeRunner := &mocks.ClaudeRunnerMock{}

					step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)

					taskMD := "---\nstatus: in_progress\nphase: planning\nassignee: github-releaser-agent\ntask_type: github-release\nrepo: bborbe/maintainer\nclone_url: https://github.com/bborbe/maintainer.git\nref: master\ncurrent_version: v1.0.0\ntask_identifier: gh-release-001\n---\n"

					md, _ := agentlib.ParseMarkdown(context.Background(), taskMD)
					result, err := step.Run(context.Background(), md)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))
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
				fakeFetcher := &mocks.Fetcher{}
				fakeFetcher.FetchReturns(
					[]byte("## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.7.7\n\n- old\n"),
					nil,
				)
				fakeRunner := &mocks.ClaudeRunnerMock{}
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

var _ = Describe("parseOwnerRepo", func() {
	DescribeTable("splits owner/name",
		func(input, wantOwner, wantName string, wantOK bool) {
			owner, name, ok := pkg.ParseOwnerRepoForTest(input)
			Expect(ok).To(Equal(wantOK))
			Expect(owner).To(Equal(wantOwner))
			Expect(name).To(Equal(wantName))
		},
		Entry("empty string", "", "", "", false),
		Entry("no slash", "badrepo", "", "", false),
		Entry("empty owner", "/name", "", "", false),
		Entry("empty name", "owner/", "", "", false),
		Entry("happy path", "owner/name", "owner", "name", true),
	)
})

var _ = Describe("classifyValidationFailure", func() {
	DescribeTable("maps validator reason to precondition",
		func(reason, want string) {
			Expect(pkg.ClassifyValidationFailureForTest(reason)).To(Equal(want))
		},
		Entry("not-first branch",
			"Unreleased is not the first ## section; found 'x' at line 1.",
			"P1_unreleased_not_first"),
		Entry("no bullet entries branch",
			"Unreleased section has no bullet entries.",
			"P2_unreleased_empty"),
		Entry("not found branch",
			"Unreleased section not found.",
			"P2_unreleased_empty"),
		Entry("default branch",
			"some unexpected reason",
			"P2_unreleased_empty"),
	)
})

var _ = Describe("steps_planning integration (spec 048 regression guard)", func() {
	// This test wires the full agent via factory.CreateAgent and runs it
	// against the real FileResultDeliverer to exercise the framework-side
	// status→frontmatter switch. The bug fixed in spec 048 lived in that
	// switch: AgentStatusDone on escalation auto-advances to
	// phase: done, status: completed; AgentStatusNeedsInput preserves
	// phase and writes status: in_progress.
	//
	// The step-level Fetcher is mocked so the test runs OFFLINE — no real
	// GitHub network calls. The Claude runner is also mocked but is never
	// invoked on a P1 escalation path (escalation short-circuits before
	// classification).
	//
	// Fixture: a CHANGELOG where ## Unreleased is NOT the first ## heading
	// — triggers P1 escalation. Per spec 047 § Desired Behavior 4, this
	// path returns the NeedsInput verdict in ## Plan + clears assignee +
	// sets previous_assignee, while leaving status/phase alone.
	Context("P1 escalation via FileResultDeliverer", func() {
		var tmpDir string
		var taskFile string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "spec-048-*")
			Expect(err).NotTo(HaveOccurred())
			taskFile = filepath.Join(tmpDir, "task.md")
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
		})

		It(
			"framework deliverer leaves status: in_progress and phase: planning unchanged on escalation",
			func() {
				// Fixture: ## Unreleased is the SECOND ## heading → P1 fail.
				initialTask := `---
status: in_progress
phase: planning
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/maintainer
clone_url: https://github.com/bborbe/maintainer.git
ref: master
current_version: v1.2.6
task_identifier: gh-release-bborbe-maintainer-master-spec048
---

# release task
`
				Expect(os.WriteFile(taskFile, []byte(initialTask), 0o600)).To(Succeed())

				// Inject the mock Fetcher via package-level seam: we cannot use
				// factory.CreateAgent directly because it wires the real
				// HTTPFetcher. Build the planning step manually with the mock
				// fetcher, wrap it in a one-phase Agent identical in shape to
				// what factory.CreateAgent produces. This is intentional — the
				// factory's job is just composition; the integration we care
				// about is the agent.Run + FileResultDeliverer chain, which
				// this exercises identically.
				badChangelog := []byte(
					"# Changelog\n\nIntro.\n\n## v1.2.6\n\n- old release\n\n## Unreleased\n\n- new bullet\n",
				)
				fakeFetcher := &mocks.Fetcher{}
				fakeFetcher.FetchReturns(badChangelog, nil)
				fakeRunner := &mocks.ClaudeRunnerMock{} // never called on P1

				step := pkg.NewPlanningStep(fakeRunner, fakeFetcher)
				agent := agentlib.NewAgent(agentlib.NewPhase(domain.TaskPhasePlanning, step))

				// Use the real FileResultDeliverer + passthrough generator —
				// same wiring as cmd/run-task. This is the deliverer whose
				// Status switch contains the bug being fixed.
				deliverer := delivery.NewFileResultDeliverer(
					delivery.NewPassthroughContentGenerator(),
					taskFile,
				)

				result, err := agent.Run(
					context.Background(),
					domain.TaskPhasePlanning,
					initialTask,
					deliverer,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Status).To(Equal(agentlib.AgentStatusNeedsInput))

				// Read back the file the deliverer wrote.
				mutated, err := os.ReadFile(taskFile)
				Expect(err).NotTo(HaveOccurred())
				mutatedStr := string(mutated)

				// Regression assertions — the bug-fix invariant lives here.
				// Each of these failed against the OLD code (AgentStatusDone
				// on escalation) because the framework switch wrote
				// phase: done + status: completed.
				Expect(mutatedStr).To(ContainSubstring("status: in_progress"))
				Expect(mutatedStr).To(ContainSubstring("phase: planning"))

				// Defense in depth: explicitly negate the bug state.
				Expect(mutatedStr).NotTo(ContainSubstring("status: completed"))
				Expect(mutatedStr).NotTo(ContainSubstring("phase: done"))

				// Sanity: assignee cleared, previous_assignee set
				// (these were already correct in the buggy version — included
				// here so a future refactor doesn't accidentally regress the
				// escalation rule's other half).
				// Note: YAML serializes empty string as "assignee: " (no quotes).
				// We use a regexp to match the line exactly (start of line, assignee:,
				// optional space, then newline — not "assignee: github-releaser-agent").
				assigneeLineRegex := `(?m)^assignee:\s*$\n`
				Expect(mutatedStr).To(MatchRegexp(assigneeLineRegex))
				Expect(mutatedStr).To(ContainSubstring("previous_assignee: github-releaser-agent"))

				// Claude must NOT have been invoked — P1 escalation
				// short-circuits before classification.
				Expect(fakeRunner.RunCallCount()).To(Equal(0))

				// Avoid "imported and not used" if claudelib is otherwise
				// unreferenced by this block.
				var _ claudelib.ClaudeRunner = fakeRunner
			},
		)
	})
})

// Compile-time assertion that factory.CreateAgent is the symbol we mean
// to keep coupled to this integration test, even though the test builds
// its own Agent to inject the mock fetcher. If this signature changes,
// update the integration test to match.
var _ = func() *agentlib.Agent {
	return factory.CreateAgent(
		claudelib.ClaudeConfigDir("/tmp"),
		claudelib.AgentDir("/tmp"),
		claudelib.ClaudeModel("sonnet"),
		"",
		map[string]string{},
	)
}
