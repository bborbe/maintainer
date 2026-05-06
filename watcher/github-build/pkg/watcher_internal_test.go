// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"encoding/json"
	"time"

	agentlib "github.com/bborbe/agent/lib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("splitRepoKey", func() {
	DescribeTable("parses allowlist key into owner and repo",
		func(key, wantOwner, wantRepo string) {
			gotOwner, gotRepo := splitRepoKey(key)
			Expect(gotOwner).To(Equal(wantOwner))
			Expect(gotRepo).To(Equal(wantRepo))
		},
		Entry("three-segment host/owner/repo", "github.com/owner/repo", "owner", "repo"),
		Entry("two-segment owner/repo", "owner/repo", "owner", "repo"),
		Entry("single segment", "single", "single", ""),
		Entry("empty string", "", "", ""),
		Entry("four segments (invalid)", "a/b/c/d", "a/b/c/d", ""),
	)
})

var _ = Describe("computeFilenameHint", func() {
	DescribeTable(
		"produces correct filename hint",
		func(provider, owner, repo, sha, want string) {
			Expect(computeFilenameHint(provider, owner, repo, sha)).To(Equal(want))
		},
		Entry(
			"normal github repo",
			"github",
			"bborbe",
			"maintainer",
			"5886450abcdef",
			"Build Failure github - bborbe-maintainer - 5886450",
		),
		Entry(
			"sha shorter than 7 chars",
			"github",
			"org",
			"repo",
			"abc12",
			"Build Failure github - org-repo - abc12",
		),
		Entry(
			"sha exactly 7 chars",
			"github",
			"org",
			"repo",
			"abc1234",
			"Build Failure github - org-repo - abc1234",
		),
		Entry(
			"sha longer than 7 chars is truncated to 7",
			"github",
			"org",
			"repo",
			"abc1234xyz",
			"Build Failure github - org-repo - abc1234",
		),
		Entry(
			"repo name with uppercase is slugified",
			"github",
			"MyOrg",
			"MyRepo",
			"abcdef0",
			"Build Failure github - myorg-myrepo - abcdef0",
		),
		Entry(
			"repo name with dot is slugified",
			"github",
			"org",
			"my.repo",
			"abcdef0",
			"Build Failure github - org-my-repo - abcdef0",
		),
		Entry(
			"repo name with colon (illegal on vault fs) is slugified",
			"github",
			"org",
			"my:repo",
			"abcdef0",
			"Build Failure github - org-my-repo - abcdef0",
		),
		Entry(
			"hyphenated names preserved",
			"github",
			"my-org",
			"my-repo",
			"abcdef0",
			"Build Failure github - my-org-my-repo - abcdef0",
		),
		Entry(
			"future bitbucket provider",
			"bitbucket",
			"team",
			"svc",
			"a1b2c3d",
			"Build Failure bitbucket - team-svc - a1b2c3d",
		),
	)
})

var _ = Describe("slugifySegment", func() {
	DescribeTable("produces filesystem-safe segment",
		func(input, want string) {
			Expect(slugifySegment(input)).To(Equal(want))
		},
		Entry("already safe lowercase", "bborbe", "bborbe"),
		Entry("uppercase converted", "MyOrg", "myorg"),
		Entry("dot replaced with hyphen", "my.repo", "my-repo"),
		Entry("colon replaced with hyphen", "my:repo", "my-repo"),
		Entry("asterisk replaced with hyphen", "my*repo", "my-repo"),
		Entry("leading special char stripped", ".leading", "leading"),
		Entry("trailing special char stripped", "trailing.", "trailing"),
		Entry("only special chars becomes empty", ":::", ""),
		Entry("mixed chars", "My.Org_1", "my-org-1"),
		Entry("digits preserved", "repo2", "repo2"),
		Entry("hyphen preserved", "my-repo", "my-repo"),
	)
})

// JSON marshal contract: lock the wire-format tag for the controller boundary.
var _ = Describe("WatcherCreateTaskCommand JSON marshalling", func() {
	It("emits filename_hint as a top-level snake_case field alongside embedded fields", func() {
		cmd := WatcherCreateTaskCommand{
			CreateTaskCommand: agentlib.CreateTaskCommand{
				TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
				Frontmatter:    agentlib.TaskFrontmatter{"assignee": "bborbe"},
				Body:           "# body",
			},
			FilenameHint: "Build Failure github - bborbe-maintainer - 5886450",
		}
		raw, err := json.Marshal(cmd)
		Expect(err).NotTo(HaveOccurred())

		// Top-level snake_case key for the controller's consumer
		Expect(
			string(raw),
		).To(ContainSubstring(`"filename_hint":"Build Failure github - bborbe-maintainer - 5886450"`))

		// Embedded fields stay at top level (struct embedding, not nested under a key)
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

var _ = Describe("formatDuration", func() {
	DescribeTable("formats duration as human-readable string",
		func(input time.Duration, want string) {
			Expect(formatDuration(input)).To(Equal(want))
		},
		Entry("zero duration returns empty", 0*time.Second, ""),
		Entry("negative duration returns empty", -1*time.Second, ""),
		Entry("seconds only", 47*time.Second, "47s"),
		Entry("minutes and seconds", 2*time.Minute+47*time.Second, "2m 47s"),
		Entry("hours, minutes, seconds", 1*time.Hour+5*time.Minute+3*time.Second, "1h 5m 3s"),
		Entry("exactly one minute", 60*time.Second, "1m 0s"),
		Entry("sub-500ms rounds to zero → empty", 499*time.Millisecond, ""),
		Entry("exactly 500ms rounds to 1s", 500*time.Millisecond, "1s"),
		Entry("1499ms rounds to 1s", 1499*time.Millisecond, "1s"),
		Entry("1500ms rounds to 2s", 1500*time.Millisecond, "2s"),
	)
})
