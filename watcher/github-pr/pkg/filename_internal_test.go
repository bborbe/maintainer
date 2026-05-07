// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"encoding/json"

	agentlib "github.com/bborbe/agent/lib"
	task "github.com/bborbe/agent/lib/command/task"
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

// Wire-format contract: lock the JSON key for the task.CreateCommand boundary.
var _ = Describe("task.CreateCommand wire format", func() {
	It("emits 'title' as the top-level key (not 'filename_hint')", func() {
		cmd := task.CreateCommand{
			Title:          "PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never",
			TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
			Frontmatter:    agentlib.TaskFrontmatter{"assignee": "pr-reviewer-agent"},
			Body:           "# body",
		}
		raw, err := json.Marshal(cmd)
		Expect(err).NotTo(HaveOccurred())

		Expect(
			string(raw),
		).To(ContainSubstring(`"title":"PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never"`))
		Expect(string(raw)).NotTo(ContainSubstring(`"filename_hint"`))
	})

	// Boundary contract: slug helper output MUST pass task.CreateCommand.Validate (level-1 contract test).
	// Prevents future drift between watcher's slug rules and lib's Title validator.
	DescribeTable(
		"computePRFilenameHint output passes task.CreateCommand.Validate",
		func(provider, owner, repo string, number int, prTitle string) {
			title := computePRFilenameHint(provider, owner, repo, number, prTitle)
			cmd := task.CreateCommand{
				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
				Title:          title,
				Frontmatter: agentlib.TaskFrontmatter{
					"assignee": "pr-reviewer-agent",
					"status":   "todo",
				},
				Body: "review the PR",
			}
			Expect(cmd.Validate(context.Background())).To(Succeed())
		},
		Entry("typical PR", "github", "bborbe", "maintainer", 2, "test: delete this PR never"),
		Entry("hyphenated repo", "github", "my-org", "my-repo", 99, "bump deps"),
		Entry(
			"special chars in title",
			"github",
			"bborbe",
			"trading",
			110,
			"fix: chromium @trixie [edge]",
		),
		Entry("empty title (slug omits segment)", "github", "bborbe", "x", 7, ""),
		Entry("unicode-only title (slug omits segment)", "github", "bborbe", "x", 7, "🚀🎉"),
	)
})
