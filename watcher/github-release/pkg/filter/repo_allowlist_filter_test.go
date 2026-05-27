// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

var _ = Describe("filter.RepoAllowlistFilter", func() {
	It("RepoAllowlistFilter allows everything when allowlist is empty", func() {
		f := filter.NewRepoAllowlistFilter(nil)
		Expect(f.Skip(filter.Release{RepoKey: "github.com/anyone/anything"})).To(BeFalse())
	})

	It("RepoAllowlistFilter allows everything when allowlist is empty slice", func() {
		f := filter.NewRepoAllowlistFilter([]string{})
		Expect(f.Skip(filter.Release{RepoKey: "github.com/anyone/anything"})).To(BeFalse())
	})

	It("RepoAllowlistFilter skips repo outside the allowlist", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/docker-utils"})
		Expect(f.Skip(filter.Release{RepoKey: "github.com/bborbe/other-repo"})).To(BeTrue())
	})

	It("RepoAllowlistFilter does not skip repo present in the allowlist", func() {
		f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/docker-utils"})
		Expect(f.Skip(filter.Release{RepoKey: "github.com/bborbe/docker-utils"})).To(BeFalse())
	})
})

var _ = Describe("filter.ParseRepoAllowlist", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = context.Background() })

	It("ParseRepoAllowlist returns nil on empty input", func() {
		entries, err := filter.ParseRepoAllowlist(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeNil())
	})

	It("ParseRepoAllowlist trims whitespace and skips empty entries", func() {
		entries, err := filter.ParseRepoAllowlist(
			ctx,
			"github.com/bborbe/a, github.com/bborbe/b , , github.com/bborbe/c",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			entries,
		).To(Equal([]string{"github.com/bborbe/a", "github.com/bborbe/b", "github.com/bborbe/c"}))
	})
})
