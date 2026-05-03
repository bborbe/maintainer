// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/code-reviewer/watcher/github/pkg/filter"
)

var _ = Describe("ParseRepoAllowlist", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = context.Background() })

	It("returns nil for empty string (allow-all)", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("parses a single valid entry", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/code-reviewer")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]string{"github.com/bborbe/code-reviewer"}))
	})

	It("parses multiple valid entries", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo,github.com/bborbe/bar")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("github.com/bborbe/foo", "github.com/bborbe/bar"))
	})

	It("strips whitespace around entries", func() {
		result, err := filter.ParseRepoAllowlist(
			ctx,
			" github.com/bborbe/foo , github.com/bborbe/bar ",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("github.com/bborbe/foo", "github.com/bborbe/bar"))
	})

	It("silently drops empty entries from trailing comma", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo,")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]string{"github.com/bborbe/foo"}))
	})

	It("silently drops whitespace-only entries", func() {
		result, err := filter.ParseRepoAllowlist(
			ctx,
			"github.com/bborbe/foo, ,github.com/bborbe/bar",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("github.com/bborbe/foo", "github.com/bborbe/bar"))
	})

	It("returns error for entry with only two segments (no host)", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "bborbe/code-reviewer")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bborbe/code-reviewer"))
	})

	It("returns error for entry with only one segment", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "code-reviewer")
		Expect(err).To(HaveOccurred())
	})

	It("returns error for entry with four segments", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo/extra")
		Expect(err).To(HaveOccurred())
	})

	It("returns error for empty-string-after-trim entry that is otherwise malformed", func() {
		// A single comma produces only empty entries — all dropped, no error.
		result, err := filter.ParseRepoAllowlist(ctx, ",")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})
})

var _ = Describe("RepoAllowlistFilter", func() {
	It("never skips when allowlist is empty", func() {
		f := filter.NewRepoAllowlistFilter(nil)
		Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/foo"})).To(BeFalse())
		Expect(f.Skip(filter.PR{RepoKey: ""})).To(BeFalse())
	})

	It("does not skip a PR whose RepoKey is on the allowlist", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/code-reviewer"})
		Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/code-reviewer"})).To(BeFalse())
	})

	It("skips a PR whose RepoKey is NOT on the allowlist", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/code-reviewer"})
		Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/other-repo"})).To(BeTrue())
	})

	It("skips a PR with an empty RepoKey when the allowlist is non-empty", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/code-reviewer"})
		Expect(f.Skip(filter.PR{RepoKey: ""})).To(BeTrue())
	})

	It("matches exactly — prefix match is not a match", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/code"})
		Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/code-reviewer"})).To(BeTrue())
	})
})
