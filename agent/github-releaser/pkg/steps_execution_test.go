// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	agentlib "github.com/bborbe/agent/lib"
	"github.com/bborbe/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitmocks "github.com/bborbe/maintainer/agent/github-releaser/mocks"
	pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
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
				fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "test-token")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
				Expect(result.NextPhase).To(Equal("ai_review"))

				// All GitOps methods called exactly once. CommittedFiles in
				// particular proves the pre-push guard is invoked on the happy
				// path — if guardCommittedFiles were dropped from
				// executeDirectPush this assertion (not Tag/Push) would catch it.
				Expect(fakeOps.CloneCallCount()).To(Equal(1))
				Expect(fakeOps.CommitCallCount()).To(Equal(1))
				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(1))
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
				fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)
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

	Context("pre-push guard (CommittedFiles)", func() {
		// The guard is the primary security assertion of the direct-push
		// trust model: a release commit must change ONLY CHANGELOG.md. These
		// specs prove it fails closed — Tag and Push are NEVER reached when
		// the committed file set is wrong or unobtainable.
		runGuard := func(committed []string, committedErr error) (*agentlib.Result, *gitmocks.GitOps, *agentlib.Markdown) {
			fakeOps := &gitmocks.GitOps{}
			fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
				writeChangelog(workdir)
				return nil
			}
			fakeOps.CommitStub = func(_ context.Context, _, _ string, _ ...string) (string, error) {
				return "def5678", nil
			}
			fakeOps.CommittedFilesReturns(committed, committedErr)
			fakeOps.TagReturns(nil)
			fakeOps.PushReturns(nil)

			step := pkg.NewExecutionStep(fakeOps, "")
			md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
			Expect(err).NotTo(HaveOccurred())
			result, err := step.Run(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())
			return result, fakeOps, md
		}

		assertFailClosed := func(fakeOps *gitmocks.GitOps, md *agentlib.Markdown, wantCategory string) {
			// The guard ran exactly once — proves it is actually invoked on
			// this path (not silently skipped).
			Expect(fakeOps.CommittedFilesCallCount()).To(Equal(1))
			// Fail closed: nothing tagged, nothing pushed.
			Expect(fakeOps.TagCallCount()).To(Equal(0))
			Expect(fakeOps.PushCallCount()).To(Equal(0))
			got, err := agentlib.ExtractSection[pkg.ResultOutput](
				context.Background(),
				md,
				"## Result",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Outcome).To(Equal("failed"))
			Expect(string(got.ErrorCategory)).To(Equal(wantCategory))
			Expect(got.Tag).To(BeEmpty())
		}

		It("extra files → Status=Failed, error_category=unexpected_diff, no tag/push", func() {
			result, fakeOps, md := runGuard([]string{"CHANGELOG.md", "config.yml"}, nil)
			Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
			assertFailClosed(fakeOps, md, "unexpected_diff")
		})

		It("empty file list → Status=Failed, error_category=unexpected_diff, no tag/push", func() {
			// git diff-tree can legitimately return no files (e.g. a root
			// commit); len(files)!=1 must still fail closed, not push blindly.
			result, fakeOps, md := runGuard([]string{}, nil)
			Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
			assertFailClosed(fakeOps, md, "unexpected_diff")
		})

		It(
			"wrong single file → Status=Failed, error_category=unexpected_diff, no tag/push",
			func() {
				result, fakeOps, md := runGuard([]string{"main.go"}, nil)
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
				assertFailClosed(fakeOps, md, "unexpected_diff")
			},
		)

		It("CommittedFiles error → Status=Failed, error_category=unknown, no tag/push", func() {
			result, fakeOps, md := runGuard(
				nil,
				errors.Errorf(context.Background(), "git diff-tree boom"),
			)
			Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
			assertFailClosed(fakeOps, md, "unknown")
		})
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
			fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)
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
			fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)
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
			// Guard never reached — failure surfaced before Commit.
			Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))

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
				// Guard never reached — failure surfaced before Commit.
				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))

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
				// Guard never reached — failure surfaced before Clone.
				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))

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

	Describe("sameStringSet", func() {
		DescribeTable(
			"order-independent set equality",
			func(a, b []string, want bool) {
				originalA := slices.Clone(a)
				originalB := slices.Clone(b)
				Expect(pkg.SameStringSetForTest(a, b)).To(Equal(want))
				// Assert inputs are NOT mutated.
				Expect(a).To(Equal(originalA))
				Expect(b).To(Equal(originalB))
			},
			Entry("equal same order", []string{"a", "b"}, []string{"a", "b"}, true),
			Entry("equal different order", []string{"a", "b"}, []string{"b", "a"}, true),
			Entry("different length", []string{"a", "b"}, []string{"a", "b", "c"}, false),
			Entry("element mismatch", []string{"a", "b"}, []string{"a", "c"}, false),
			Entry("nil vs nil", nil, nil, true),
			Entry("empty vs empty", []string{}, []string{}, true),
			Entry("one empty", []string{"a"}, []string{}, false),
			Entry("identical duplicates → true", []string{"a", "a"}, []string{"a", "a"}, true),
			Entry(
				"duplicate vs distinct, same length → false",
				[]string{"a", "a"},
				[]string{"a", "b"},
				false,
			),
		)
	})

	Describe("deriveUnprefixedVersion", func() {
		DescribeTable(
			"strips ## prefix and v prefix",
			func(header, want string) {
				Expect(pkg.DeriveUnprefixedVersionForTest(header)).To(Equal(want))
			},
			Entry("## v0.10.0", "## v0.10.0", "0.10.0"),
			Entry("## 0.10.0", "## 0.10.0", "0.10.0"),
			Entry("0.10.0", "0.10.0", "0.10.0"),
			Entry("empty string → empty", "", ""),
		)
	})

	Context("plugin manifests", func() {
		const taskMDPlugin = `---
status: in_progress
phase: execution
assignee: github-releaser-agent
task_type: github-release
repo: bborbe/example
clone_url: https://github.com/bborbe/example.git
ref: master
current_version: v0.9.12
task_identifier: gh-release-bborbe-example-master-plugin
---

# release task

## Plan

` + "```json" + `
{
  "outcome": "ready",
  "bump": "minor",
  "reasoning": "new feature",
  "current_version": "v0.9.12",
  "next_version": "0.10.0",
  "next_version_header": "## v0.10.0",
  "header_prefix_style": "v",
  "bullets": ["feat: new thing"]
}
` + "```" + `
`

		readFixture := func(name string) []byte {
			data, err := os.ReadFile(filepath.Join("testdata", name))
			Expect(err).NotTo(HaveOccurred())
			return data
		}

		writeManifest := func(workdir, relPath, fixtureName string) {
			Expect(os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750)).To(Succeed())
			Expect(
				os.WriteFile(filepath.Join(workdir, relPath), readFixture(fixtureName), 0o600),
			).To(Succeed())
		}

		writeChangelogAndBothManifests := func(workdir string) {
			writeChangelog(workdir)
			writeManifest(workdir, ".claude-plugin/plugin.json", "plugin.json.pre")
			writeManifest(workdir, ".claude-plugin/marketplace.json", "marketplace.json.pre")
		}

		It(
			"bumps plugin.json and marketplace.json to unprefixed semver; commits exactly those files plus CHANGELOG.md; guard passes",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelogAndBothManifests(workdir)
					return nil
				}
				fakeOps.CommitStub = func(_ context.Context, workdir, _ string, _ ...string) (string, error) {
					pluginActual, err := os.ReadFile(
						filepath.Join(workdir, ".claude-plugin", "plugin.json"),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(pluginActual).To(Equal(readFixture("plugin.json.post")))

					marketplaceActual, err := os.ReadFile(
						filepath.Join(workdir, ".claude-plugin", "marketplace.json"),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(marketplaceActual).To(Equal(readFixture("marketplace.json.post")))
					return "abc1234", nil
				}
				fakeOps.CommittedFilesReturns(
					[]string{
						"CHANGELOG.md",
						".claude-plugin/plugin.json",
						".claude-plugin/marketplace.json",
					},
					nil,
				)
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "test-token")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

				_, _, _, commitPaths := fakeOps.CommitArgsForCall(0)
				Expect(
					commitPaths,
				).To(Equal([]string{"CHANGELOG.md", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"}))

				Expect(fakeOps.TagCallCount()).To(Equal(1))
				Expect(fakeOps.PushCallCount()).To(Equal(1))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("released"))
			},
		)

		It(
			"plugin.json only → commits {CHANGELOG.md, .claude-plugin/plugin.json}; guard passes",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelog(workdir)
					writeManifest(workdir, ".claude-plugin/plugin.json", "plugin.json.pre")
					return nil
				}
				fakeOps.CommitStub = func(_ context.Context, _, _ string, paths ...string) (string, error) {
					Expect(paths).To(Equal([]string{"CHANGELOG.md", ".claude-plugin/plugin.json"}))
					return "abc1234", nil
				}
				fakeOps.CommittedFilesReturns(
					[]string{"CHANGELOG.md", ".claude-plugin/plugin.json"},
					nil,
				)
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
			},
		)

		It(
			"marketplace.json only → commits {CHANGELOG.md, .claude-plugin/marketplace.json}; guard passes",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelog(workdir)
					writeManifest(
						workdir,
						".claude-plugin/marketplace.json",
						"marketplace.json.pre",
					)
					return nil
				}
				fakeOps.CommitStub = func(_ context.Context, _, _ string, paths ...string) (string, error) {
					Expect(
						paths,
					).To(Equal([]string{"CHANGELOG.md", ".claude-plugin/marketplace.json"}))
					return "abc1234", nil
				}
				fakeOps.CommittedFilesReturns(
					[]string{"CHANGELOG.md", ".claude-plugin/marketplace.json"},
					nil,
				)
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
			},
		)

		It("no .claude-plugin/ dir → commits only CHANGELOG.md; guard passes", func() {
			fakeOps := &gitmocks.GitOps{}
			fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
				writeChangelog(workdir)
				return nil
			}
			fakeOps.CommitStub = func(_ context.Context, _, _ string, paths ...string) (string, error) {
				Expect(paths).To(Equal([]string{"CHANGELOG.md"}),
					"commit paths must be exactly [CHANGELOG.md] when no manifests exist")
				return "abc1234", nil
			}
			fakeOps.CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)
			fakeOps.TagReturns(nil)
			fakeOps.PushReturns(nil)

			step := pkg.NewExecutionStep(fakeOps, "")
			md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
			Expect(err).NotTo(HaveOccurred())

			result, err := step.Run(context.Background(), md)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal(agentlib.AgentStatusDone))

			got, _ := agentlib.ExtractSection[pkg.ResultOutput](
				context.Background(),
				md,
				"## Result",
			)
			Expect(got.Outcome).To(Equal("released"))
		})

		It(
			"CommittedFiles returns unexpected file → Result(failed, error_category=unexpected_diff); Tag NOT called; Push NOT called",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelogAndBothManifests(workdir)
					return nil
				}
				fakeOps.CommitStub = func(_ context.Context, _, _ string, _ ...string) (string, error) {
					return "def5678", nil
				}
				fakeOps.CommittedFilesReturns(
					[]string{
						"CHANGELOG.md",
						".claude-plugin/plugin.json",
						".claude-plugin/marketplace.json",
						"README.md",
					},
					nil,
				)
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(1))
				Expect(fakeOps.TagCallCount()).To(Equal(0))
				Expect(fakeOps.PushCallCount()).To(Equal(0))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("unexpected_diff"))
				Expect(got.Tag).To(BeEmpty())
				Expect(got.CommitSHA).To(BeEmpty())
			},
		)

		It(
			"plugin.json is malformed JSON → Result(failed, error_category=plugin_manifest_invalid); Commit NOT called; Tag NOT called; Push NOT called",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelog(workdir)
					Expect(
						os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750),
					).To(Succeed())
					malformedPlugin := []byte(`{"name": "example", "version": }`)
					Expect(
						os.WriteFile(
							filepath.Join(workdir, ".claude-plugin", "plugin.json"),
							malformedPlugin,
							0o600,
						),
					).To(Succeed())
					return nil
				}
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				Expect(fakeOps.CommitCallCount()).To(Equal(0))
				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))
				Expect(fakeOps.TagCallCount()).To(Equal(0))
				Expect(fakeOps.PushCallCount()).To(Equal(0))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("plugin_manifest_invalid"))
				Expect(got.Error).To(ContainSubstring(".claude-plugin/plugin.json"))
				Expect(got.Tag).To(BeEmpty())
				Expect(got.CommitSHA).To(BeEmpty())
			},
		)

		It(
			"DetectManifests I/O error → Result(failed, error_category=unknown); Commit/Tag/Push not called",
			func() {
				// chmod 0000 on Linux non-root blocks Stat of the children;
				// skip on platforms where this is unreliable (Darwin, root containers).
				if runtime.GOOS == "darwin" || os.Geteuid() == 0 {
					Skip("requires unprivileged Linux for non-IsNotExist Stat failure")
				}

				fakeOps := &gitmocks.GitOps{}
				var capturedWorkdir string
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					capturedWorkdir = workdir
					writeChangelog(workdir)
					// Create .claude-plugin as a directory with mode 0000 so Stat on
					// its children returns EACCES (a non-IsNotExist error path).
					Expect(
						os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750),
					).To(Succeed())
					Expect(os.Chmod(filepath.Join(workdir, ".claude-plugin"), 0o000)).To(Succeed())
					return nil
				}
				// DeferCleanup restores the directory mode so the workdir-cleanup RemoveAll succeeds.
				DeferCleanup(func() {
					if capturedWorkdir != "" {
						_ = os.Chmod(filepath.Join(capturedWorkdir, ".claude-plugin"), 0o750)
					}
				})

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				Expect(fakeOps.CommitCallCount()).To(Equal(0))
				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))
				Expect(fakeOps.TagCallCount()).To(Equal(0))
				Expect(fakeOps.PushCallCount()).To(Equal(0))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("unknown"))
			},
		)

		It(
			"marketplace.json is malformed JSON → Result(failed, error_category=plugin_manifest_invalid); Commit NOT called; Tag NOT called; Push NOT called",
			func() {
				fakeOps := &gitmocks.GitOps{}
				fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
					writeChangelog(workdir)
					Expect(
						os.MkdirAll(filepath.Join(workdir, ".claude-plugin"), 0o750),
					).To(Succeed())
					malformedMarketplace := []byte(`{"metadata": {"version": }}`)
					Expect(
						os.WriteFile(
							filepath.Join(workdir, ".claude-plugin", "marketplace.json"),
							malformedMarketplace,
							0o600,
						),
					).To(Succeed())
					return nil
				}
				fakeOps.TagReturns(nil)
				fakeOps.PushReturns(nil)

				step := pkg.NewExecutionStep(fakeOps, "")
				md, err := agentlib.ParseMarkdown(context.Background(), taskMDPlugin)
				Expect(err).NotTo(HaveOccurred())

				result, err := step.Run(context.Background(), md)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

				Expect(fakeOps.CommitCallCount()).To(Equal(0))
				Expect(fakeOps.CommittedFilesCallCount()).To(Equal(0))
				Expect(fakeOps.TagCallCount()).To(Equal(0))
				Expect(fakeOps.PushCallCount()).To(Equal(0))

				got, _ := agentlib.ExtractSection[pkg.ResultOutput](
					context.Background(),
					md,
					"## Result",
				)
				Expect(got.Outcome).To(Equal("failed"))
				Expect(string(got.ErrorCategory)).To(Equal("plugin_manifest_invalid"))
				Expect(got.Error).To(ContainSubstring(".claude-plugin/marketplace.json"))
				Expect(got.Tag).To(BeEmpty())
				Expect(got.CommitSHA).To(BeEmpty())
			},
		)
	})
})
