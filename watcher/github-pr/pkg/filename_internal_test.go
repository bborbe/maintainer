// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"

	agentlib "github.com/bborbe/agent/lib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("slugifyTitle", func() {
	DescribeTable(
		"produces correct slug",
		func(input, want string) {
			Expect(slugifyTitle(input)).To(Equal(want))
		},
		Entry("simple lowercase", "fix bug", "fix-bug"),
		Entry("uppercase converted", "Fix Bug", "fix-bug"),
		Entry("special chars replaced with hyphen", "feat: new-feature!", "feat-new-feature"),
		Entry("consecutive special chars collapsed to one hyphen", "hello   world", "hello-world"),
		Entry("leading special char stripped", "!leading", "leading"),
		Entry("trailing special char stripped", "trailing!", "trailing"),
		Entry("only special chars → empty string", "!!!", ""),
		Entry("empty string → empty string", "", ""),
		Entry("unicode-only → empty string", "🚀🎉", ""),
		Entry("mixed unicode and ascii", "fix 🐛 bug", "fix-bug"),
		Entry("digits preserved", "v1 release", "v1-release"),
		Entry("already slug-safe", "my-feature", "my-feature"),
		Entry(
			"truncation at 50 chars",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
		),
		Entry(
			"truncation trims trailing hyphen",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm-extra-words-here",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
		),
		Entry("pr title with colon", "feat: add new endpoint", "feat-add-new-endpoint"),
		Entry("pr title with slash", "fix/auth bug", "fix-auth-bug"),
		Entry("pr title with dots", "bump v1.2.3", "bump-v1-2-3"),
	)
})

var _ = Describe("computePRFilenameHint", func() {
	DescribeTable("produces correct filename hint",
		func(provider, owner, repo string, number int, title, want string) {
			Expect(computePRFilenameHint(provider, owner, repo, number, title)).To(Equal(want))
		},
		Entry("normal PR with title",
			"github", "bborbe", "maintainer", 2, "test: delete this PR never",
			"PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never"),
		Entry("title with special chars",
			"github", "bborbe", "trading", 110, "fix: chromium trixie",
			"PR Review github - bborbe-trading - 110 - fix-chromium-trixie"),
		Entry("empty title → no slug segment",
			"github", "bborbe", "x", 7, "",
			"PR Review github - bborbe-x - 7"),
		Entry("whitespace-only title → no slug segment",
			"github", "bborbe", "x", 7, "   ",
			"PR Review github - bborbe-x - 7"),
		Entry("unicode-only title → no slug segment",
			"github", "bborbe", "x", 7, "🚀🎉",
			"PR Review github - bborbe-x - 7"),
		Entry(
			"slug truncated at 50 chars",
			"github",
			"org",
			"repo",
			1,
			"this is a very long pull request title that exceeds the maximum slug length limit here",
			"PR Review github - org-repo - 1 - this-is-a-very-long-pull-request-title-that-exceed",
		),
		Entry("future bitbucket provider",
			"bitbucket", "team", "svc", 42, "fix auth bug",
			"PR Review bitbucket - team-svc - 42 - fix-auth-bug"),
		Entry("hyphenated repo name joined correctly",
			"github", "my-org", "my-repo", 99, "bump deps",
			"PR Review github - my-org-my-repo - 99 - bump-deps"),
	)
})

// JSON marshal contract: lock the wire-format tag for the controller boundary.
var _ = Describe("WatcherCreateTaskCommand JSON marshalling", func() {
	It("emits filename_hint as a top-level snake_case field alongside embedded fields", func() {
		cmd := WatcherCreateTaskCommand{
			CreateTaskCommand: agentlib.CreateTaskCommand{
				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
				Frontmatter:    agentlib.TaskFrontmatter{"assignee": "pr-reviewer-agent"},
				Body:           "# body",
			},
			FilenameHint: "PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never",
		}
		raw, err := json.Marshal(cmd)
		Expect(err).NotTo(HaveOccurred())

		// Top-level snake_case key consumed by the controller
		Expect(
			string(raw),
		).To(ContainSubstring(`"filename_hint":"PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never"`))

		// Embedded fields stay at top level (struct embedding, not nested)
		Expect(
			string(raw),
		).To(ContainSubstring(`"taskIdentifier":"00000000-0000-0000-0000-000000000000"`))

		// No nesting under a "CreateTaskCommand" key
		Expect(string(raw)).NotTo(ContainSubstring(`"CreateTaskCommand"`))
	})

	It("omits filename_hint when empty (omitempty contract)", func() {
		cmd := WatcherCreateTaskCommand{
			CreateTaskCommand: agentlib.CreateTaskCommand{
				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
				Frontmatter:    agentlib.TaskFrontmatter{},
				Body:           "",
			},
			FilenameHint: "",
		}
		raw, err := json.Marshal(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring(`"filename_hint"`))
	})
})
