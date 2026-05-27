// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
)

type fakeCursorReader struct {
	data map[string]string
}

func (f *fakeCursorReader) LastSeenSHA(repoKey string) string {
	return f.data[repoKey]
}

var _ = Describe("filter.SHAUnchangedFilter", func() {
	It("SHAUnchangedFilter skips when LastSeenSHA equals HeadSHA", func() {
		data := map[string]string{"github.com/bborbe/docker-utils": "d630ef3"}
		f := filter.NewSHAUnchangedFilter(&fakeCursorReader{data: data})
		Expect(f.Skip(filter.Release{
			RepoKey: "github.com/bborbe/docker-utils",
			HeadSHA: "d630ef3",
		})).To(BeTrue())
	})

	It("SHAUnchangedFilter emits when LastSeenSHA differs from HeadSHA", func() {
		data := map[string]string{"github.com/bborbe/docker-utils": "d630ef3"}
		f := filter.NewSHAUnchangedFilter(&fakeCursorReader{data: data})
		Expect(f.Skip(filter.Release{
			RepoKey: "github.com/bborbe/docker-utils",
			HeadSHA: "different-sha",
		})).To(BeFalse())
	})

	It("SHAUnchangedFilter emits when repo is unseen by the cursor", func() {
		f := filter.NewSHAUnchangedFilter(&fakeCursorReader{data: map[string]string{}})
		Expect(f.Skip(filter.Release{
			RepoKey: "github.com/bborbe/new-repo",
			HeadSHA: "abc123",
		})).To(BeFalse())
	})

	It("SHAUnchangedFilter handles empty HeadSHA against unseen repo", func() {
		// degenerate case — production path never passes empty HeadSHA through; documented for posterity
		f := filter.NewSHAUnchangedFilter(&fakeCursorReader{data: map[string]string{}})
		Expect(f.Skip(filter.Release{
			RepoKey: "x",
			HeadSHA: "",
		})).To(BeTrue())
	})
})
