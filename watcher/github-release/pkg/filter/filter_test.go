// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

var _ = Describe("filter.TaskCreationFilters", func() {
	It("TaskCreationFilters returns false when every filter votes false", func() {
		chain := filter.TaskCreationFilters{
			filter.NewEmptyUnreleasedFilter(),
			filter.NewAutoReleaseFilter(),
		}
		Expect(chain.Skip(filter.Release{
			UnreleasedBullets: 3,
			AutoRelease:       false,
		})).To(BeFalse())
	})

	It("TaskCreationFilters returns true on first filter that votes true", func() {
		chain := filter.TaskCreationFilters{
			filter.NewEmptyUnreleasedFilter(),
			filter.NewAutoReleaseFilter(),
		}
		Expect(chain.Skip(filter.Release{
			UnreleasedBullets: 0,
			AutoRelease:       false,
		})).To(BeTrue())
	})

	It("TaskCreationFilters returns true when later filter votes true", func() {
		chain := filter.TaskCreationFilters{
			filter.NewEmptyUnreleasedFilter(),
			filter.NewAutoReleaseFilter(),
		}
		Expect(chain.Skip(filter.Release{
			UnreleasedBullets: 3,
			AutoRelease:       true,
		})).To(BeTrue())
	})

	It("TaskCreationFilters with empty slice never skips", func() {
		var chain filter.TaskCreationFilters
		Expect(chain.Skip(filter.Release{})).To(BeFalse())
	})
})
