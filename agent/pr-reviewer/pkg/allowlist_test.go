// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
)

var _ = Describe("ParseRepoAllowlist", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns nil for empty string", func() {
		result, err := pkg.ParseRepoAllowlist(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("parses a single valid entry", func() {
		result, err := pkg.ParseRepoAllowlist(ctx, "github.com/bborbe/maintainer")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]string{"github.com/bborbe/maintainer"}))
	})

	It("parses multiple valid entries", func() {
		result, err := pkg.ParseRepoAllowlist(
			ctx,
			"github.com/bborbe/maintainer,github.com/bborbe/agent",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			result,
		).To(Equal([]string{"github.com/bborbe/maintainer", "github.com/bborbe/agent"}))
	})

	It("strips whitespace around entries", func() {
		result, err := pkg.ParseRepoAllowlist(
			ctx,
			" github.com/bborbe/maintainer , github.com/bborbe/agent ",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			result,
		).To(Equal([]string{"github.com/bborbe/maintainer", "github.com/bborbe/agent"}))
	})

	It("silently drops trailing comma (empty entry)", func() {
		result, err := pkg.ParseRepoAllowlist(ctx, "github.com/bborbe/maintainer,")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal([]string{"github.com/bborbe/maintainer"}))
	})

	It("returns error for non-host-qualified entry (two segments)", func() {
		_, err := pkg.ParseRepoAllowlist(ctx, "bborbe/code-reviewer")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bborbe/code-reviewer"))
	})

	It("returns error for single-segment entry", func() {
		_, err := pkg.ParseRepoAllowlist(ctx, "code-reviewer")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("code-reviewer"))
	})

	It("returns error for four-segment entry", func() {
		_, err := pkg.ParseRepoAllowlist(ctx, "github.com/bborbe/maintainer/extra")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("github.com/bborbe/maintainer/extra"))
	})
})
