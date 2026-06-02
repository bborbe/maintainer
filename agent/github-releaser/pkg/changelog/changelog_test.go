// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package changelog_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
)

var _ = Describe("ValidateUnreleased", func() {
	DescribeTable("ValidateUnreleased",
		func(content []byte, valid bool, reason string, line int) {
			v, r, l := changelog.ValidateUnreleased(content)
			Expect(v).To(Equal(valid))
			Expect(r).To(Equal(reason))
			Expect(l).To(Equal(line))
		},
		Entry("P1 valid - Unreleased first",
			[]byte("# Changelog\n\n## Unreleased\n\n- feat: add foo\n\n## v1.0.0\n\n- initial\n"),
			true, "", 0),
		Entry(
			"P1 fail - Unreleased not first",
			[]byte(
				"# Changelog\n\n\n\n\n\n\n\n\n\n## 1.2.6\n\n- some fix\n\n## Unreleased\n\n- feat: add foo\n",
			),
			false,
			"Unreleased is not the first ## section; found '1.2.6' at line 11. Move ## Unreleased above all release headings.",
			11,
		),
		Entry("no Unreleased section",
			[]byte("# Changelog\n\n## v1.0.0\n\n- initial\n"),
			false, "Unreleased section not found.", 0),
		Entry(
			"P2 fail - empty Unreleased",
			[]byte(
				"# Changelog\n\n\n\n\n\n\n\n\n\n## Unreleased\n\nNo bullets here.\n\n## v1.0.0\n\n- initial\n",
			),
			false,
			"Unreleased section has no bullet entries.",
			11,
		),
		Entry("trailing whitespace heading tolerated",
			[]byte("# Changelog\n\n## Unreleased   \n\n- feat: add foo\n\n## v1.0.0\n"),
			true, "", 0),
		Entry("nil content returns not found",
			nil,
			false, "Unreleased section not found.", 0),
		Entry("empty content returns not found",
			[]byte{},
			false, "Unreleased section not found.", 0),
		Entry(
			"star bullet not counted",
			[]byte(
				"# Changelog\n\n\n\n\n\n\n\n\n\n## Unreleased\n\n* feat: add foo\n\n## v1.0.0\n",
			),
			false,
			"Unreleased section has no bullet entries.",
			11,
		),
		Entry(
			"plus bullet not counted",
			[]byte(
				"# Changelog\n\n\n\n\n\n\n\n\n\n## Unreleased\n\n+ feat: add foo\n\n## v1.0.0\n",
			),
			false,
			"Unreleased section has no bullet entries.",
			11,
		),
	)
})

var _ = Describe("ExtractUnreleasedBullets", func() {
	DescribeTable("ExtractUnreleasedBullets",
		func(content []byte, expected []string) {
			result := changelog.ExtractUnreleasedBullets(content)
			Expect(result).To(Equal(expected))
		},
		Entry(
			"extracts bullets in order",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- feat: add foo\n- fix: add bar\n- refactor: add baz\n\n## v1.0.0\n",
			),
			[]string{"feat: add foo", "fix: add bar", "refactor: add baz"},
		),
		Entry("empty Unreleased returns non-nil empty slice",
			[]byte("# Changelog\n\n## Unreleased\n\nNo bullets here.\n\n## v1.0.0\n"),
			[]string{}),
		Entry("absent Unreleased returns nil",
			[]byte("# Changelog\n\n## v1.0.0\n\n- initial\n"),
			nil),
		Entry(
			"only first Unreleased block is parsed",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- first bullet\n\n## v1.0.0\n\n## Unreleased\n\n- second bullet\n",
			),
			[]string{"first bullet"},
		),
		Entry("nil content returns nil",
			nil,
			nil),
		Entry("empty content returns nil",
			[]byte{},
			nil),
		Entry("bullet with leading whitespace after dash space",
			[]byte("# Changelog\n\n## Unreleased\n\n-  leading space after dash\n\n## v1.0.0\n"),
			[]string{" leading space after dash"}),
	)
})

var _ = Describe("InferHeaderPrefixStyle", func() {
	DescribeTable("InferHeaderPrefixStyle",
		func(content []byte, expected string) {
			result := changelog.InferHeaderPrefixStyle(content)
			Expect(result).To(Equal(expected))
		},
		Entry("v-prefix historic",
			[]byte("# Changelog\n\n## Unreleased\n\n## v1.2.3\n"),
			"v"),
		Entry("no-prefix historic",
			[]byte("# Changelog\n\n## Unreleased\n\n## 1.2.3\n"),
			""),
		Entry("no historic release defaults to v",
			[]byte("# Changelog\n\n## Unreleased\n"),
			"v"),
		Entry("nil content defaults to v",
			nil,
			"v"),
		Entry("empty content defaults to v",
			[]byte{},
			"v"),
		Entry("only Unreleased heading defaults to v",
			[]byte("# Changelog\n\n## Unreleased\n\n- feat: add foo\n"),
			"v"),
		Entry("v-prefix with longer version",
			[]byte("# Changelog\n\n## Unreleased\n\n## v10.20.30\n"),
			"v"),
		Entry("no-prefix with longer version",
			[]byte("# Changelog\n\n## Unreleased\n\n## 10.20.30\n"),
			""),
		Entry("malformed heading keeps scanning",
			[]byte("# Changelog\n\n## Unreleased\n\n## v1.2.3\n\n## InvalidHeading\n"),
			"v"),
	)
})

var _ = Describe("RewriteUnreleasedHeader", func() {
	DescribeTable("replaces ## Unreleased line with the given header",
		func(input []byte, newHeader string, expected []byte) {
			got, err := changelog.RewriteUnreleasedHeader(context.Background(), input, newHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got)).To(Equal(string(expected)))
		},
		Entry(
			"rewrite unreleased — happy path replaces the heading and preserves bullets",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.0.0\n\n- initial\n",
			),
			"## v1.0.1",
			[]byte(
				"# Changelog\n\n## v1.0.1\n\n- feat: add foo\n- fix: bar\n\n## v1.0.0\n\n- initial\n",
			),
		),
		Entry("rewrite unreleased — tolerates trailing whitespace on the heading line",
			[]byte("# Changelog\n\n## Unreleased   \n\n- feat: bar\n\n## v0.9.0\n\n- old\n"),
			"## v0.9.1",
			[]byte("# Changelog\n\n## v0.9.1\n\n- feat: bar\n\n## v0.9.0\n\n- old\n")),
		Entry("rewrite unreleased — first occurrence only when duplicate ## Unreleased present",
			[]byte("## Unreleased\n\n- a\n\n## Unreleased\n\n- b\n"),
			"## v1.2.8",
			[]byte("## v1.2.8\n\n- a\n\n## Unreleased\n\n- b\n")),
		Entry(
			"rewrite unreleased — empty newHeader replaces the heading with a blank line (current behavior)",
			[]byte("# Changelog\n\n## Unreleased\n\n- feat: add foo\n\n## v1.0.0\n\n- initial\n"),
			"",
			[]byte("# Changelog\n\n\n\n- feat: add foo\n\n## v1.0.0\n\n- initial\n"),
		),
	)

	DescribeTable("returns a wrapped error when ## Unreleased is absent",
		func(input []byte) {
			_, err := changelog.RewriteUnreleasedHeader(context.Background(), input, "## v1.2.3")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unreleased header not found"))
		},
		Entry("rewrite unreleased — error when no Unreleased heading present",
			[]byte("# Changelog\n\n## v1.0.0\n\n- initial\n")),
		Entry("rewrite unreleased — error on empty content",
			[]byte("")),
	)
})

var _ = Describe("ExtractUnreleasedBody", func() {
	DescribeTable(
		"returns verbatim body of ## Unreleased section",
		func(content []byte, expected string) {
			got, err := changelog.ExtractUnreleasedBody(context.Background(), content)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(expected))
		},
		Entry(
			"typical body with bullets is returned verbatim (incl. blank line right after heading)",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- feat: add foo\n- fix: bar\n\n## v1.0.0\n\n- old\n",
			),
			"\n- feat: add foo\n- fix: bar\n\n",
		),
		Entry("body without trailing blank line before next heading (incl. leading blank)",
			[]byte("## Unreleased\n\n- feat: x\n## v1.0.0\n"),
			"\n- feat: x\n"),
		Entry("body with extra leading blank line is preserved (no trim)",
			[]byte("## Unreleased\n\n\n- feat: x\n\n## v1.0.0\n"),
			"\n\n- feat: x\n\n"),
		Entry("multi-line body with blank lines between bullets is preserved",
			[]byte("## Unreleased\n\n- feat: a\n\n- fix: b\n\n## v1.0.0\n"),
			"\n- feat: a\n\n- fix: b\n\n"),
		Entry("body with trailing whitespace is preserved (line-ending is normalized to \\n)",
			[]byte("## Unreleased\n\n- feat: x   \n\n## v1.0.0\n"),
			"\n- feat: x   \n\n"),
		Entry("## Unreleased immediately followed by next heading returns empty string",
			[]byte("## Unreleased\n## v1.0.0\n\n- old\n"),
			""),
		Entry(
			"## Unreleased with only blank lines before next heading returns just those blank lines",
			[]byte("## Unreleased\n\n\n## v1.0.0\n\n- old\n"),
			"\n\n",
		),
		Entry("## Unreleased at end of file with no body returns empty string",
			[]byte("# Changelog\n\n## Unreleased\n"),
			""),
		Entry("only first Unreleased block body is returned",
			[]byte("## Unreleased\n\n- first\n\n## v1.0.0\n\n## Unreleased\n\n- second\n"),
			"\n- first\n\n"),
	)

	DescribeTable("returns a wrapped error when ## Unreleased is absent",
		func(content []byte) {
			_, err := changelog.ExtractUnreleasedBody(context.Background(), content)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unreleased header not found"))
		},
		Entry("absent Unreleased heading returns error",
			[]byte("# Changelog\n\n## v1.0.0\n\n- initial\n")),
		Entry("nil content returns error",
			nil),
		Entry("empty content returns error",
			[]byte{}),
	)
})

var _ = Describe("ReplaceUnreleasedBody", func() {
	DescribeTable(
		"replaces ## Unreleased body with newBody; preserves text before/after",
		func(input []byte, newBody string, expected []byte) {
			got, err := changelog.ReplaceUnreleasedBody(
				context.Background(),
				input,
				newBody,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got)).To(Equal(string(expected)))
		},
		Entry(
			"typical replacement preserves text before and after",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- raw commit line one\n- raw commit line two\n\n## v1.0.0\n\n- initial\n",
			),
			"- feat: cleaned\n",
			[]byte(
				"# Changelog\n\n## Unreleased\n- feat: cleaned\n## v1.0.0\n\n- initial\n",
			),
		),
		Entry(
			"empty new body produces just the heading + blank line + next heading",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- raw line\n\n## v1.0.0\n\n- initial\n",
			),
			"",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n## v1.0.0\n\n- initial\n",
			),
		),
		Entry(
			"newBody without trailing \\n gets a single \\n appended before the next heading",
			[]byte(
				"# Changelog\n\n## Unreleased\n\n- raw line\n\n## v1.0.0\n\n- initial\n",
			),
			"- feat: cleaned",
			[]byte(
				"# Changelog\n\n## Unreleased\n- feat: cleaned\n## v1.0.0\n\n- initial\n",
			),
		),
		Entry(
			"newBody already ends with \\n is not double-newlined",
			[]byte(
				"## Unreleased\n\n- raw line\n\n## v1.0.0\n",
			),
			"- feat: cleaned\n",
			[]byte(
				"## Unreleased\n- feat: cleaned\n## v1.0.0\n",
			),
		),
		Entry(
			"## Unreleased at end of file with newBody inserts cleanly",
			[]byte("# Changelog\n\n## Unreleased\n"),
			"- feat: cleaned\n",
			[]byte("# Changelog\n\n## Unreleased\n- feat: cleaned\n"),
		),
		Entry(
			"input without trailing newline preserves that property",
			[]byte("## Unreleased\n\n- raw line\n"),
			"- feat: cleaned\n",
			[]byte("## Unreleased\n- feat: cleaned\n"),
		),
		Entry(
			"first occurrence of ## Unreleased is replaced; later duplicate is left alone",
			[]byte(
				"## Unreleased\n\n- a\n\n## v1.0.0\n\n## Unreleased\n\n- b\n",
			),
			"- feat: cleaned\n",
			[]byte(
				"## Unreleased\n- feat: cleaned\n## v1.0.0\n\n## Unreleased\n\n- b\n",
			),
		),
	)

	DescribeTable("returns a wrapped error when ## Unreleased is absent",
		func(input []byte) {
			_, err := changelog.ReplaceUnreleasedBody(
				context.Background(),
				input,
				"- feat: cleaned\n",
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unreleased header not found"))
		},
		Entry("no ## Unreleased heading returns error",
			[]byte("# Changelog\n\n## v1.0.0\n\n- initial\n")),
		Entry("nil content returns error",
			nil),
		Entry("empty content returns error",
			[]byte{}),
	)
})
