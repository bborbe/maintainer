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

var _ = Describe("RepoAllowlistFilter", func() {
	Context("with empty allowlist", func() {
		It("never skips (allow-all)", func() {
			f := filter.NewRepoAllowlistFilter(nil)
			Expect(f.Skip("owner/repo")).To(BeFalse())
			Expect(f.Skip("any/repo")).To(BeFalse())
		})
	})

	Context("with non-empty allowlist", func() {
		It("skips repos not in the list", func() {
			f := filter.NewRepoAllowlistFilter([]string{"owner/allowed", "owner/also-allowed"})
			Expect(f.Skip("owner/not-allowed")).To(BeTrue())
			Expect(f.Skip("other/repo")).To(BeTrue())
		})

		It("passes repos in the list", func() {
			f := filter.NewRepoAllowlistFilter([]string{"owner/allowed", "owner/also-allowed"})
			Expect(f.Skip("owner/allowed")).To(BeFalse())
			Expect(f.Skip("owner/also-allowed")).To(BeFalse())
		})
	})
})

var _ = Describe("ParseRepoAllowlist", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns nil for empty string (allow-all)", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("parses a single valid entry", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "owner/repo")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("owner/repo"))
	})

	It("parses multiple comma-separated entries", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "owner/repo,owner/other")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("owner/repo", "owner/other"))
	})

	It("trims whitespace around entries", func() {
		result, err := filter.ParseRepoAllowlist(ctx, " owner/repo , owner/other ")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("owner/repo", "owner/other"))
	})

	It("drops empty entries from trailing commas", func() {
		result, err := filter.ParseRepoAllowlist(ctx, "owner/repo,")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf("owner/repo"))
	})

	It("returns error for entry without slash", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "invalid")
		Expect(err).To(HaveOccurred())
	})

	It("returns error for entry with too many segments", func() {
		_, err := filter.ParseRepoAllowlist(ctx, "host/owner/repo")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("RepoFilters OR-composite", func() {
	It("never skips when empty", func() {
		var fs filter.RepoFilters
		Expect(fs.Skip("owner/repo")).To(BeFalse())
	})

	It("skips if any filter votes skip", func() {
		fs := filter.RepoFilters{
			filter.NewRepoAllowlistFilter([]string{"owner/a"}),
			filter.NewRepoAllowlistFilter([]string{"owner/b"}),
		}
		// "owner/c" is not in either allowlist → at least one skips
		Expect(fs.Skip("owner/c")).To(BeTrue())
	})

	It("does not skip when all filters pass", func() {
		fs := filter.RepoFilters{
			filter.NewRepoAllowlistFilter([]string{"owner/a", "owner/b"}),
			filter.NewRepoAllowlistFilter(nil), // allow-all
		}
		Expect(fs.Skip("owner/a")).To(BeFalse())
	})
})
