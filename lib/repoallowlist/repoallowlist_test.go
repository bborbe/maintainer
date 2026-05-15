// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repoallowlist_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/lib/repoallowlist"
)

var _ = Describe("IsAllowed", func() {
	DescribeTable("IsAllowed",
		func(allowlist []string, target string, expected bool) {
			Expect(repoallowlist.IsAllowed(allowlist, target)).To(Equal(expected))
		},
		// Allow-all cases
		Entry("nil allowlist allows everything",
			nil, "github.com/bborbe/maintainer", true),
		Entry("empty allowlist allows everything",
			[]string{}, "github.com/bborbe/maintainer", true),
		Entry("nil allowlist allows empty target",
			nil, "", true),
		Entry("empty allowlist allows empty target",
			[]string{}, "", true),

		// Literal match
		Entry("literal entry matches exact target",
			[]string{"github.com/bborbe/maintainer"}, "github.com/bborbe/maintainer", true),
		Entry("literal entry does not match different repo",
			[]string{"github.com/bborbe/maintainer"}, "github.com/bborbe/other", false),
		Entry("literal match is case-sensitive — uppercase does not match lowercase entry",
			[]string{"github.com/bborbe/maintainer"}, "github.com/bborbe/Maintainer", false),
		Entry("literal match is case-sensitive — lowercase does not match uppercase entry",
			[]string{"github.com/bborbe/Maintainer"}, "github.com/bborbe/maintainer", false),

		// Wildcard match
		Entry("wildcard entry matches any repo under same owner",
			[]string{"github.com/bborbe/*"}, "github.com/bborbe/maintainer", true),
		Entry("wildcard entry matches another repo under same owner",
			[]string{"github.com/bborbe/*"}, "github.com/bborbe/other-repo", true),
		Entry("wildcard entry does NOT match different owner",
			[]string{"github.com/bborbe/*"}, "github.com/other-owner/maintainer", false),
		Entry("wildcard entry does NOT match different host",
			[]string{"github.com/bborbe/*"}, "gitlab.com/bborbe/maintainer", false),

		// Malformed entries — skipped, do not match
		Entry("entry with fewer than three segments is skipped",
			[]string{"github.com/bborbe"}, "github.com/bborbe", false),
		Entry("wildcard in owner position is skipped",
			[]string{"github.com/*/maintainer"}, "github.com/bborbe/maintainer", false),
		Entry("wildcard in host position is skipped",
			[]string{"*/bborbe/maintainer"}, "github.com/bborbe/maintainer", false),
		Entry("two wildcards (owner and repo) are skipped",
			[]string{"github.com/*/*"}, "github.com/bborbe/maintainer", false),

		// Empty target against non-empty allowlist
		Entry("empty target returns false against non-empty allowlist",
			[]string{"github.com/bborbe/*"}, "", false),

		// Whitespace handling
		Entry("entry with surrounding whitespace is trimmed and matched literally",
			[]string{"  github.com/bborbe/maintainer  "}, "github.com/bborbe/maintainer", true),
		Entry("entry with surrounding whitespace is trimmed for wildcard match",
			[]string{"  github.com/bborbe/*  "}, "github.com/bborbe/maintainer", true),
		Entry("whitespace-only entry is skipped",
			[]string{"   "}, "github.com/bborbe/maintainer", false),
		Entry("empty string entry is skipped",
			[]string{""}, "github.com/bborbe/maintainer", false),

		// Multiple entries — first match wins
		Entry(
			"second entry matches when first does not",
			[]string{
				"github.com/other/repo",
				"github.com/bborbe/maintainer",
			},
			"github.com/bborbe/maintainer",
			true,
		),
		Entry(
			"malformed entry skipped, valid entry behind it still matches",
			[]string{
				"github.com/bborbe",
				"github.com/bborbe/*",
			},
			"github.com/bborbe/maintainer",
			true,
		),
	)
})

var _ = Describe("Validate", func() {
	DescribeTable("Validate",
		func(allowlist []string, expectErr bool) {
			err := repoallowlist.Validate(context.Background(), allowlist)
			if expectErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("nil allowlist is valid", nil, false),
		Entry("empty allowlist is valid", []string{}, false),
		Entry("valid literal entry is valid",
			[]string{"github.com/bborbe/maintainer"}, false),
		Entry("valid wildcard entry is valid",
			[]string{"github.com/bborbe/*"}, false),
		Entry("mixed valid literal and wildcard is valid",
			[]string{"github.com/bborbe/maintainer", "github.com/other/*"}, false),
		Entry("whitespace-only entry is skipped (valid)",
			[]string{"   "}, false),
		Entry("empty string entry is skipped (valid)",
			[]string{""}, false),

		// Malformed entries — each causes a validation error
		Entry("fewer than three segments returns error",
			[]string{"github.com/bborbe"}, true),
		Entry("wildcard in owner position returns error",
			[]string{"github.com/*/maintainer"}, true),
		Entry("wildcard in host position returns error",
			[]string{"*/bborbe/maintainer"}, true),
		Entry("two wildcards (owner and repo) returns error",
			[]string{"github.com/*/*"}, true),

		// Aggregate: ALL malformed entries are reported, not just the first
		Entry("multiple malformed entries both appear in aggregate error",
			[]string{"github.com/bborbe", "github.com/*/foo"}, true),
	)

	It("Validate returns aggregate error mentioning each malformed entry", func() {
		allowlist := []string{"github.com/bborbe", "github.com/*/foo"}
		err := repoallowlist.Validate(context.Background(), allowlist)
		Expect(err).To(HaveOccurred())
		msg := err.Error()
		Expect(msg).To(ContainSubstring("github.com/bborbe"))
		Expect(msg).To(ContainSubstring("github.com/*/foo"))
	})
})
