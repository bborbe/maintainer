// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package maintainerconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/maintainer/lib/maintainerconfig"
)

var _ = Describe("Parse", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	DescribeTable("valid documents",
		func(content string, expected maintainerconfig.MaintainerConfig) {
			cfg, err := maintainerconfig.Parse(ctx, []byte(content))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).To(Equal(expected))
		},
		Entry("empty bytes -> zero-value, nil",
			"",
			maintainerconfig.MaintainerConfig{}),
		Entry("prReviewer.autoApprove: true -> PrReviewer.AutoApprove true",
			"prReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
			}),
		Entry("prReviewer absent -> AutoApprove false",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{AutoRelease: true},
			}),
		Entry("release.autoRelease: true still parses -> Release.AutoRelease true",
			"release:\n  autoRelease: true\n",
			maintainerconfig.MaintainerConfig{
				Release: maintainerconfig.ReleaseConfig{AutoRelease: true},
			}),
		Entry("both namespaces populated -> both parsed",
			"release:\n  autoRelease: true\nprReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				Release:    maintainerconfig.ReleaseConfig{AutoRelease: true},
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
			}),
		Entry("unknown top-level key ignored, no error",
			"build-fix:\n  enabled: true\nprReviewer:\n  autoApprove: true\n",
			maintainerconfig.MaintainerConfig{
				PrReviewer: maintainerconfig.PrReviewerConfig{AutoApprove: true},
			}),
	)

	It("malformed YAML -> wrapped error", func() {
		cfg, err := maintainerconfig.Parse(ctx, []byte("prReviewer:\n  autoApprove: [unclosed\n"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unmarshal .maintainer.yaml"))
		Expect(cfg).To(Equal(maintainerconfig.MaintainerConfig{}))
	})
})
