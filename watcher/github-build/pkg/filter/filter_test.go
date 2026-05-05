// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
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
