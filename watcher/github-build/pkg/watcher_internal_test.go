// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("splitRepoKey", func() {
	DescribeTable("parses allowlist key into owner and repo",
		func(key, wantOwner, wantRepo string) {
			gotOwner, gotRepo := splitRepoKey(key)
			Expect(gotOwner).To(Equal(wantOwner))
			Expect(gotRepo).To(Equal(wantRepo))
		},
		Entry("three-segment host/owner/repo", "github.com/owner/repo", "owner", "repo"),
		Entry("two-segment owner/repo", "owner/repo", "owner", "repo"),
		Entry("single segment", "single", "single", ""),
		Entry("empty string", "", "", ""),
		Entry("four segments (invalid)", "a/b/c/d", "a/b/c/d", ""),
	)
})
