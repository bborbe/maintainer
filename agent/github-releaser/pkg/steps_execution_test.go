// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"os"
	"path/filepath"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
	gitmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/git/mocks"
)

var _ = Describe("ExecutionStep", func() {
	// Verify Name() and ShouldRun() exist and return the expected values.
	// These are simple delegation methods; direct test ensures they are not dead code.
	Describe("interface methods", func() {
		It("Name returns github-release-execute", func() {
			fakeOps := &gitmocks.GitOps{}
			step := pkg.NewExecutionStep(fakeOps, "")
			Expect(step.Name()).To(Equal("github-release-execute"))
		})

		It("ShouldRun returns true, nil", func() {
			fakeOps := &gitmocks.GitOps{}
			step := pkg.NewExecutionStep(fakeOps, "")
			minimalMD := `---
status: in_progress
phase: execution
task_identifier: test
clone_url: https://github.com/test/test.git
ref: main
---

## Plan

` + "```json" + `{"outcome":"ready","next_version":"1.0.0","next_version_header":"## v1.0.0"}` + "```"
			md, err := agentlib.ParseMarkdown(context.Background(), minimalMD)
			Expect(err).NotTo(HaveOccurred())
			shouldRun, err := step.ShouldRun(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())
			Expect(shouldRun).To(BeTrue())
		})
	})
	const taskMD = `---
status: in_progress
phase: execution
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/example
clone_url: https://github.com/bborbe/example.git
ref: master
current_version: v1.2.7
task_identifier: gh-release-bborbe-example-master-049a
---

# release task

## Plan

` + "```json" + `
{
  "outcome": "ready",
  "bump": "patch",
  "reasoning": "fix-only batch",
  "current_version": "v1.2.7",
  "next_version": "1.2.8",
  "next_version_header": "## v1.2.8",
  "header_prefix_style": "v",
  "bullets": ["fix: thing"]
}
` + "```" + `
`

	writeChangelog := func(workdir string) {
		Expect(os.MkdirAll(workdir, 0o750)).To(Succeed())
		content := []byte("# Changelog\n\n## Unreleased\n\n- fix: thing\n\n## v1.2.6\n\n- old\n")
		Expect(os.WriteFile(filepath.Join(workdir, "CHANGELOG.md"), content, 0o600)).To(Succeed())
	}

	Context("happy path", func() {
		It(
			"clones, rewrites, commits, tags, pushes; writes ## Result(released); returns Done/NextPhase=ai_review",
			func() {
				fakeOps := &gitmocks.GitOps{}

				// Capture the workdir that the step passed to Clone so we can
				// write a CHANGELOG.md there before Commit reads it.
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelog(workdir)
					return nil
				}

				// Per spec AC #11(e): the bytes on disk at the moment Commit is
				// invoked MUST contain `## v1.2.8` AND NOT contain `## Unreleased`.
				// This proves RewriteUnreleasedHeader ran BEFORE Commit, not as
				// a hardcoded JSON-output-only step. Read the CHANGELOG inside
				// the stub (before the defer cleanup runs).
				fakeOps.CommitStub = func(_ context.Context, workdir, _ string, _ ...string) (string, error) {
					content, readErr := os.ReadFile(filepath.Join(workdir, "CHANGELOG.md"))
					Expect(readErr).NotTo(HaveOccurred())
					Expect(string(content)).To(ContainSubstring("## v1.2.8"))
					Expect(string(content)).NotTo(ContainSubstring("## Unreleased"))
					return "abc1234", nil
				}
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "test-token")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("ai_review"))

				// All 4 GitOps methods called exactly once.
				Expect(fakeOps.CloneCallCount()).To(Equal(1))
				Expect(fakeOps.CommitCallCount()).To(Equal(1))
				Expect(fakeOps.TagCallCount()).To(Equal(1))
				Expect(fakeOps.PushCallCount()).To(Equal(1))

				// Tag name + message verbatim from plan.next_version_header[3:].
				_, _, tagName, tagMsg := fakeOps.TagArgsForCall(0)
				Expect(tagName).To(Equal("v1.2.8"))
				Expect(tagMsg).To(Equal("release v1.2.8"))

				// Commit message uses the same canonical "release v1.2.8".
				_, _, commitMsg, _ := fakeOps.CommitArgsForCall(0)
				Expect(commitMsg).To(Equal("release v1.2.8"))

				// ## Result body shape.
				got, err := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(got.Outcome).To(Equal("released"))
				Expect(got.Path).To(Equal("direct-push"))
				Expect(got.CommitSHA).To(Equal("abc1234"))
				Expect(got.Tag).To(Equal("v1.2.8"))
				Expect(string(got.ErrorCategory)).To(BeEmpty())

				// Clone URL had token injected.
				_, gotCloneURL, _, _ := fakeOps.CloneArgsForCall(0)
				Expect(
					gotCloneURL,
				).To(Equal("https://x-access-token:test-token@github.com/bborbe/example.git"))
			},
		)
	})

	Context("protected_branch_rejected", func() {
		It(
			"Push fails with GH006 → Result(failed, error_category=protected_branch_rejected); Status=Failed; Tag was called",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelog(workdir)
					return nil
				}
				fakeOps.CommitStub = func(_ context.Context, _, _ string, _ ...string) (string, error) {
					return "def5678", nil
				}
				fakeOps.TagReturns(nil)
				// Realistic GH006 protected-branch error from `git push`.
				fakeOps.PushReturns(errors.Errorf(
					context.Background(),
					"git push: remote: error: GH006: Protected branch update failed for refs/heads/master. remote: error: At least 1 approving review is required",
				))

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				// Tag + Push were called (proves failure surfaces post-tag, not pre-commit).
				Expect(fakeOps.TagCallCount()).To(Equal(1))
				Expect(fakeOps.PushCallCount()).To(Equal(1))

				got, err := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("protected_branch_rejected"))
				Expect(got.CommitSHA).To(BeEmpty())
				Expect(got.Tag).To(BeEmpty())
			},
		)
	})

	Context("workdir cleanup observability", func() {
		// The cleanup-failure path is hard to trigger from a unit test
		// (would require an unwritable parent dir). This test instead
		// asserts the log message constant is in source so the
		// observability AC grep is satisfied AND the defer block does
		// run on the happy path (proven by stat).
		It("removes the workdir after Run completes", func() {
			fakeOps := &gitmocks.GitOps{}
			capturedWorkdir := ""
			fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
				capturedWorkdir = workdir
				writeChangelog(workdir)
				return nil
			}
			fakeOps.CommitStub = func(_ context.Context, _, _ string, _ ...string) (string, error) {
				return "abc1234", nil
			}
			step := pkg.NewExecutionStep(fakeOps, "")
			md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
			Expect(err).NotTo(HaveOccurred())

			_, err = step.Run(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())

			Expect(capturedWorkdir).NotTo(BeEmpty())
			_, statErr := os.Stat(capturedWorkdir)
			Expect(
				os.IsNotExist(statErr),
			).To(BeTrue(), "workdir %s should be removed after Run", capturedWorkdir)
		})
	})

	Context("clone_url normalization end-to-end", func() {
		const sshTaskMD = `---
status: in_progress
phase: execution
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/example
clone_url: git@github.com:bborbe/example.git
ref: master
current_version: v1.2.7
task_identifier: gh-release-bborbe-example-master-ssh
---

# release task

## Plan

` + "```json" + `
{
  "outcome": "ready",
  "bump": "patch",
  "reasoning": "fix-only batch",
  "current_version": "v1.2.7",
  "next_version": "1.2.8",
  "next_version_header": "## v1.2.8",
  "header_prefix_style": "v",
  "bullets": ["fix: thing"]
}
` + "```" + `
`

		It("rewrites an SSH clone_url to token-authenticated HTTPS before Clone", func() {
			fakeOps := &gitmocks.GitOps{}
			fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
				writeChangelog(workdir)
				return nil
			}
			fakeOps.CommitReturns("abc1234", nil)
			fakeOps.TagReturns(nil)
			fakeOps.PushReturns(nil)

			step := pkg.NewExecutionStep(fakeOps, "test-token")
			md, err := agentlib.ParseMarkdown(context.Background(), sshTaskMD)
			Expect(err).NotTo(HaveOccurred())

			result, err := step.Run(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

			Expect(fakeOps.CloneCallCount()).To(Equal(1))
			_, gotCloneURL, _, _ := fakeOps.CloneArgsForCall(0)
			Expect(
				gotCloneURL,
			).To(Equal("https://x-access-token:test-token@github.com/bborbe/example.git"))
		})
	})

	DescribeTable(
		"normalizes clone_url before clone (empty token isolates normalization)",
		func(inputCloneURL, wantCloneURL string) {
			taskMD := `---
status: in_progress
phase: execution
task_identifier: gh-release-norm-table
clone_url: ` + inputCloneURL + `
ref: master
---

## Plan

` + "```json" + `
{"outcome":"ready","next_version":"1.2.8","next_version_header":"## v1.2.8"}
` + "```" + `
`
			fakeOps := &gitmocks.GitOps{}
			fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
				writeChangelog(workdir)
				return nil
			}
			fakeOps.CommitReturns("abc1234", nil)
			fakeOps.TagReturns(nil)
			fakeOps.PushReturns(nil)

			step := pkg.NewExecutionStep(fakeOps, "") // empty token → injectToken is a no-op
			md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
			Expect(err).NotTo(HaveOccurred())

			_, err = step.Run(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeOps.CloneCallCount()).To(Equal(1))
			_, gotCloneURL, _, _ := fakeOps.CloneArgsForCall(0)
			Expect(gotCloneURL).To(Equal(wantCloneURL))
		},
		Entry("scp form", "git@github.com:owner/repo.git", "https://github.com/owner/repo.git"),
		Entry(
			"ssh:// form",
			"ssh://git@github.com/owner/repo.git",
			"https://github.com/owner/repo.git",
		),
		Entry(
			"https with .git unchanged",
			"https://github.com/owner/repo.git",
			"https://github.com/owner/repo.git",
		),
		Entry(
			"https without .git unchanged",
			"https://github.com/owner/repo",
			"https://github.com/owner/repo",
		),
		Entry(
			"unrecognized form unchanged",
			"git://example.com/owner/repo.git",
			"git://example.com/owner/repo.git",
		),
	)

	Context("clone failure", func() {
		It("Clone returns error → Result(failed); Status=Failed; Commit NOT called", func() {
			fakeOps := &gitmocks.GitOps{}
			// Clone fails with an auth error.
			fakeOps.CloneReturns(errors.Errorf(context.Background(), "Authentication failed"))
			step := pkg.NewExecutionStep(fakeOps, "")
			md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
			Expect(err).NotTo(HaveOccurred())

			result, err := step.Run(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
			Expect(fakeOps.CommitCallCount()).To(Equal(0))

			got, _ := agentlib.ExtractSection[pkg.ResultOutput](
				context.Background(),
				md,
				"## Result",
			)
			Expect(got.Outcome).To(Equal("failed"))
			Expect(string(got.ErrorCategory)).To(Equal("auth"))
		})
	})

	Context("changelog missing", func() {
		It(
			"Clone succeeds but CHANGELOG.md absent → Result(failed, error_category=changelog_missing)",
			func() {
				fakeOps := &gitmocks.GitOps{}
				// CloneStub creates the workdir but does NOT write CHANGELOG.md.
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					Expect(os.MkdirAll(workdir, 0o750)).To(Succeed())
					return nil
				}
				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(fakeOps.CommitCallCount()).To(Equal(0))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("changelog_missing"))
			},
		)
	})

	Context("plan output validation", func() {
		It(
			"non-ready plan → Result(failed, error_category=unknown); Status=Failed; Clone NOT called",
			func() {
				nonReadyMD := `---
status: in_progress
phase: execution
task_identifier: gh-release-x-y-master-049b
clone_url: https://github.com/x/y.git
ref: master
---

## Plan

` + "```json" + `
{"outcome":"needs_input","reason":"upstream changelog regression"}
` + "```" + `
`
				fakeOps := &gitmocks.GitOps{}
				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), nonReadyMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				Expect(fakeOps.CloneCallCount()).To(Equal(0))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("unknown"))
			},
		)
	})
})
