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
		func(input string, maxSlug int, want string) {
			Expect(slugifyTitle(input, maxSlug)).To(Equal(want))
		},
		Entry("simple lowercase", "fix bug", DefaultMaxSlugLen, "fix-bug"),
		Entry("uppercase converted", "Fix Bug", DefaultMaxSlugLen, "fix-bug"),
		Entry(
			"special chars replaced with hyphen",
			"feat: new-feature!",
			DefaultMaxSlugLen,
			"feat-new-feature",
		),
		Entry(
			"consecutive special chars collapsed to one hyphen",
			"hello   world",
			DefaultMaxSlugLen,
			"hello-world",
		),
		Entry("leading special char stripped", "!leading", DefaultMaxSlugLen, "leading"),
		Entry("trailing special char stripped", "trailing!", DefaultMaxSlugLen, "trailing"),
		Entry("only special chars → empty string", "!!!", DefaultMaxSlugLen, ""),
		Entry("empty string → empty string", "", DefaultMaxSlugLen, ""),
		Entry("unicode-only → empty string", "🚀🎉", DefaultMaxSlugLen, ""),
		Entry("mixed unicode and ascii", "fix 🐛 bug", DefaultMaxSlugLen, "fix-bug"),
		Entry("digits preserved", "v1 release", DefaultMaxSlugLen, "v1-release"),
		Entry("already slug-safe", "my-feature", DefaultMaxSlugLen, "my-feature"),
		Entry(
			"49-char input not truncated at default cap",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
			DefaultMaxSlugLen,
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
		),
		Entry(
			"truncation at custom cap 50 trims trailing hyphen",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm-extra-words-here",
			50,
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
		),
		Entry(
			"truncation at default cap 80 trims trailing hyphen",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz abcdefghijklmnop more",
			DefaultMaxSlugLen,
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz-abcdefghijklmnop",
		),
		Entry(
			"pr title with colon",
			"feat: add new endpoint",
			DefaultMaxSlugLen,
			"feat-add-new-endpoint",
		),
		Entry("pr title with slash", "fix/auth bug", DefaultMaxSlugLen, "fix-auth-bug"),
		Entry("pr title with dots", "bump v1.2.3", DefaultMaxSlugLen, "bump-v1-2-3"),
	)
})

var _ = Describe("computePRTitle", func() {
	DescribeTable(
		"produces correct title",
		func(provider, owner, repo string, number int, title string, maxSlug, maxTitle int, want string) {
			Expect(
				computePRTitle(provider, owner, repo, number, title, maxSlug, maxTitle),
			).To(Equal(want))
		},
		Entry("normal PR with title",
			"github", "bborbe", "maintainer", 2, "test: delete this PR never",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
			"PR Review github - bborbe-maintainer - 2 - test-delete-this-pr-never"),
		Entry("title with special chars",
			"github", "bborbe", "trading", 110, "fix: chromium trixie",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
			"PR Review github - bborbe-trading - 110 - fix-chromium-trixie"),
		Entry("empty title → no slug segment",
			"github", "bborbe", "x", 7, "",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
			"PR Review github - bborbe-x - 7"),
		Entry("whitespace-only title → no slug segment",
			"github", "bborbe", "x", 7, "   ",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
			"PR Review github - bborbe-x - 7"),
		Entry("unicode-only title → no slug segment",
			"github", "bborbe", "x", 7, "🚀🎉",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
			"PR Review github - bborbe-x - 7"),
		Entry(
			"slug truncated at default cap 80",
			"github",
			"org",
			"repo",
			1,
			"this is a very long pull request title that exceeds the maximum slug length limit here",
			DefaultMaxSlugLen,
			DefaultMaxTitleLen,
			"PR Review github - org-repo - 1 - this-is-a-very-long-pull-request-title-that-exceeds-the-maximum-slug-length-limi",
		),
		Entry(
			"slug truncated at custom cap 30 with hyphen-trim",
			"github",
			"bborbe",
			"x",
			1,
			"abcdefghijklmnopqrstuvwxyz ab more",
			30, DefaultMaxTitleLen,
			"PR Review github - bborbe-x - 1 - abcdefghijklmnopqrstuvwxyz-ab",
		),
		Entry(
			"title cap kicks in: slug under slug-cap but full title truncated at maxTitle",
			"github",
			"bborbe",
			"maintainer",
			1,
			"very long title",
			80, 40,
			"PR Review github - bborbe-maintainer - 1",
		),
		Entry("future bitbucket provider",
			"bitbucket", "team", "svc", 42, "fix auth bug",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
			"PR Review bitbucket - team-svc - 42 - fix-auth-bug"),
		Entry("hyphenated repo name joined correctly",
			"github", "my-org", "my-repo", 99, "bump deps",
			DefaultMaxSlugLen, DefaultMaxTitleLen,
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
		"computePRTitle output passes task.CreateCommand.Validate",
		func(provider, owner, repo string, number int, prTitle string) {
			title := computePRTitle(
				provider,
				owner,
				repo,
				number,
				prTitle,
				DefaultMaxSlugLen,
				DefaultMaxTitleLen,
			)
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
