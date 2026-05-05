// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-build/pkg/filter"
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
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/go-skeleton")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]string{"github.com/bborbe/go-skeleton"}))
	})

	It("parses multiple valid entries", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/a,github.com/bborbe/b")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("github.com/bborbe/a", "github.com/bborbe/b"))
	})

	It("strips whitespace and drops empty entries from trailing comma", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/a , github.com/bborbe/b,")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("github.com/bborbe/a", "github.com/bborbe/b"))
	})

	It("silently drops whitespace-only entries", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/a, ,github.com/bborbe/b")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("github.com/bborbe/a", "github.com/bborbe/b"))
	})

	It("returns nil for comma-only input (all entries empty after trim)", func() {
		result, err := filter.ParseRepoAllowlist(ctx, ",")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("returns error for two-segment entry (missing host)", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "bborbe/repo")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bborbe/repo"))
	})

	It("returns error for four-segment entry", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "a/b/c/d")
		Expect(err).To(HaveOccurred())
	})

	It("returns error for host segment with underscore", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "git_hub.com/owner/repo")
		Expect(err).To(HaveOccurred())
	})

	It("parses real dev.env value (regression: startup shape)", func() {
		result, err := filter.ParseRepoAllowlist(
			ctx,
			"github.com/bborbe/go-skeleton,github.com/bborbe/jira-task-creator",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			result,
		).To(ConsistOf("github.com/bborbe/go-skeleton", "github.com/bborbe/jira-task-creator"))
	})
})

var _ = Describe("RepoAllowlistFilter (host-qualified keys)", func() {
	It("never skips when allowlist is empty", func() {
		f := filter.NewRepoAllowlistFilter(nil)
		Expect(f.Skip("github.com/bborbe/foo")).To(BeFalse())
		Expect(f.Skip("")).To(BeFalse())
	})

	It("does not skip a repo whose key is on the allowlist", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/maintainer"})
		Expect(f.Skip("github.com/bborbe/maintainer")).To(BeFalse())
	})

	It("skips a repo whose key is NOT on the allowlist", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/maintainer"})
		Expect(f.Skip("github.com/bborbe/other-repo")).To(BeTrue())
	})

	It("matches exactly — prefix match is not a match", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/code"})
		Expect(f.Skip("github.com/bborbe/maintainer")).To(BeTrue())
	})
})
