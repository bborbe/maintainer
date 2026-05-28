// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

var _ = Describe("filter.AutoReleaseFilter", func() {
	It("AutoReleaseFilter skips when AutoRelease is true", func() {
		f := filter.NewAutoReleaseFilter()
		Expect(f.Skip(filter.Release{AutoRelease: true})).To(Equal("auto_release"))
	})

	It("AutoReleaseFilter does not skip when AutoRelease is false", func() {
		f := filter.NewAutoReleaseFilter()
		Expect(f.Skip(filter.Release{AutoRelease: false})).To(BeEmpty())
	})

	It("AutoReleaseFilter does not skip the zero-value Release", func() {
		f := filter.NewAutoReleaseFilter()
		Expect(f.Skip(filter.Release{})).To(BeEmpty())
	})
})
